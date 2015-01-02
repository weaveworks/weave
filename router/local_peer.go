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

func (peer *LocalPeer) NextBroadcastHops(srcPeer *Peer) []*LocalConnection {
	nextHops := peer.Router.Routes.Broadcast(srcPeer.Name)
	if len(nextHops) == 0 {
		return nil
	}
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
	return nextConns
}

func (peer *LocalPeer) RelayBroadcast(srcPeer *Peer, df bool, frame []byte, dec *EthernetDecoder) error {
	for _, conn := range peer.NextBroadcastHops(srcPeer) {
		err := conn.Forward(df, &ForwardedFrame{
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
			case PDeleteConnection:
				peer.handleDeleteConnection(query.payload.(*LocalConnection))
				query.resultChan <- nil
			case PConnectionEstablished:
				peer.handleConnectionEstablished(query.payload.(*LocalConnection))
			case PBroadcastTCP:
				peer.handleBroadcastTCP(query.payload.([]byte))
			}
		case <-gossipTimer:
			peer.Router.SendAllGossip()
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
