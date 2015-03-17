package router

import (
	"fmt"
	"log"
	"net"
	"time"
)

type LocalPeer struct {
	*Peer
	Router    *Router
	queryChan chan<- *PeerInteraction
}

type PeerInteraction struct {
	Interaction
	payload interface{}
}

func NewLocalPeer(name PeerName, nickName string, router *Router) *LocalPeer {
	return &LocalPeer{Peer: NewPeer(name, nickName, 0, 0), Router: router}
}

func (peer *LocalPeer) Start() {
	queryChan := make(chan *PeerInteraction, ChannelSize)
	peer.queryChan = queryChan
	go peer.queryLoop(queryChan)
}

func (peer *LocalPeer) Forward(dstPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	return peer.Relay(peer.Peer, dstPeer, df, frame, dec)
}

func (peer *LocalPeer) Broadcast(df bool, frame []byte, dec *EthernetDecoder) {
	peer.RelayBroadcast(peer.Peer, df, frame, dec)
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

func (peer *LocalPeer) RelayBroadcast(srcPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) {
	for _, conn := range peer.NextBroadcastHops(srcPeer) {
		err := conn.Forward(df, &ForwardedFrame{
			srcPeer: srcPeer,
			dstPeer: conn.Remote(),
			frame:   frame},
			dec)
		if err != nil {
			if ftbe, ok := err.(FrameTooBigError); ok {
				log.Printf("dropping too big DF broadcast frame (%v -> %v): PMTU= %v\n", dec.ip.DstIP, dec.ip.SrcIP, ftbe.EPMTU)
			} else {
				log.Println(err)
			}
		}
	}
}

func (peer *LocalPeer) NextBroadcastHops(srcPeer *Peer) []*LocalConnection {
	nextHops := peer.Router.Routes.Broadcast(srcPeer.Name)
	if len(nextHops) == 0 {
		return nil
	}
	nextConns := make([]*LocalConnection, 0, len(nextHops))
	peer.RLock()
	defer peer.RUnlock()
	for _, hopName := range nextHops {
		conn, found := peer.connections[hopName]
		// Again, !found could just be due to a race.
		if found {
			nextConns = append(nextConns, conn.(*LocalConnection))
		}
	}
	return nextConns
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
	connRemote := NewRemoteConnection(peer.Peer, nil, tcpConn.RemoteAddr().String(), true, false)
	connLocal := NewLocalConnection(connRemote, tcpConn, udpAddr, peer.Router)
	connLocal.Start(acceptNewPeer)
	return nil
}

// ACTOR client API

const (
	PAddConnection = iota
	PConnectionEstablished
	PDeleteConnection
)

// Sync.
func (peer *LocalPeer) AddConnection(conn *LocalConnection) {
	resultChan := make(chan interface{})
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PAddConnection, resultChan: resultChan},
		payload:     conn}
	<-resultChan
}

// Async.
func (peer *LocalPeer) ConnectionEstablished(conn *LocalConnection) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PConnectionEstablished},
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

// ACTOR server

func (peer *LocalPeer) queryLoop(queryChan <-chan *PeerInteraction) {
	gossipTimer := time.Tick(GossipInterval)
	for {
		select {
		case query, ok := <-queryChan:
			if !ok {
				return
			}
			switch query.code {
			case PAddConnection:
				peer.handleAddConnection(query.payload.(*LocalConnection))
				query.resultChan <- nil
			case PConnectionEstablished:
				peer.handleConnectionEstablished(query.payload.(*LocalConnection))
			case PDeleteConnection:
				peer.handleDeleteConnection(query.payload.(*LocalConnection))
				query.resultChan <- nil
			}
		case <-gossipTimer:
			peer.Router.SendAllGossip()
		}
	}
}

func (peer *LocalPeer) handleAddConnection(conn Connection) {
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
		switch conn.BreakTie(dupConn) {
		case TieBreakWon:
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
		case TieBreakLost:
			conn.Shutdown(dupErr)
			return
		case TieBreakTied:
			// oh good grief. Sod it, just kill both of them.
			conn.Shutdown(dupErr)
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
			return
		}
	}
	if err := peer.checkConnectionLimit(); err != nil {
		conn.Shutdown(err)
		return
	}
	_, isConnectedPeer := peer.Router.Routes.Unicast(toName)
	peer.addConnection(conn)
	if isConnectedPeer {
		conn.Log("connection added")
	} else {
		conn.Log("connection added (new peer)")
		peer.Router.SendAllGossipDown(conn)
	}
	peer.broadcastPeerUpdate(conn.Remote())
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
	peer.Router.Peers.GarbageCollect()
	peer.broadcastPeerUpdate()
}

func (peer *LocalPeer) broadcastPeerUpdate(peers ...*Peer) {
	peer.Router.Routes.Recalculate()
	keys := peerNameSet{peer.Name: true} // create a set including ourself plus anything passed in
	for _, p := range peers {
		keys[p.Name] = true
	}
	// TODO We should just be invoking TopologyGossip.GossipBroadcast
	// here, but route calculation is asynchronous and in this
	// particular case would likely result in the broadcast not
	// reaching all peers. So instead we slightly break the Gossip
	// abstraction (hence the cast) and send a regular update. This is
	// less efficient though since it will almost certainly reach
	// peers more than once.
	peer.Router.TopologyGossip.(*GossipChannel).SendGossipUpdateFor(keys)
}

func (peer *LocalPeer) checkConnectionLimit() error {
	if 0 != peer.Router.ConnLimit && peer.ConnectionCount() >= peer.Router.ConnLimit {
		return fmt.Errorf("Connection limit reached (%v)", peer.Router.ConnLimit)
	}
	return nil
}
