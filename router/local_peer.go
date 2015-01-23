package router

import (
	"fmt"
	"log"
	"net"
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

func NewLocalPeer(name PeerName, router *Router) *LocalPeer {
	return &LocalPeer{Peer: NewPeer(name, 0, 0), Router: router}
}

func (peer *LocalPeer) Start() {
	queryChan := make(chan *PeerInteraction, ChannelSize)
	peer.queryChan = queryChan
	go peer.queryLoop(queryChan)
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
	connRemote := NewRemoteConnection(peer.Peer, nil, tcpConn.RemoteAddr().String())
	connLocal := NewLocalConnection(connRemote, tcpConn, udpAddr, peer.Router)
	connLocal.Start(acceptNewPeer)
	return nil
}

// ACTOR client API

const (
	PAddConnection = iota
	PDeleteConnection
	PConnectionEstablished
	PSendProtocolMsg
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
func (peer *LocalPeer) SendProtocolMsg(m ProtocolMsg) {
	peer.queryChan <- &PeerInteraction{
		Interaction: Interaction{code: PSendProtocolMsg},
		payload:     m}
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
			conn := query.payload.(*LocalConnection)
			if peer.handleAddConnection(conn) {
				conn.log("connection added")
			}
		case PDeleteConnection:
			conn := query.payload.(*LocalConnection)
			if peer.handleDeleteConnection(conn) {
				conn.log("connection deleted")
			}
			query.resultChan <- nil
		case PConnectionEstablished:
			conn := query.payload.(*LocalConnection)
			if peer.handleConnectionEstablished(conn) {
				conn.log("connection fully established")
			}
		case PSendProtocolMsg:
			peer.handleSendProtocolMsg(query.payload.(ProtocolMsg))
		}
	}
}

func (peer *LocalPeer) handleAddConnection(conn Connection) bool {
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
			return false
		}
		switch conn.BreakTie(dupConn) {
		case TieBreakWon:
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
		case TieBreakLost:
			conn.Shutdown(dupErr)
			return false
		case TieBreakTied:
			// oh good grief. Sod it, just kill both of them.
			conn.Shutdown(dupErr)
			dupConn.Shutdown(dupErr)
			peer.handleDeleteConnection(dupConn)
			return false
		}
	}
	if err := peer.checkConnectionLimit(); err != nil {
		conn.Shutdown(err)
		return false
	}
	peer.addConnection(conn)
	return true
}

func (peer *LocalPeer) handleDeleteConnection(conn Connection) bool {
	if peer.Peer != conn.Local() {
		log.Fatal("Attempt made to delete connection from peer where peer is not the source of connection")
	}
	if conn.Remote() == nil {
		log.Fatal("Attempt made to delete connection to peer with unknown remote peer")
	}
	toName := conn.Remote().Name
	if connFound, found := peer.connections[toName]; !found || connFound != conn {
		return false
	}
	peer.deleteConnection(conn)
	// Must do garbage collection first to ensure we don't send out an
	// update with unreachable peers (can cause looping)
	peer.Router.Peers.GarbageCollect()
	if conn.Established() {
		peer.broadcastPeerUpdate()
	}
	return true
}

func (peer *LocalPeer) handleConnectionEstablished(conn Connection) bool {
	if peer.Peer != conn.Local() {
		log.Fatal("Peer informed of active connection where peer is not the source of connection")
	}
	if dupConn, found := peer.connections[conn.Remote().Name]; !found || conn != dupConn {
		conn.Shutdown(fmt.Errorf("Cannot set unknown connection active"))
		return false
	}
	peer.connectionEstablished(conn)
	peer.broadcastPeerUpdate(conn.Remote())
	return true
}

func (peer *LocalPeer) handleSendProtocolMsg(m ProtocolMsg) {
	peer.ForEachConnection(func(_ PeerName, conn Connection) {
		conn.(ProtocolSender).SendProtocolMsg(m)
	})
}

func (peer *LocalPeer) broadcastPeerUpdate(peers ...*Peer) {
	peer.Router.Routes.Recalculate()
	peer.handleSendProtocolMsg(ProtocolMsg{ProtocolUpdate, EncodePeers(append(peers, peer.Peer)...)})
}

func (peer *LocalPeer) checkConnectionLimit() error {
	if 0 != peer.Router.ConnLimit && peer.ConnectionCount() >= peer.Router.ConnLimit {
		return fmt.Errorf("Connection limit reached (%v)", peer.Router.ConnLimit)
	}
	return nil
}
