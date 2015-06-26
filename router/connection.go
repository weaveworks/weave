package router

import (
	"encoding/binary"
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
	TCPConn           *net.TCPConn
	version           byte
	tcpSender         TCPSender
	remoteUDPAddr     *net.UDPAddr
	receivedHeartbeat bool
	stackFrag         bool
	effectivePMTU     int
	SessionKey        *[32]byte
	heartbeatTCP      *time.Ticker
	heartbeatTimeout  *time.Timer
	heartbeatFrame    []byte
	heartbeat         *time.Ticker
	fragTest          *time.Ticker
	forwarder         *Forwarder
	forwarderDF       *ForwarderDF
	Decryptor         Decryptor
	Router            *Router
	uid               uint64
	actionChan        chan<- ConnectionAction
	finished          <-chan struct{} // closed to signal that actorLoop has finished
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
		effectivePMTU:    DefaultPMTU,
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
		conn.Log("Effective PMTU set to", pmtu)
	}
}

// Called by the connection's actor process, and by the connection's
// TCP receiver process. StackFrag is read in conn.forward
func (conn *LocalConnection) setStackFrag(frag bool) {
	conn.Lock()
	defer conn.Unlock()
	conn.stackFrag = frag
}

// Called by the connection's TCP receiver process.
func (conn *LocalConnection) pmtuVerified(pmtu int) {
	conn.RLock()
	fwd := conn.forwarderDF
	conn.RUnlock()
	if fwd != nil {
		fwd.PMTUVerified(pmtu)
	}
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

// Async
//
// Heartbeating serves two purposes: a) keeping NAT paths alive, and
// b) updating a remote peer's knowledge of our address, in the event
// it changes (e.g. because NAT paths expired).
func (conn *LocalConnection) ReceivedHeartbeat(remoteUDPAddr *net.UDPAddr, connUID uint64) {
	if remoteUDPAddr == nil || connUID != conn.uid {
		return
	}
	conn.sendAction(func() error {
		oldRemoteUDPAddr := conn.remoteUDPAddr
		old := conn.receivedHeartbeat
		conn.Lock()
		conn.remoteUDPAddr = remoteUDPAddr
		conn.receivedHeartbeat = true
		conn.Unlock()
		conn.heartbeatTimeout.Reset(HeartbeatTimeout)
		if !old {
			if err := conn.sendSimpleProtocolMsg(ProtocolConnectionEstablished); err != nil {
				return err
			}
		}
		if oldRemoteUDPAddr == nil {
			return conn.sendFastHeartbeats()
		} else if oldRemoteUDPAddr.String() != remoteUDPAddr.String() {
			log.Println("Peer", conn.remote, "moved from", old, "to", remoteUDPAddr)
		}
		return nil
	})
}

// Async
func (conn *LocalConnection) SetEstablished() {
	conn.sendAction(func() error {
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
		conn.Send(true, PMTUDiscovery)
		conn.heartbeat = time.NewTicker(SlowHeartbeat)
		conn.fragTest = time.NewTicker(FragTestInterval)
		// avoid initial waits for timers to fire
		conn.Send(true, conn.heartbeatFrame)
		conn.performFragTest()
		return nil
	})
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

	conn.TCPConn.SetLinger(0)

	_, dec, err := conn.handshake(acceptNewPeer)
	if err != nil {
		return
	}
	conn.Log("completed handshake; using protocol version", conn.version)

	// The ordering of the following is very important. [1]

	if conn.remoteUDPAddr != nil {
		if err = conn.ensureForwarders(); err != nil {
			return
		}
	}
	if err = conn.Router.Ourself.AddConnection(conn); err != nil {
		return
	}
	if err = conn.initHeartbeats(); err != nil {
		return
	}
	go conn.receiveTCP(dec)
	err = conn.actorLoop(actionChan)
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

func (conn *LocalConnection) initHeartbeats() error {
	conn.heartbeatTCP = time.NewTicker(TCPHeartbeat)
	conn.heartbeatTimeout = time.NewTimer(HeartbeatTimeout)
	conn.heartbeatFrame = make([]byte, EthernetOverhead+8)
	binary.BigEndian.PutUint64(conn.heartbeatFrame[EthernetOverhead:], conn.uid)
	if conn.remoteUDPAddr == nil {
		return nil
	}
	return conn.sendFastHeartbeats()
}

func (conn *LocalConnection) actorLoop(actionChan <-chan ConnectionAction) (err error) {
	for err == nil {
		select {
		case action := <-actionChan:
			err = action()
		case <-conn.heartbeatTCP.C:
			err = conn.sendSimpleProtocolMsg(ProtocolHeartbeat)
		case <-conn.heartbeatTimeout.C:
			err = fmt.Errorf("timed out waiting for UDP heartbeat")
		case <-tickerChan(conn.heartbeat):
			conn.Send(true, conn.heartbeatFrame)
		case <-tickerChan(conn.fragTest):
			conn.performFragTest()
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

	if conn.heartbeatTimeout != nil {
		conn.heartbeatTimeout.Stop()
	}

	stopTicker(conn.heartbeatTCP)
	stopTicker(conn.heartbeat)
	stopTicker(conn.fragTest)

	// blank out the forwardChan so that the router processes don't
	// try to send any more
	conn.stopForwarders()

	conn.Router.ConnectionMaker.ConnectionTerminated(conn.remoteTCPAddr, err)
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
	case ProtocolConnectionEstablished:
		// We sent fast heartbeats to the remote peer, which has now
		// received at least one of them and told us via this message.
		// We can now consider the connection as established from our
		// end.
		conn.SetEstablished()
	case ProtocolFragmentationReceived:
		conn.setStackFrag(true)
	case ProtocolPMTUVerified:
		conn.pmtuVerified(int(binary.BigEndian.Uint16(payload)))
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

func (conn *LocalConnection) sendFastHeartbeats() error {
	err := conn.ensureForwarders()
	if err == nil {
		conn.heartbeat = time.NewTicker(FastHeartbeat)
		conn.Send(true, conn.heartbeatFrame) // avoid initial wait
	}
	return err
}

func (conn *LocalConnection) performFragTest() {
	conn.setStackFrag(false)
	conn.Send(false, FragTest)
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
