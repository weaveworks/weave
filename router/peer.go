package router

import (
	"fmt"
	"log"
	"net"
	"sort"
)

func NewPeer(name PeerName, uid uint64, version uint64, router *Router) *Peer {
	if uid == 0 {
		uid = randUint64()
	}
	return &Peer{
		Name:        name,
		NameByte:    name.Bin(),
		connections: make(map[PeerName]Connection),
		version:     version,
		UID:         uid,
		Router:      router}
}

func (peer *Peer) Version() uint64 {
	peer.RLock()
	defer peer.RUnlock()
	return peer.version
}

func (peer *Peer) IncrementLocalRefCount() {
	peer.Lock()
	defer peer.Unlock()
	peer.localRefCount += 1
}

func (peer *Peer) DecrementLocalRefCount() {
	peer.Lock()
	defer peer.Unlock()
	peer.localRefCount -= 1
}

func (peer *Peer) IsLocallyReferenced() bool {
	peer.RLock()
	defer peer.RUnlock()
	return peer.localRefCount != 0
}

func (peer *Peer) ConnectionCount() int {
	peer.RLock()
	defer peer.RUnlock()
	return len(peer.connections)
}

func (peer *Peer) ConnectionTo(name PeerName) (Connection, bool) {
	peer.RLock()
	defer peer.RUnlock()
	conn, found := peer.connections[name]
	return conn, found // yes, you really can't inline that. FFS.
}

func (peer *Peer) ForEachConnection(fun func(PeerName, Connection)) {
	peer.RLock()
	defer peer.RUnlock()
	for name, conn := range peer.connections {
		fun(name, conn)
	}
}

// Calculate the routing table from this peer to all peers reachable
// from it, returning a "next hop" map of PeerNameX -> PeerNameY,
// which says "in order to send a message to X, the peer should send
// the message to its neighbour Y".
//
// Because currently we do not have weightings on the connections
// between peers, there is no need to use a minimum spanning tree
// algorithm. Instead we employ the simpler and cheaper breadth-first
// widening. The computation is deterministic, which ensures that when
// it is performed on the same data by different peers, they get the
// same result. This is important since otherwise we risk message loss
// or routing cycles.
//
// When the 'symmetric' flag is set, only symmetric connections are
// considered, i.e. where both sides indicate they have a connection
// to the other.
//
// When a non-nil stopAt peer is supplied, the widening stops when it
// reaches that peer. The boolean return indicates whether that has
// happened.
//
// We acquire read locks on peers as we encounter them during the
// traversal. This prevents the connectivity graph from changing
// underneath us in ways that would invalidate the result. Thus the
// answer returned may be out of date, but never inconsistent.
func (peer *Peer) Routes(stopAt *Peer, symmetric bool) (bool, map[PeerName]PeerName) {
	peer.RLock()
	defer peer.RUnlock()
	routes := make(map[PeerName]PeerName)
	routes[peer.Name] = UnknownPeerName
	nextWorklist := []*Peer{peer}
	for len(nextWorklist) > 0 {
		worklist := nextWorklist
		sort.Sort(ListOfPeers(worklist))
		nextWorklist = []*Peer{}
		for _, curPeer := range worklist {
			if curPeer == stopAt {
				return true, routes
			}
			curName := curPeer.Name
			for remoteName, conn := range curPeer.connections {
				if _, found := routes[remoteName]; found {
					continue
				}
				remote := conn.Remote()
				remote.RLock()
				if _, found := remote.connections[curName]; !symmetric || found {
					defer remote.RUnlock()
					nextWorklist = append(nextWorklist, remote)
					// We now know how to get to remoteName: the same
					// way we get to curPeer. Except, if curPeer is
					// the starting peer in which case we know we can
					// reach remoteName directly.
					if curPeer == peer {
						routes[remoteName] = remoteName
					} else {
						routes[remoteName] = routes[curName]
					}
				} else {
					remote.RUnlock()
				}
			}
		}
	}
	return false, routes
}

func (peer *Peer) SetVersionAndConnections(version uint64, connections map[PeerName]Connection) {
	if peer == peer.Router.Ourself {
		log.Fatal("Attempt to set version and connections on the local peer", peer.Name)
	}
	peer.Lock()
	defer peer.Unlock()
	peer.version = version
	peer.connections = connections
}

func (peer *Peer) Forward(dstPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.Relay(peer, dstPeer, df, frame, dec)
}

func (peer *Peer) Broadcast(df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.RelayBroadcast(peer, df, frame, dec)
}

func (peer *Peer) Relay(srcPeer, dstPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	relayPeerName, found := peer.Router.Topology.Unicast(dstPeer.Name)
	if !found {
		// Not necessarily an error as there could be a race with the
		// dst disappearing whilst the frame is in flight
		log.Println("Received packet for unknown destination:", dstPeer.Name)
		return nil
	}
	conn, found := peer.ConnectionTo(relayPeerName)
	if !found {
		// Again, could just be a race, not necessarily an error
		log.Println("Unable to find connection to relay peer", relayPeerName)
		return nil
	}
	return conn.(*LocalConnection).Forward(df, &ForwardedFrame{
		srcPeer: srcPeer,
		dstPeer: dstPeer,
		frame:   frame},
		dec)
}

func (peer *Peer) CallBroadcastFunc(srcPeer *Peer, fn func(conn *LocalConnection) error) error {
	nextHops := peer.Router.Topology.Broadcast(srcPeer.Name)
	if len(nextHops) == 0 {
		return nil
	}
	// We must not hold a read lock on peer during the conn.Forward
	// below, since that is a potentially blocking operation (e.g. if
	// the channel is full).
	nextConns := make([]*LocalConnection, 0, len(nextHops))
	peer.RLock()
	for _, hopName := range nextHops {
		conn, found := peer.connections[hopName]
		// Again, !found could just be due to a race.
		if found {
			nextConns = append(nextConns, conn.(*LocalConnection))
		}
	}
	peer.RUnlock()
	for _, conn := range nextConns {
		if err := fn(conn); err != nil {
			return err
		}
	}
	return nil
}

func (peer *Peer) RelayBroadcast(srcPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.CallBroadcastFunc(srcPeer, func(conn *LocalConnection) error {
		err := conn.Forward(df, &ForwardedFrame{
			srcPeer: srcPeer,
			dstPeer: conn.Remote(),
			frame:   frame},
			dec)
		if err != nil {
			return err
		}
		return nil
	})
}

func (peer *Peer) Gossip() {
	buf := peer.Router.GossipDelegate.LocalState(false)
	msg := Concat([]byte{ProtocolGossipBroadcast}, buf)
	peer.RelayGossip(peer.Name, msg)
}

func (peer *Peer) RelayGossip(srcName PeerName, msg []byte) {
	log.Println("Have been asked to relay gossip", len(msg), "bytes from", srcName)
	if srcPeer, found := peer.Router.Peers.Fetch(srcName); found {
		peer.CallBroadcastFunc(srcPeer, func(conn *LocalConnection) error {
			log.Println("about to gossip", len(msg), "bytes on", conn)
			conn.SendTCP(msg)
			return nil
		})
	} else {
		log.Println("Unable to relay gossip from unknown peer", srcName)
	}
}

func (peer *Peer) GossipBroadcastOn(conn *LocalConnection, buf []byte) {
	log.Println("GossipBroadcastOn", conn, len(buf), "bytes")
	peerName := peer.Name.Bin()
	nameLenByte := []byte{byte(len(peerName))}
	msg := Concat([]byte{ProtocolGossipBroadcast}, []byte{GossipVersion}, nameLenByte, peerName, buf)
	conn.SendTCP(msg)
}

func (peer *Peer) GossipSendTo(dstPeer *Peer, buf []byte) {
	log.Println("GossipSendTo", len(buf), "bytes to", dstPeer)
	peerName := peer.Name.Bin()
	nameLenByte := []byte{byte(len(peerName))}
	msg := Concat([]byte{ProtocolGossipUnicast}, []byte{GossipVersion}, nameLenByte, peerName, buf)
	peer.RelayGossipTo(peer.Name, dstPeer.Name, msg)
}

func (peer *Peer) RelayGossipTo(srcPeerName, dstPeerName PeerName, msg []byte) error {
	relayPeerName, found := peer.Router.Topology.Unicast(dstPeerName)
	if !found {
		log.Println("Received gossip for unknown destination:", dstPeerName)
		return nil
	}
	conn, found := peer.ConnectionTo(relayPeerName)
	if !found {
		log.Println("Gossip: Unable to find connection to relay peer", relayPeerName)
		return nil
	}
	log.Println("relaying gossip", len(msg), "bytes on", conn)
	conn.(*LocalConnection).SendTCP(msg)
	return nil
}

func (peer *Peer) String() string {
	peer.RLock()
	defer peer.RUnlock()
	return fmt.Sprint("Peer ", peer.Name, " (v", peer.version, ") (UID ", peer.UID, ")")
}

func (peer *Peer) StartLocalPeer() {
	if peer.Router.Ourself != peer {
		log.Fatal("Attempt to start peer which is not the local peer")
	}
	queryChan := make(chan *PeerInteraction, ChannelSize)
	peer.queryChan = queryChan
	go peer.queryLoop(queryChan)
}

func (peer *Peer) CreateConnection(peerAddr string, acceptNewPeer bool) error {
	if err := peer.checkConnectionLimit(); err != nil {
		return err
	}
	// We're dialing the remote so that means connections will come from random ports
	addrStr := NormalisePeerAddr(peerAddr)
	tcpAddr, tcpErr := net.ResolveTCPAddr("tcp4", addrStr)
	udpAddr, udpErr := net.ResolveUDPAddr("udp4", addrStr)
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
	connRemote := NewRemoteConnection(peer, nil, tcpConn.RemoteAddr().String())
	NewLocalConnection(connRemote, acceptNewPeer, tcpConn, udpAddr, peer.Router)
	return nil
}

// ACTOR client API

const (
	PAddConnection         = iota
	PBroadcastTCP          = iota
	PDeleteConnection      = iota
	PConnectionEstablished = iota
)

// Async: rely on the peer to shut us down if we shouldn't be adding
// ourselves, so therefore this can be async
func (peer *Peer) AddConnection(conn *LocalConnection) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PAddConnection},
		payload:     conn}
}

// Sync.
func (peer *Peer) DeleteConnection(conn *LocalConnection) {
	resultChan := make(chan interface{})
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PDeleteConnection, resultChan: resultChan},
		payload:     conn}
	<-resultChan
}

// Async.
func (peer *Peer) ConnectionEstablished(conn *LocalConnection) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PConnectionEstablished},
		payload:     conn}
}

// Async.
func (peer *Peer) BroadcastTCP(msg []byte) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PBroadcastTCP},
		payload:     msg}
}

// ACTOR server

func (peer *Peer) queryLoop(queryChan <-chan *PeerInteraction) {
	for {
		query, ok := <-queryChan
		if !ok {
			return
		}
		switch query.code {
		case PAddConnection:
			peer.handleAddConnection(query.payload.(*LocalConnection))
		case PDeleteConnection:
			peer.handleDeleteConnection(query.payload.(*LocalConnection))
			query.resultChan <- nil
		case PConnectionEstablished:
			peer.handleConnectionEstablished(query.payload.(*LocalConnection))
		case PBroadcastTCP:
			peer.handleBroadcastTCP(query.payload.([]byte))
		}
	}
}

func (peer *Peer) handleAddConnection(conn *LocalConnection) {
	if peer != conn.Local() {
		log.Fatal("Attempt made to add connection to peer where peer is not the source of connection")
	}
	if conn.Remote() == nil {
		log.Fatal("Attempt made to add connection to peer with unknown remote peer")
	}
	toName := conn.Remote().Name
	dupErr := fmt.Errorf("Multiple connections to %s added to %s", toName, peer.Name)
	// deliberately non symmetrical
	if dupConn, found := peer.connections[toName]; found {
		if dupConn == conn {
			return
		}
		// conn.UID is used as the tie breaker here, in the
		// knowledge that both sides will make the same decision.
		dupConnLocal := dupConn.(*LocalConnection)
		if conn.UID == dupConnLocal.UID {
			// oh good grief. Sod it, just kill both of them.
			conn.CheckFatal(dupErr)
			dupConnLocal.CheckFatal(dupErr)
			peer.handleDeleteConnection(dupConnLocal)
			return
		} else if conn.UID < dupConnLocal.UID {
			dupConnLocal.CheckFatal(dupErr)
			peer.handleDeleteConnection(dupConnLocal)
		} else {
			conn.CheckFatal(dupErr)
			return
		}
	}
	if err := peer.checkConnectionLimit(); err != nil {
		conn.CheckFatal(err)
		return
	}
	peer.Lock()
	peer.connections[toName] = conn
	peer.Unlock()
}

func (peer *Peer) handleDeleteConnection(conn *LocalConnection) {
	if peer != conn.Local() {
		log.Fatal("Attempt made to delete connection from peer where peer is not the source of connection")
	}
	if conn.Remote() == nil {
		log.Fatal("Attempt made to delete connection to peer with unknown remote peer")
	}
	toName := conn.Remote().Name
	if connFound, found := peer.connections[toName]; !found || connFound != conn {
		return
	}
	peer.Lock()
	delete(peer.connections, toName)
	peer.Unlock()
	broadcast := false
	if conn.Established() {
		peer.Lock()
		peer.version += 1
		peer.Unlock()
		broadcast = true
	}
	// Must do garbage collection first to ensure we don't send out an
	// update with unreachable peers (can cause looping)
	peer.Router.Peers.GarbageCollect(peer.Router)
	if broadcast {
		peer.broadcastPeerUpdate()
	}
}

func (peer *Peer) handleConnectionEstablished(conn *LocalConnection) {
	if peer != conn.Local() {
		log.Fatal("Peer informed of active connection where peer is not the source of connection")
	}
	if dupConn, found := peer.connections[conn.Remote().Name]; !found || conn != dupConn {
		conn.CheckFatal(fmt.Errorf("Cannot set unknown connection active"))
		return
	}
	peer.Lock()
	peer.version += 1
	peer.Unlock()
	log.Println("Peer", peer.Name, "established active connection to remote peer", conn.Remote().Name, "at", conn.RemoteTCPAddr())
	peer.broadcastPeerUpdate(conn.Remote())

	// Send new friend our state
	buf := peer.Router.GossipDelegate.LocalState(true)
	peer.GossipBroadcastOn(conn, buf)
}

func (peer *Peer) handleBroadcastTCP(msg []byte) {
	peer.Router.Topology.RebuildRoutes()
	peer.ForEachConnection(func(_ PeerName, conn Connection) {
		conn.(*LocalConnection).SendTCP(msg)
	})
}

func (peer *Peer) broadcastPeerUpdate(peers ...*Peer) {
	peer.handleBroadcastTCP(Concat(ProtocolUpdateByte, EncodePeers(append(peers, peer)...)))
}

func (peer *Peer) checkConnectionLimit() error {
	if 0 != peer.Router.ConnLimit && peer.ConnectionCount() >= peer.Router.ConnLimit {
		return fmt.Errorf("Connection limit reached (%v)", peer.Router.ConnLimit)
	}
	return nil
}
