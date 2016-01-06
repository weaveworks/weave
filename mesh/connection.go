package mesh

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

type Connection interface {
	Local() *Peer
	Remote() *Peer
	RemoteTCPAddr() string
	Outbound() bool
	Established() bool
	BreakTie(Connection) ConnectionTieBreak
	Shutdown(error)
	Log(args ...interface{})
}

type ConnectionTieBreak int

const (
	TieBreakWon ConnectionTieBreak = iota
	TieBreakLost
	TieBreakTied
)

var ErrConnectToSelf = fmt.Errorf("Cannot connect to ourself")

type RemoteConnection struct {
	local         *Peer
	remote        *Peer
	remoteTCPAddr string
	outbound      bool
	established   bool
}

type LocalConnection struct {
	sync.RWMutex
	RemoteConnection
	TCPConn         *net.TCPConn
	TrustRemote     bool // is remote on a trusted subnet?
	TrustedByRemote bool // does remote trust us?
	version         byte
	tcpSender       TCPSender
	SessionKey      *[32]byte
	heartbeatTCP    *time.Ticker
	Router          *Router
	uid             uint64
	actionChan      chan<- ConnectionAction
	errorChan       chan<- error
	finished        <-chan struct{} // closed to signal that actorLoop has finished
	OverlayConn     OverlayConnection
	gossipSenders   *GossipSenders
}

type GossipConnection interface {
	GossipSenders() *GossipSenders
}

type ConnectionAction func() error

func NewRemoteConnection(from, to *Peer, tcpAddr string, outbound bool, established bool) *RemoteConnection {
	return &RemoteConnection{
		local:         from,
		remote:        to,
		remoteTCPAddr: tcpAddr,
		outbound:      outbound,
		established:   established,
	}
}

func (conn *RemoteConnection) Local() *Peer                           { return conn.local }
func (conn *RemoteConnection) Remote() *Peer                          { return conn.remote }
func (conn *RemoteConnection) RemoteTCPAddr() string                  { return conn.remoteTCPAddr }
func (conn *RemoteConnection) Outbound() bool                         { return conn.outbound }
func (conn *RemoteConnection) Established() bool                      { return conn.established }
func (conn *RemoteConnection) BreakTie(Connection) ConnectionTieBreak { return TieBreakTied }
func (conn *RemoteConnection) Shutdown(error)                         {}

func (conn *RemoteConnection) Log(args ...interface{}) {
	log.Println(append(append([]interface{}{}, fmt.Sprintf("->[%s|%s]:", conn.remoteTCPAddr, conn.remote)), args...)...)
}

func (conn *RemoteConnection) ErrorLog(args ...interface{}) {
	log.Errorln(append(append([]interface{}{}, fmt.Sprintf("->[%s|%s]:", conn.remoteTCPAddr, conn.remote)), args...)...)
}

// Does not return anything. If the connection is successful, it will
// end up in the local peer's connections map.
func StartLocalConnection(connRemote *RemoteConnection, tcpConn *net.TCPConn, router *Router, acceptNewPeer bool) {
	if connRemote.local != router.Ourself.Peer {
		log.Fatal("Attempt to create local connection from a peer which is not ourself")
	}
	actionChan := make(chan ConnectionAction, ChannelSize)
	errorChan := make(chan error, 1)
	finished := make(chan struct{})
	conn := &LocalConnection{
		RemoteConnection: *connRemote, // NB, we're taking a copy of connRemote here.
		Router:           router,
		TCPConn:          tcpConn,
		TrustRemote:      router.Trusts(connRemote),
		uid:              randUint64(),
		actionChan:       actionChan,
		errorChan:        errorChan,
		finished:         finished}
	conn.gossipSenders = NewGossipSenders(conn, finished)
	go conn.run(actionChan, errorChan, finished, acceptNewPeer)
}

func (conn *LocalConnection) BreakTie(dupConn Connection) ConnectionTieBreak {
	dupConnLocal := dupConn.(*LocalConnection)
	// conn.uid is used as the tie breaker here, in the knowledge that
	// both sides will make the same decision.
	if conn.uid < dupConnLocal.uid {
		return TieBreakWon
	} else if dupConnLocal.uid < conn.uid {
		return TieBreakLost
	} else {
		return TieBreakTied
	}
}

func (conn *LocalConnection) Established() bool {
	return conn.established
}

func (conn *LocalConnection) SendProtocolMsg(m ProtocolMsg) error {
	if err := conn.sendProtocolMsg(m); err != nil {
		conn.Shutdown(err)
		return err
	}
	return nil
}

func (conn *LocalConnection) GossipSenders() *GossipSenders {
	return conn.gossipSenders
}

// ACTOR methods

// NB: The conn.* fields are only written by the connection actor
// process, which is the caller of the ConnectionAction funs. Hence we
// do not need locks for reading, and only need write locks for fields
// read by other processes.

// Non-blocking.
func (conn *LocalConnection) Shutdown(err error) {
	// err should always be a real error, even if only io.EOF
	if err == nil {
		panic("nil error")
	}

	select {
	case conn.errorChan <- err:
	default:
	}
}

// Send an actor request to the actorLoop, but don't block if
// actorLoop has exited - see http://blog.golang.org/pipelines for
// pattern
func (conn *LocalConnection) sendAction(action ConnectionAction) {
	select {
	case conn.actionChan <- action:
	case <-conn.finished:
	}
}

// ACTOR server

func (conn *LocalConnection) run(actionChan <-chan ConnectionAction, errorChan <-chan error, finished chan<- struct{}, acceptNewPeer bool) {
	var err error // important to use this var and not create another one with 'err :='
	defer func() { conn.shutdown(err) }()
	defer close(finished)

	conn.TCPConn.SetLinger(0)
	intro, err := ProtocolIntroParams{
		MinVersion: conn.Router.ProtocolMinVersion,
		MaxVersion: ProtocolMaxVersion,
		Features:   conn.makeFeatures(),
		Conn:       conn.TCPConn,
		Password:   conn.Router.Password,
		Outbound:   conn.outbound,
	}.DoIntro()
	if err != nil {
		return
	}

	conn.SessionKey = intro.SessionKey
	conn.tcpSender = intro.Sender
	conn.version = intro.Version

	remote, err := conn.parseFeatures(intro.Features)
	if err != nil {
		return
	}

	if err = conn.registerRemote(remote, acceptNewPeer); err != nil {
		return
	}

	conn.Log("connection ready; using protocol version", conn.version)

	// only use negotiated session key for untrusted connections
	var sessionKey *[32]byte
	if conn.Untrusted() {
		sessionKey = conn.SessionKey
	}

	params := OverlayConnectionParams{
		RemotePeer:         conn.remote,
		LocalAddr:          conn.TCPConn.LocalAddr().(*net.TCPAddr),
		RemoteAddr:         conn.TCPConn.RemoteAddr().(*net.TCPAddr),
		Outbound:           conn.outbound,
		ConnUID:            conn.uid,
		SessionKey:         sessionKey,
		SendControlMessage: conn.sendOverlayControlMessage,
		Features:           intro.Features,
	}
	if conn.OverlayConn, err = conn.Router.Overlay.PrepareConnection(params); err != nil {
		return
	}

	// As soon as we do AddConnection, the new connection becomes
	// visible to the packet routing logic.  So AddConnection must
	// come after PrepareConnection
	if err = conn.Router.Ourself.AddConnection(conn); err != nil {
		return
	}
	conn.Router.ConnectionMaker.ConnectionCreated(conn)

	// OverlayConnection confirmation comes after AddConnection,
	// because only after that completes do we know the connection is
	// valid: in particular that it is not a duplicate connection to
	// the same peer. Overlay communication on a duplicate connection
	// can cause problems such as tripping up overlay crypto at the
	// other end due to data being decoded by the other connection. It
	// is also generally wasteful to engage in any interaction with
	// the remote on a connection that turns out to be invalid.
	conn.OverlayConn.Confirm()

	// receiveTCP must follow also AddConnection. In the absence
	// of any indirect connectivity to the remote peer, the first
	// we hear about it (and any peers reachable from it) is
	// through topology gossip it sends us on the connection. We
	// must ensure that the connection has been added to Ourself
	// prior to processing any such gossip, otherwise we risk
	// immediately gc'ing part of that newly received portion of
	// the topology (though not the remote peer itself, since that
	// will have a positive ref count), leaving behind dangling
	// references to peers. Hence we must invoke AddConnection,
	// which is *synchronous*, first.
	conn.heartbeatTCP = time.NewTicker(TCPHeartbeat)
	go conn.receiveTCP(intro.Receiver)

	// AddConnection must precede actorLoop. More precisely, it
	// must precede shutdown, since that invokes DeleteConnection
	// and is invoked on termination of this entire
	// function. Essentially this boils down to a prohibition on
	// running AddConnection in a separate goroutine, at least not
	// without some synchronisation. Which in turn requires the
	// launching of the receiveTCP goroutine to precede actorLoop.
	err = conn.actorLoop(actionChan, errorChan)
}

func (conn *LocalConnection) makeFeatures() map[string]string {
	features := map[string]string{
		"PeerNameFlavour": PeerNameFlavour,
		"Name":            conn.local.Name.String(),
		"NickName":        conn.local.NickName,
		"ShortID":         fmt.Sprint(conn.local.ShortID),
		"UID":             fmt.Sprint(conn.local.UID),
		"ConnID":          fmt.Sprint(conn.uid),
		"Trusted":         fmt.Sprint(conn.TrustRemote),
	}
	conn.Router.Overlay.AddFeaturesTo(features)
	return features
}

type features map[string]string

func (features features) MustHave(keys []string) error {
	for _, key := range keys {
		if _, ok := features[key]; !ok {
			return fmt.Errorf("Field %s is missing", key)
		}
	}
	return nil
}

func (features features) Get(key string) string {
	return features[key]
}

func (conn *LocalConnection) parseFeatures(features features) (*Peer, error) {
	if err := features.MustHave([]string{"PeerNameFlavour", "Name", "NickName", "UID", "ConnID"}); err != nil {
		return nil, err
	}

	remotePeerNameFlavour := features.Get("PeerNameFlavour")
	if remotePeerNameFlavour != PeerNameFlavour {
		return nil, fmt.Errorf("Peer name flavour mismatch (ours: '%s', theirs: '%s')", PeerNameFlavour, remotePeerNameFlavour)
	}

	name, err := PeerNameFromString(features.Get("Name"))
	if err != nil {
		return nil, err
	}

	nickName := features.Get("NickName")

	var shortID uint64
	var hasShortID bool
	if shortIDStr, present := features["ShortID"]; present {
		hasShortID = true
		shortID, err = strconv.ParseUint(shortIDStr, 10, PeerShortIDBits)
		if err != nil {
			return nil, err
		}
	}

	var trusted bool
	if trustedStr, present := features["Trusted"]; present {
		trusted, err = strconv.ParseBool(trustedStr)
		if err != nil {
			return nil, err
		}
	}
	conn.TrustedByRemote = trusted

	uid, err := ParsePeerUID(features.Get("UID"))
	if err != nil {
		return nil, err
	}

	remoteConnID, err := strconv.ParseUint(features.Get("ConnID"), 10, 64)
	if err != nil {
		return nil, err
	}

	conn.uid ^= remoteConnID
	peer := NewPeer(name, nickName, uid, 0, PeerShortID(shortID))
	peer.HasShortID = hasShortID
	return peer, nil
}

func (conn *LocalConnection) registerRemote(remote *Peer, acceptNewPeer bool) error {
	if acceptNewPeer {
		conn.remote = conn.Router.Peers.FetchWithDefault(remote)
	} else {
		conn.remote = conn.Router.Peers.FetchAndAddRef(remote.Name)
		if conn.remote == nil {
			return fmt.Errorf("Found unknown remote name: %s at %s", remote.Name, conn.remoteTCPAddr)
		}
	}

	if conn.remote == conn.local {
		return ErrConnectToSelf
	}

	return nil
}

func (conn *LocalConnection) actorLoop(actionChan <-chan ConnectionAction, errorChan <-chan error) (err error) {
	fwdErrorChan := conn.OverlayConn.ErrorChannel()
	fwdEstablishedChan := conn.OverlayConn.EstablishedChannel()

	for err == nil {
		select {
		case err = <-errorChan:
		case err = <-fwdErrorChan:
		default:
			select {
			case action := <-actionChan:
				err = action()
			case <-conn.heartbeatTCP.C:
				err = conn.sendSimpleProtocolMsg(ProtocolHeartbeat)
			case <-fwdEstablishedChan:
				conn.established = true
				fwdEstablishedChan = nil
				conn.Router.Ourself.ConnectionEstablished(conn)
			case err = <-errorChan:
			case err = <-fwdErrorChan:
			}
		}
	}
	return
}

func (conn *LocalConnection) shutdown(err error) {
	if conn.remote == nil {
		log.Errorf("->[%s] connection shutting down due to error during handshake: %v", conn.remoteTCPAddr, err)
	} else {
		conn.ErrorLog("connection shutting down due to error:", err)
	}

	if conn.TCPConn != nil {
		checkWarn(conn.TCPConn.Close())
	}

	if conn.remote != nil {
		conn.Router.Peers.Dereference(conn.remote)
		conn.Router.Ourself.DeleteConnection(conn)
	}

	if conn.heartbeatTCP != nil {
		conn.heartbeatTCP.Stop()
	}

	if conn.OverlayConn != nil {
		conn.OverlayConn.Stop()
	}

	conn.Router.ConnectionMaker.ConnectionTerminated(conn, err)
}

func (conn *LocalConnection) sendOverlayControlMessage(tag byte, msg []byte) error {
	return conn.sendProtocolMsg(ProtocolMsg{ProtocolTag(tag), msg})
}

// Helpers

func (conn *LocalConnection) sendSimpleProtocolMsg(tag ProtocolTag) error {
	return conn.sendProtocolMsg(ProtocolMsg{tag: tag})
}

func (conn *LocalConnection) sendProtocolMsg(m ProtocolMsg) error {
	return conn.tcpSender.Send(append([]byte{byte(m.tag)}, m.msg...))
}

func (conn *LocalConnection) receiveTCP(receiver TCPReceiver) {
	var err error
	for {
		conn.extendReadDeadline()

		var msg []byte
		if msg, err = receiver.Receive(); err != nil {
			break
		}
		if len(msg) < 1 {
			conn.Log("ignoring blank msg")
			continue
		}
		if err = conn.handleProtocolMsg(ProtocolTag(msg[0]), msg[1:]); err != nil {
			break
		}
	}
	conn.Shutdown(err)
}

func (conn *LocalConnection) handleProtocolMsg(tag ProtocolTag, payload []byte) error {
	switch tag {
	case ProtocolHeartbeat:
	case ProtocolReserved1, ProtocolReserved2, ProtocolReserved3, ProtocolOverlayControlMsg:
		conn.OverlayConn.ControlMessage(byte(tag), payload)
	case ProtocolGossipUnicast, ProtocolGossipBroadcast, ProtocolGossip:
		return conn.Router.handleGossip(tag, payload)
	default:
		conn.Log("ignoring unknown protocol tag:", tag)
	}
	return nil
}

func (conn *LocalConnection) extendReadDeadline() {
	conn.TCPConn.SetReadDeadline(time.Now().Add(TCPHeartbeat * 2))
}

func (conn *LocalConnection) Untrusted() bool {
	return !conn.TrustRemote || !conn.TrustedByRemote
}
