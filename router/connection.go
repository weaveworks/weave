package router

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
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
	TCPConn       *net.TCPConn
	tcpSender     TCPSender
	remoteUDPAddr *net.UDPAddr
	SessionKey    *[32]byte
	heartbeatTCP  *time.Ticker
	Router        *Router
	uid           uint64
	actionChan    chan<- ConnectionAction
	finished      <-chan struct{} // closed to signal that actorLoop has finished
	forwarder     InterHostForwarder
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

func (conn *RemoteConnection) String() string {
	from := "<nil>"
	if conn.local != nil {
		from = conn.local.String()
	}
	to := "<nil>"
	if conn.remote != nil {
		to = conn.remote.String()
	}
	return fmt.Sprint("Connection ", from, "->", to)
}

// Does not return anything. If the connection is successful, it will
// end up in the local peer's connections map.
func StartLocalConnection(connRemote *RemoteConnection, tcpConn *net.TCPConn, udpAddr *net.UDPAddr, router *Router, acceptNewPeer bool) {
	if connRemote.local != router.Ourself.Peer {
		log.Fatal("Attempt to create local connection from a peer which is not ourself")
	}
	// NB, we're taking a copy of connRemote here.
	actionChan := make(chan ConnectionAction, ChannelSize)
	finished := make(chan struct{})
	conn := &LocalConnection{
		RemoteConnection: *connRemote,
		Router:           router,
		TCPConn:          tcpConn,
		remoteUDPAddr:    udpAddr,
		actionChan:       actionChan,
		finished:         finished}
	go conn.run(actionChan, finished, acceptNewPeer)
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
	conn.RLock()
	defer conn.RUnlock()
	return conn.established
}

// Send directly, not via the Actor.  If it goes via the Actor we can
// get a deadlock where LocalConnection is blocked talking to
// LocalPeer and LocalPeer is blocked trying send a ProtocolMsg via
// LocalConnection, and the channels are full in both directions so
// nothing can proceed.
func (conn *LocalConnection) SendProtocolMsg(m ProtocolMsg) {
	if err := conn.sendProtocolMsg(m); err != nil {
		conn.Shutdown(err)
	}
}

// ACTOR methods

// NB: The conn.* fields are only written by the connection actor
// process, which is the caller of the ConnectionAction funs. Hence we
// do not need locks for reading, and only need write locks for fields
// read by other processes.

// Async
func (conn *LocalConnection) Shutdown(err error) {
	// err should always be a real error, even if only io.EOF
	if err == nil {
		panic("nil error")
	}

	// Run on its own goroutine in case the channel is backed up
	go func() { conn.sendAction(func() error { return err }) }()
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

func (conn *LocalConnection) run(actionChan <-chan ConnectionAction, finished chan<- struct{}, acceptNewPeer bool) {
	var err error // important to use this var and not create another one with 'err :='
	defer func() { conn.shutdown(err) }()
	defer close(finished)

	dec, err := conn.prepareConnection(acceptNewPeer)
	if err != nil {
		return
	}

	// Tell the forwarder that the connection is confirmed
	conn.forwarder.SetListener(ConnectionAsForwarderListener{conn})

	conn.heartbeatTCP = time.NewTicker(TCPHeartbeat)
	go conn.receiveTCP(dec)
	err = conn.actorLoop(actionChan)
}

func (conn *LocalConnection) prepareConnection(acceptNewPeer bool) (*gob.Decoder, error) {
	tcpConn := conn.TCPConn
	tcpConn.SetLinger(0)
	enc := gob.NewEncoder(tcpConn)
	dec := gob.NewDecoder(tcpConn)

	remotePeer, err := conn.handshake(enc, dec, acceptNewPeer)
	if err != nil {
		return nil, err
	}
	conn.Log("completed handshake")

	if err = conn.setRemote(remotePeer); err != nil {
		return nil, err
	}

	// setRemote may have resulted in the local short id changing.
	// So we need to ensure we gossip that, even if we error out
	// below.
	defer conn.Router.broadcastPeerUpdate(remotePeer)

	// The ordering of the following is very important. [1]

	localIP := tcpConn.LocalAddr().(*net.TCPAddr).IP
	if conn.forwarder, err = conn.Router.InterHost.MakeForwarder(conn.remote, localIP, conn.remoteUDPAddr, conn.uid, conn.udpCrypto(), conn.sendInterHostControlMessage); err != nil {
		return nil, err
	}

	if err = conn.Router.Ourself.AddConnection(conn); err != nil {
		return nil, err
	}

	return dec, nil
}

// [1] Ordering constraints:
//
// (a) AddConnections must precede initHeartbeats. It is only after
// the former completes that we know the connection is valid, in
// particular is not a duplicate connection to the same peer. Sending
// heartbeats on a duplicate connection can trip up crypto at the
// other end, since the associated UDP packets may get decoded by the
// other connection. It is also generally wasteful to engage in any
// interaction with the remote on a connection that turns out to be
// invald.
//
// (b) AddConnection must precede receiveTCP. In the absence of any
// indirect connectivity to the remote peer, the first we hear about
// it (and any peers reachable from it) is through topology gossip it
// sends us on the connection. We must ensure that the connection has
// been added to Ourself prior to processing any such gossip,
// otherwise we risk immediately gc'ing part of that newly received
// portion of the topology (though not the remote peer itself, since
// that will have a positive ref count), leaving behind dangling
// references to peers. Hence we must invoke AddConnection, which is
// *synchronous*, first.
//
// (c) AddConnection must precede actorLoop. More precisely, it must
// precede shutdown, since that invokes DeleteConnection and is
// invoked on termination of this entire function. Essentially this
// boils down to a prohibition on running AddConnection in a separate
// goroutine, at least not without some synchronisation. Which in turn
// requires us the launching of the receiveTCP goroutine to precede
// actorLoop.
//
// (d) AddConnection should precede receiveTCP. There is no point
// starting the latter if the former fails.
//
// (e) initHeartbeats should precede actorLoop. The former is setting
// LocalConnection fields accessed by the latter. Since the latter
// runs in a separate goroutine, we'd have to add some synchronisation
// if initHeartbeats isn't run first.
//
// (f) ensureForwarders should precede AddConnection. As soon as a
// connection has been added to LocalPeer by the latter, it becomes
// visible to the packet routing logic, which will end up dropping
// packets if the forwarders haven't been created yet. We cannot
// prevent that completely, since, for example, forwarder can only be
// created when we know the remote UDP address, but it helps to try.

func (conn *LocalConnection) setRemote(toPeer *Peer) error {
	toPeer = conn.Router.Peers.FetchWithDefault(toPeer)
	switch toPeer {
	case nil:
		return fmt.Errorf("multiple peers with the same Name")
	case conn.local:
		conn.remote = toPeer // have to do assigment here to ensure Shutdown releases ref count
		return fmt.Errorf("Cannot connect to ourself")
	default:
		conn.remote = toPeer
		return nil
	}
}

func (conn *LocalConnection) actorLoop(actionChan <-chan ConnectionAction) (err error) {
	for err == nil {
		select {
		case action := <-actionChan:
			err = action()
		case <-conn.heartbeatTCP.C:
			err = conn.sendSimpleProtocolMsg(ProtocolHeartbeat)
		}
	}
	return
}

func (conn *LocalConnection) shutdown(err error) {
	if conn.remote == nil {
		log.Printf("->[%s] connection shutting down due to error during handshake: %v\n", conn.remoteTCPAddr, err)
	} else {
		conn.Log("connection shutting down due to error:", err)
	}

	if conn.TCPConn != nil {
		checkWarn(conn.TCPConn.Close())
	}

	if conn.remote != nil {
		conn.Router.Peers.Dereference(conn.remote)
		conn.Router.Ourself.DeleteConnection(conn)
	}

	stopTicker(conn.heartbeatTCP)

	if conn.forwarder != nil {
		conn.forwarder.Close()
	}

	conn.Router.ConnectionMaker.ConnectionTerminated(conn.remoteTCPAddr, err)
}

func (conn *LocalConnection) udpCrypto() InterHostCrypto {
	name := conn.local.NameByte
	if conn.Router.UsingPassword() {
		return InterHostCrypto{
			Dec:   NewNaClDecryptor(conn.SessionKey, conn.outbound),
			Enc:   NewNaClEncryptor(name, conn.SessionKey, conn.outbound, false),
			EncDF: NewNaClEncryptor(name, conn.SessionKey, conn.outbound, true),
		}
	} else {
		return InterHostCrypto{
			Dec:   NewNonDecryptor(),
			Enc:   NewNonEncryptor(name),
			EncDF: NewNonEncryptor(name),
		}
	}
}

func (conn *LocalConnection) sendInterHostControlMessage(msg []byte) error {
	return conn.sendProtocolMsg(ProtocolMsg{ProtocolInterHostControlMsg, msg})
}

type ConnectionAsForwarderListener struct{ conn *LocalConnection }

func (l ConnectionAsForwarderListener) Established() {
	l.conn.sendAction(func() error {
		old := l.conn.established
		l.conn.Lock()
		l.conn.established = true
		l.conn.Unlock()
		if !old {
			l.conn.Router.Ourself.ConnectionEstablished(l.conn)
		}
		return nil
	})
}

func (l ConnectionAsForwarderListener) Error(err error) {
	l.conn.sendAction(func() error { return err })
}

// Helpers

func (conn *LocalConnection) sendSimpleProtocolMsg(tag ProtocolTag) error {
	return conn.sendProtocolMsg(ProtocolMsg{tag: tag})
}

func (conn *LocalConnection) sendProtocolMsg(m ProtocolMsg) error {
	return conn.tcpSender.Send(Concat([]byte{byte(m.tag)}, m.msg))
}

func (conn *LocalConnection) receiveTCP(decoder *gob.Decoder) {
	usingPassword := conn.SessionKey != nil
	var receiver TCPReceiver
	if usingPassword {
		receiver = NewEncryptedTCPReceiver(conn.SessionKey, conn.outbound)
	} else {
		receiver = NewSimpleTCPReceiver()
	}
	var err error
	for {
		var msg []byte
		conn.extendReadDeadline()
		if err = decoder.Decode(&msg); err != nil {
			break
		}
		msg, err = receiver.Decode(msg)
		if err != nil {
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
	case ProtocolInterHostControlMsg:
		conn.forwarder.ControlMessage(payload)
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

func (conn *LocalConnection) Forward(key ForwardPacketKey) FlowOp {
	return conn.forwarder.Forward(key)
}

func tickerChan(ticker *time.Ticker) <-chan time.Time {
	if ticker != nil {
		return ticker.C
	}
	return nil
}

func stopTicker(ticker *time.Ticker) {
	if ticker != nil {
		ticker.Stop()
	}
}
