package router

import (
	"fmt"
	"log"
	"net"
	"sort"
)

func NewPeer(name PeerName, uid uint64, version uint64) *Peer {
	if uid == 0 {
		uid = randUint64()
	}
	return &Peer{
		Name:        name,
		NameByte:    name.Bin(),
		UID:         uid,
		version:     version,
		connections: make(map[PeerName]Connection)}
}

func (peer *Peer) String() string {
	peer.RLock()
	defer peer.RUnlock()
	return fmt.Sprint("Peer ", peer.Name, " (v", peer.version, ") (UID ", peer.UID, ")")
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

func (peer *Peer) SetVersionAndConnections(version uint64, connections map[PeerName]Connection) {
	peer.Lock()
	defer peer.Unlock()
	peer.version = version
	peer.connections = connections
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

func StartLocalPeer(name PeerName, router *Router) *LocalPeer {
	peer := &LocalPeer{Peer: NewPeer(name, 0, 0), Router: router}
	queryChan := make(chan *PeerInteraction, ChannelSize)
	peer.queryChan = queryChan
	go peer.queryLoop(queryChan)
	return peer
}

func (peer *LocalPeer) Forward(dstPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.Relay(peer.Peer, dstPeer, df, frame, dec)
}

func (peer *LocalPeer) Broadcast(df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.RelayBroadcast(peer.Peer, df, frame, dec)
}

func (peer *LocalPeer) Relay(srcPeer, dstPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	relayPeerName, found := peer.Router.Routes.Unicast(dstPeer.Name)
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

func (peer *LocalPeer) RelayBroadcast(srcPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	nextHops := peer.Router.Routes.Broadcast(srcPeer.Name)
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
	var err error
	for _, conn := range nextConns {
		err = conn.Forward(df, &ForwardedFrame{
			srcPeer: srcPeer,
			dstPeer: conn.Remote(),
			frame:   frame},
			dec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (peer *LocalPeer) CreateConnection(peerAddr string, acceptNewPeer bool) error {
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
	connRemote := NewRemoteConnection(peer.Peer, nil, tcpConn.RemoteAddr().String())
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
func (peer *LocalPeer) AddConnection(conn *LocalConnection) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PAddConnection},
		payload:     conn}
}

// Sync.
func (peer *LocalPeer) DeleteConnection(conn *LocalConnection) {
	resultChan := make(chan interface{})
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PDeleteConnection, resultChan: resultChan},
		payload:     conn}
	<-resultChan
}

// Async.
func (peer *LocalPeer) ConnectionEstablished(conn *LocalConnection) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PConnectionEstablished},
		payload:     conn}
}

// Async.
func (peer *LocalPeer) BroadcastTCP(msg []byte) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PBroadcastTCP},
		payload:     msg}
}

// ACTOR server

func (peer *LocalPeer) queryLoop(queryChan <-chan *PeerInteraction) {
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

func (peer *LocalPeer) handleAddConnection(conn *LocalConnection) {
	if peer.Peer != conn.Local() {
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

func (peer *LocalPeer) handleDeleteConnection(conn *LocalConnection) {
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
	peer.Router.Peers.GarbageCollect()
	if broadcast {
		peer.broadcastPeerUpdate()
	}
}

func (peer *LocalPeer) handleConnectionEstablished(conn *LocalConnection) {
	if peer.Peer != conn.Local() {
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
}

func (peer *LocalPeer) handleBroadcastTCP(msg []byte) {
	peer.ForEachConnection(func(_ PeerName, conn Connection) {
		conn.(*LocalConnection).SendTCP(msg)
	})
}

func (peer *LocalPeer) broadcastPeerUpdate(peers ...*Peer) {
	peer.Router.Routes.Recalculate()
	peer.handleBroadcastTCP(Concat(ProtocolUpdateByte, EncodePeers(append(peers, peer.Peer)...)))
}

func (peer *LocalPeer) checkConnectionLimit() error {
	if 0 != peer.Router.ConnLimit && peer.ConnectionCount() >= peer.Router.ConnLimit {
		return fmt.Errorf("Connection limit reached (%v)", peer.Router.ConnLimit)
	}
	return nil
}
