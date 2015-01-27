package router

import (
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type Connection interface {
	Local() *Peer
	Remote() *Peer
	BreakTie(Connection) ConnectionTieBreak
	RemoteTCPAddr() string
	Established() bool
	Shutdown(error)
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
}

type LocalConnection struct {
	sync.RWMutex
	RemoteConnection
	TCPConn           *net.TCPConn
	tcpSender         TCPSender
	remoteUDPAddr     *net.UDPAddr
	established       bool
	receivedHeartbeat bool
	stackFrag         bool
	effectivePMTU     int
	SessionKey        *[32]byte
	heartbeatFrame    *ForwardedFrame
	heartbeat         *time.Ticker
	fragTest          *time.Ticker
	forwardChan       chan<- *ForwardedFrame
	forwardChanDF     chan<- *ForwardedFrame
	stopForward       chan<- interface{}
	stopForwardDF     chan<- interface{}
	verifyPMTU        chan<- int
	Decryptor         Decryptor
	Router            *Router
	uid               uint64
	queryChan         chan<- *ConnectionInteraction
}

type ConnectionInteraction struct {
	Interaction
	payload interface{}
}

func NewRemoteConnection(from, to *Peer, tcpAddr string) *RemoteConnection {
	return &RemoteConnection{
		local:         from,
		remote:        to,
		remoteTCPAddr: tcpAddr}
}

func (conn *RemoteConnection) Local() *Peer {
	return conn.local
}

func (conn *RemoteConnection) Remote() *Peer {
	return conn.remote
}

func (conn *RemoteConnection) BreakTie(Connection) ConnectionTieBreak {
	return TieBreakTied
}

func (conn *RemoteConnection) RemoteTCPAddr() string {
	return conn.remoteTCPAddr
}

func (conn *RemoteConnection) Established() bool {
	return true
}

func (conn *RemoteConnection) Shutdown(error) {
}

func (conn *RemoteConnection) String() string {
	from := "<nil>"
	if conn.local != nil {
		from = conn.local.Name.String()
	}
	to := "<nil>"
	if conn.remote != nil {
		to = conn.remote.Name.String()
	}
	return fmt.Sprint("Connection ", from, "->", to)
}

func NewLocalConnection(connRemote *RemoteConnection, tcpConn *net.TCPConn, udpAddr *net.UDPAddr, router *Router) *LocalConnection {
	if connRemote.local != router.Ourself.Peer {
		log.Fatal("Attempt to create local connection from a peer which is not ourself")
	}
	// NB, we're taking a copy of connRemote here.
	return &LocalConnection{
		RemoteConnection: *connRemote,
		Router:           router,
		TCPConn:          tcpConn,
		remoteUDPAddr:    udpAddr,
		effectivePMTU:    DefaultPMTU}
}

// Async. Does not return anything. If the connection is successful,
// it will end up in the local peer's connections map.
func (conn *LocalConnection) Start(acceptNewPeer bool) {
	queryChan := make(chan *ConnectionInteraction, ChannelSize)
	conn.queryChan = queryChan
	go conn.run(queryChan, acceptNewPeer)
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

// Read by the forwarder processes when in the UDP senders
func (conn *LocalConnection) RemoteUDPAddr() *net.UDPAddr {
	conn.RLock()
	defer conn.RUnlock()
	return conn.remoteUDPAddr
}

func (conn *LocalConnection) Established() bool {
	conn.RLock()
	defer conn.RUnlock()
	return conn.established
}

// Called by forwarder processes, read in Forward (by sniffer and udp
// listener process in router).
func (conn *LocalConnection) setEffectivePMTU(pmtu int) {
	conn.Lock()
	defer conn.Unlock()
	if conn.effectivePMTU != pmtu {
		conn.effectivePMTU = pmtu
		conn.log("Effective PMTU set to", pmtu)
	}
}

// Called by the connection's actor process, and by the connection's
// TCP received process. StackFrag is read in conn.Forward (called by
// router udp listener and sniffer processes)
func (conn *LocalConnection) setStackFrag(frag bool) {
	conn.Lock()
	defer conn.Unlock()
	conn.stackFrag = frag
}

func (conn *LocalConnection) log(args ...interface{}) {
	log.Println(append(append([]interface{}{}, fmt.Sprintf("->[%s]:", conn.remote.Name)), args...)...)
}

// ACTOR client API

const (
	CSendProtocolMsg = iota
	CSetEstablished
	CReceivedHeartbeat
	CShutdown
)

// Async
func (conn *LocalConnection) Shutdown(err error) {
	conn.queryChan <- &ConnectionInteraction{
		Interaction: Interaction{code: CShutdown},
		payload:     err}
}

// Async
//
// Heartbeating serves two purposes: a) keeping NAT paths alive, and
// b) updating a remote peer's knowledge of our address, in the event
// it changes (e.g. because NAT paths expired).
func (conn *LocalConnection) ReceivedHeartbeat(remoteUDPAddr *net.UDPAddr, connUID uint64) {
	if remoteUDPAddr == nil || connUID != conn.uid {
		return
	}
	conn.queryChan <- &ConnectionInteraction{
		Interaction: Interaction{code: CReceivedHeartbeat},
		payload:     remoteUDPAddr}
}

// Async
func (conn *LocalConnection) SetEstablished() {
	conn.queryChan <- &ConnectionInteraction{Interaction: Interaction{code: CSetEstablished}}
}

// Async
func (conn *LocalConnection) SendProtocolMsg(m ProtocolMsg) {
	conn.queryChan <- &ConnectionInteraction{
		Interaction: Interaction{code: CSendProtocolMsg},
		payload:     m}
}

// ACTOR server

func (conn *LocalConnection) run(queryChan <-chan *ConnectionInteraction, acceptNewPeer bool) {
	defer conn.handleShutdown()

	tcpConn := conn.TCPConn
	tcpConn.SetLinger(0)
	enc := gob.NewEncoder(tcpConn)
	dec := gob.NewDecoder(tcpConn)

	if err := conn.handshake(enc, dec, acceptNewPeer); err != nil {
		log.Printf("->[%s] connection shutting down due to error during handshake: %v\n", conn.remoteTCPAddr, err)
		return
	}
	log.Printf("->[%s] completed handshake with %s\n", conn.remoteTCPAddr, conn.remote.Name)

	heartbeatFrameBytes := make([]byte, EthernetOverhead+8)
	binary.BigEndian.PutUint64(heartbeatFrameBytes[EthernetOverhead:], conn.uid)
	conn.heartbeatFrame = &ForwardedFrame{
		srcPeer: conn.local,
		dstPeer: conn.remote,
		frame:   heartbeatFrameBytes}

	go conn.receiveTCP(dec)
	conn.Router.Ourself.AddConnection(conn)

	if conn.remoteUDPAddr != nil {
		if err := conn.sendFastHeartbeats(); err != nil {
			conn.log("connection shutting down due to error:", err)
			return
		}
	}

	err := conn.queryLoop(queryChan)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() && !conn.established {
		conn.log("connection shutting down due to timeout; possibly caused by blocked UDP connectivity")
	} else if err != nil {
		conn.log("connection shutting down due to error:", err)
	} else {
		conn.log("connection shutting down")
	}
}

func (conn *LocalConnection) queryLoop(queryChan <-chan *ConnectionInteraction) (err error) {
	terminate := false
	for !terminate && err == nil {
		select {
		case query, ok := <-queryChan:
			if !ok {
				break
			}
			switch query.code {
			case CShutdown:
				err = query.payload.(error)
				terminate = true
			case CReceivedHeartbeat:
				err = conn.handleReceivedHeartbeat(query.payload.(*net.UDPAddr))
			case CSetEstablished:
				err = conn.handleSetEstablished()
			case CSendProtocolMsg:
				err = conn.handleSendProtocolMsg(query.payload.(ProtocolMsg))
			}
		case <-tickerChan(conn.heartbeat):
			conn.Forward(true, conn.heartbeatFrame, nil)
		case <-tickerChan(conn.fragTest):
			conn.setStackFrag(false)
			err = conn.handleSendSimpleProtocolMsg(ProtocolStartFragmentationTest)
		}
	}
	return
}

// Handlers
//
// NB: The conn.* fields are only written by the connection actor
// process, which is the caller of the handlers. Hence we do not need
// locks for reading, and only need write locks for fields read by
// other processes.

func (conn *LocalConnection) handleReceivedHeartbeat(remoteUDPAddr *net.UDPAddr) error {
	oldRemoteUDPAddr := conn.remoteUDPAddr
	old := conn.receivedHeartbeat
	conn.Lock()
	conn.remoteUDPAddr = remoteUDPAddr
	conn.receivedHeartbeat = true
	conn.Unlock()
	if !old {
		if err := conn.handleSendSimpleProtocolMsg(ProtocolConnectionEstablished); err != nil {
			return err
		}
	}
	if oldRemoteUDPAddr == nil {
		return conn.sendFastHeartbeats()
	} else if oldRemoteUDPAddr.String() != remoteUDPAddr.String() {
		log.Println("Peer", conn.remote.Name, "moved from", old, "to", remoteUDPAddr)
	}
	return nil
}

func (conn *LocalConnection) handleSetEstablished() error {
	stopTicker(conn.heartbeat)
	old := conn.established
	conn.Lock()
	conn.established = true
	conn.Unlock()
	if old {
		return nil
	}
	conn.Router.Ourself.ConnectionEstablished(conn)
	if err := conn.ensureForwarders(); err != nil {
		return err
	}
	// Send a large frame down the DF channel in order to prompt
	// PMTU discovery to start.
	conn.Forward(true, &ForwardedFrame{
		srcPeer: conn.local,
		dstPeer: conn.remote,
		frame:   PMTUDiscovery},
		nil)
	conn.heartbeat = time.NewTicker(SlowHeartbeat)
	conn.fragTest = time.NewTicker(FragTestInterval)
	// avoid initial waits for timers to fire
	conn.Forward(true, conn.heartbeatFrame, nil)
	conn.setStackFrag(false)
	if err := conn.handleSendSimpleProtocolMsg(ProtocolStartFragmentationTest); err != nil {
		return err
	}
	return nil
}

func (conn *LocalConnection) handleSendSimpleProtocolMsg(tag ProtocolTag) error {
	return conn.handleSendProtocolMsg(ProtocolMsg{tag: tag})
}

func (conn *LocalConnection) handleSendProtocolMsg(m ProtocolMsg) error {
	return conn.tcpSender.Send(Concat([]byte{byte(m.tag)}, m.msg))
}

func (conn *LocalConnection) handleShutdown() {
	if conn.TCPConn != nil {
		checkWarn(conn.TCPConn.Close())
	}

	if conn.remote != nil {
		conn.remote.DecrementLocalRefCount()
		conn.Router.Ourself.DeleteConnection(conn)
	}

	stopTicker(conn.heartbeat)
	stopTicker(conn.fragTest)

	// blank out the forwardChan so that the router processes don't
	// try to send any more
	conn.stopForwarders()

	conn.Router.ConnectionMaker.ConnectionTerminated(conn.remoteTCPAddr)
}

// Helpers

func (conn *LocalConnection) handshake(enc *gob.Encoder, dec *gob.Decoder, acceptNewPeer bool) error {
	// We do not need to worry about locking in here as at this point
	// the connection is not reachable by any go-routine other than
	// ourself. Only when we add this connection to the conn.local
	// peer will it be visible from multiple go-routines.

	conn.extendReadDeadline()

	localConnID := randUint64()
	versionStr := fmt.Sprint(ProtocolVersion)
	handshakeSend := map[string]string{
		"Protocol":        Protocol,
		"ProtocolVersion": versionStr,
		"PeerNameFlavour": PeerNameFlavour,
		"Name":            conn.local.Name.String(),
		"UID":             fmt.Sprint(conn.local.UID),
		"ConnID":          fmt.Sprint(localConnID)}
	handshakeRecv := map[string]string{}

	usingPassword := conn.Router.UsingPassword()
	var public, private *[32]byte
	var err error
	if usingPassword {
		public, private, err = GenerateKeyPair()
		if err != nil {
			return err
		}
		handshakeSend["PublicKey"] = hex.EncodeToString(public[:])
	}
	enc.Encode(handshakeSend)

	err = dec.Decode(&handshakeRecv)
	if err != nil {
		return err
	}
	_, err = checkHandshakeStringField("Protocol", Protocol, handshakeRecv)
	if err != nil {
		return err
	}
	_, err = checkHandshakeStringField("ProtocolVersion", versionStr, handshakeRecv)
	if err != nil {
		return err
	}
	_, err = checkHandshakeStringField("PeerNameFlavour", PeerNameFlavour, handshakeRecv)
	if err != nil {
		return err
	}
	nameStr, err := checkHandshakeStringField("Name", "", handshakeRecv)
	if err != nil {
		return err
	}
	name, err := PeerNameFromString(nameStr)
	if err != nil {
		return err
	}
	if !acceptNewPeer {
		if _, found := conn.Router.Peers.Fetch(name); !found {
			return fmt.Errorf("Found unknown remote name: %s at %s", name, conn.remoteTCPAddr)
		}
	}
	if existingConn, found := conn.local.ConnectionTo(name); found && existingConn.Established() {
		return fmt.Errorf("Already have connection to %s at %s", name, existingConn.RemoteTCPAddr())
	}

	uidStr, err := checkHandshakeStringField("UID", "", handshakeRecv)
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil {
		return err
	}

	remoteConnIdStr, err := checkHandshakeStringField("ConnID", "", handshakeRecv)
	if err != nil {
		return err
	}
	remoteConnID, err := strconv.ParseUint(remoteConnIdStr, 10, 64)
	if err != nil {
		return err
	}
	conn.uid = localConnID ^ remoteConnID

	if usingPassword {
		remotePublicStr, rpErr := checkHandshakeStringField("PublicKey", "", handshakeRecv)
		if rpErr != nil {
			return rpErr
		}
		remotePublicSlice, rpErr := hex.DecodeString(remotePublicStr)
		if rpErr != nil {
			return rpErr
		}
		remotePublic := [32]byte{}
		for idx, elem := range remotePublicSlice {
			remotePublic[idx] = elem
		}
		conn.SessionKey = FormSessionKey(&remotePublic, private, conn.Router.Password)
		conn.tcpSender = NewEncryptedTCPSender(enc, conn)
		conn.Decryptor = NewNaClDecryptor(conn)
	} else {
		if _, found := handshakeRecv["PublicKey"]; found {
			return fmt.Errorf("Remote network is encrypted. Password required.")
		}
		conn.tcpSender = NewSimpleTCPSender(enc)
		conn.Decryptor = NewNonDecryptor(conn)
	}

	toPeer := NewPeer(name, uid, 0)
	toPeer = conn.Router.Peers.FetchWithDefault(toPeer)
	switch toPeer {
	case nil:
		return fmt.Errorf("Connection appears to be with different version of a peer we already know of")
	case conn.local:
		conn.remote = toPeer // have to do assigment here to ensure Shutdown releases ref count
		return fmt.Errorf("Cannot connect to ourself")
	default:
		conn.remote = toPeer
		return nil
	}
}

func checkHandshakeStringField(fieldName string, expectedValue string, handshake map[string]string) (string, error) {
	val, found := handshake[fieldName]
	if !found {
		return "", fmt.Errorf("Field %s is missing", fieldName)
	}
	if expectedValue != "" && val != expectedValue {
		return "", fmt.Errorf("Field %s has wrong value; expected '%s', received '%s'", fieldName, expectedValue, val)
	}
	return val, nil
}

func (conn *LocalConnection) receiveTCP(decoder *gob.Decoder) {
	defer conn.Decryptor.Shutdown()
	usingPassword := conn.SessionKey != nil
	var receiver TCPReceiver
	if usingPassword {
		receiver = NewEncryptedTCPReceiver(conn)
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
			conn.log("ignoring blank msg")
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
	case ProtocolConnectionEstablished:
		// We sent fast heartbeats to the remote peer, which has now
		// received at least one of them and told us via this message.
		// We can now consider the connection as established from our
		// end.
		conn.SetEstablished()
	case ProtocolStartFragmentationTest:
		conn.Forward(false, &ForwardedFrame{
			srcPeer: conn.local,
			dstPeer: conn.remote,
			frame:   FragTest},
			nil)
	case ProtocolFragmentationReceived:
		conn.setStackFrag(true)
	case ProtocolNonce:
		if conn.SessionKey == nil {
			return fmt.Errorf("unexpected nonce on unencrypted connection")
		}
		conn.Decryptor.ReceiveNonce(payload)
	case ProtocolPMTUVerified:
		conn.verifyPMTU <- int(binary.BigEndian.Uint16(payload))
	case ProtocolGossipUnicast:
		return conn.Router.handleGossip(payload, deliverGossipUnicast)
	case ProtocolGossipBroadcast:
		return conn.Router.handleGossip(payload, deliverGossipBroadcast)
	case ProtocolGossip:
		return conn.Router.handleGossip(payload, deliverGossip)
	default:
		conn.log("ignoring unknown protocol tag:", tag)
	}
	return nil
}

func (conn *LocalConnection) extendReadDeadline() {
	conn.TCPConn.SetReadDeadline(time.Now().Add(ReadTimeout))
}

func (conn *LocalConnection) sendFastHeartbeats() error {
	err := conn.ensureForwarders()
	if err == nil {
		conn.heartbeat = time.NewTicker(FastHeartbeat)
		conn.Forward(true, conn.heartbeatFrame, nil) // avoid initial wait
	}
	return err
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
