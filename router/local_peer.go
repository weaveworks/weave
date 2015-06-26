package router

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type LocalPeer struct {
	sync.RWMutex
	*Peer
	router     *Router
	actionChan chan<- LocalPeerAction
}

type LocalPeerAction func()

func NewLocalPeer(name PeerName, nickName string, router *Router) *LocalPeer {
	actionChan := make(chan LocalPeerAction, ChannelSize)
	peer := &LocalPeer{
		Peer:       NewPeer(name, nickName, 0, 0),
		router:     router,
		actionChan: actionChan,
	}
	go peer.actorLoop(actionChan)
	return peer
}

func (peer *LocalPeer) Forward(dstPeer *Peer, frame []byte, dec *EthernetDecoder) error {
	return peer.Relay(peer.Peer, dstPeer, frame, dec)
}

func (peer *LocalPeer) Broadcast(frame []byte, dec *EthernetDecoder) {
	peer.RelayBroadcast(peer.Peer, frame, dec)
}

func (peer *LocalPeer) Relay(srcPeer, dstPeer *Peer, frame []byte, dec *EthernetDecoder) error {
	relayPeerName, found := peer.router.Routes.Unicast(dstPeer.Name)
	if !found {
		// Not necessarily an error as there could be a race with the
		// dst disappearing whilst the frame is in flight
		log.Println("Received packet for unknown destination:", dstPeer)
		return nil
	}
	conn, found := peer.ConnectionTo(relayPeerName)
	if !found {
		// Again, could just be a race, not necessarily an error
		log.Println("Unable to find connection to relay peer", relayPeerName)
		return nil
	}
	return conn.(*LocalConnection).Forward(&ForwardedFrame{
		srcPeer: srcPeer,
		dstPeer: dstPeer,
		frame:   frame},
		dec)
}

func (peer *LocalPeer) RelayBroadcast(srcPeer *Peer, frame []byte, dec *EthernetDecoder) {
	nextHops := peer.router.Routes.Broadcast(srcPeer.Name)
	if len(nextHops) == 0 {
		return
	}
	for _, conn := range peer.ConnectionsTo(nextHops) {
		err := conn.(*LocalConnection).Forward(&ForwardedFrame{
			srcPeer: srcPeer,
			dstPeer: conn.Remote(),
			frame:   frame},
			dec)
		if err != nil {
			if ftbe, ok := err.(FrameTooBigError); ok {
				log.Printf("dropping too big DF broadcast frame (%v -> %v): PMTU= %v\n", dec.IP.DstIP, dec.IP.SrcIP, ftbe.EPMTU)
			} else {
				log.Println(err)
			}
		}
	}
}

func (peer *LocalPeer) Info() string {
	peer.RLock()
	defer peer.RUnlock()
	return peer.Peer.Info()
}

func (peer *LocalPeer) Connections() ConnectionSet {
	connections := make(ConnectionSet)
	peer.RLock()
	defer peer.RUnlock()
	for _, conn := range peer.connections {
		connections[conn] = void
	}
	return connections
}

func (peer *LocalPeer) ConnectionTo(name PeerName) (Connection, bool) {
	peer.RLock()
	defer peer.RUnlock()
	conn, found := peer.connections[name]
	return conn, found // yes, you really can't inline that. FFS.
}

func (peer *LocalPeer) ConnectionsTo(names []PeerName) []Connection {
	conns := make([]Connection, 0, len(names))
	peer.RLock()
	defer peer.RUnlock()
	for _, name := range names {
		conn, found := peer.connections[name]
		// Again, !found could just be due to a race.
		if found {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (peer *LocalPeer) CreateConnection(peerAddr string, acceptNewPeer bool) error {
	if err := peer.checkConnectionLimit(); err != nil {
		return err
	}
	tcpAddr, tcpErr := net.ResolveTCPAddr("tcp4", peerAddr)
	udpAddr, udpErr := net.ResolveUDPAddr("udp4", peerAddr)
	if tcpErr != nil || udpErr != nil {
		// they really should have the same value, but just in case...
		if tcpErr == nil {
			return udpErr
		}
		return tcpErr
	}
	tcpConn, err := net.DialTCP("tcp4", nil, tcpAddr)
	if err != nil {
		return err
	}
	connRemote := NewRemoteConnection(peer.Peer, nil, tcpConn.RemoteAddr().String(), true, false)
	StartLocalConnection(connRemote, tcpConn, udpAddr, peer.router, acceptNewPeer)
	return nil
}

// ACTOR client API

// Sync.
func (peer *LocalPeer) AddConnection(conn *LocalConnection) error {
	resultChan := make(chan error)
	peer.actionChan <- func() {
		resultChan <- peer.handleAddConnection(conn)
	}
	return <-resultChan
}

// Async.
func (peer *LocalPeer) ConnectionEstablished(conn *LocalConnection) {
	peer.actionChan <- func() {
		peer.handleConnectionEstablished(conn)
	}
}

// Sync.
func (peer *LocalPeer) DeleteConnection(conn *LocalConnection) {
	resultChan := make(chan interface{})
	peer.actionChan <- func() {
		peer.handleDeleteConnection(conn)
		resultChan <- nil
	}
	<-resultChan
}

// ACTOR server

func (peer *LocalPeer) actorLoop(actionChan <-chan LocalPeerAction) {
	gossipTimer := time.Tick(GossipInterval)
	for {
		select {
		case action := <-actionChan:
			action()
		case <-gossipTimer:
			peer.router.SendAllGossip()
		}
	}
}

func (peer *LocalPeer) handleAddConnection(conn Connection) error {
	if peer.Peer != conn.Local() {
		log.Fatal("Attempt made to add connection to peer where peer is not the source of connection")
	}
	if conn.Remote() == nil {
		log.Fatal("Attempt made to add connection to peer with unknown remote peer")
	}
	toName := conn.Remote().Name
	dupErr := fmt.Errorf("Multiple connections to %s added to %s", conn.Remote(), peer.String())
	// deliberately non symmetrical
	if dupConn, found := peer.connections[toName]; found {
		if dupConn == conn {
			return nil
		}
		switch conn.BreakTie(dupConn) {
		case TieBreakWon:
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
		case TieBreakLost:
			return dupErr
		case TieBreakTied:
			// oh good grief. Sod it, just kill both of them.
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
			return dupErr
		}
	}
	if err := peer.checkConnectionLimit(); err != nil {
		return err
	}
	_, isConnectedPeer := peer.router.Routes.Unicast(toName)
	peer.addConnection(conn)
	if isConnectedPeer {
		conn.Log("connection added")
	} else {
		conn.Log("connection added (new peer)")
		peer.router.SendAllGossipDown(conn)
	}
	peer.broadcastPeerUpdate(conn.Remote())
	return nil
}

func (peer *LocalPeer) handleConnectionEstablished(conn Connection) {
	if peer.Peer != conn.Local() {
		log.Fatal("Peer informed of active connection where peer is not the source of connection")
	}
	if dupConn, found := peer.connections[conn.Remote().Name]; !found || conn != dupConn {
		conn.Shutdown(fmt.Errorf("Cannot set unknown connection active"))
		return
	}
	peer.connectionEstablished(conn)
	conn.Log("connection fully established")
	peer.broadcastPeerUpdate()
}

func (peer *LocalPeer) handleDeleteConnection(conn Connection) {
	if peer.Peer != conn.Local() {
		log.Fatal("Attempt made to delete connection from peer where peer is not the source of connection")
	}
	if conn.Remote() == nil {
		log.Fatal("Attempt made to delete connection to peer with unknown remote peer")
	}
	toName := conn.Remote().Name
	if connFound, found := peer.connections[toName]; !found || connFound != conn {
		return
	}
	peer.deleteConnection(conn)
	conn.Log("connection deleted")
	// Must do garbage collection first to ensure we don't send out an
	// update with unreachable peers (can cause looping)
	peer.router.Peers.GarbageCollect()
	peer.broadcastPeerUpdate()
}

// helpers

func (peer *LocalPeer) broadcastPeerUpdate(peers ...*Peer) {
	peer.router.Routes.Recalculate()
	peer.router.TopologyGossip.GossipBroadcast(NewTopologyGossipData(peer.router.Peers, append(peers, peer.Peer)...))
}

func (peer *LocalPeer) checkConnectionLimit() error {
	limit := peer.router.ConnLimit
	if 0 != limit && peer.connectionCount() >= limit {
		return fmt.Errorf("Connection limit reached (%v)", limit)
	}
	return nil
}

func (peer *LocalPeer) addConnection(conn Connection) {
	peer.Lock()
	defer peer.Unlock()
	peer.connections[conn.Remote().Name] = conn
	peer.version++
}

func (peer *LocalPeer) deleteConnection(conn Connection) {
	peer.Lock()
	defer peer.Unlock()
	delete(peer.connections, conn.Remote().Name)
	peer.version++
}

func (peer *LocalPeer) connectionEstablished(conn Connection) {
	peer.Lock()
	defer peer.Unlock()
	peer.version++
}

func (peer *LocalPeer) connectionCount() int {
	peer.RLock()
	defer peer.RUnlock()
	return len(peer.connections)
}
