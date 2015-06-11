package router

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"time"
)

const macMaxAge = 10 * time.Minute // [1]

// [1] should be greater than typical ARP cache expiries, i.e. > 3/2 *
// /proc/sys/net/ipv4_neigh/*/base_reachable_time_ms on Linux

type LogFrameFunc func(string, []byte, *EthernetDecoder)

type RouterConfig struct {
	Port      int
	Password  []byte
	ConnLimit int
	LogFrame  LogFrameFunc
	IntraHost IntraHost
	InterHost InterHost
}

type Router struct {
	RouterConfig
	Ourself         *LocalPeer
	Macs            *MacCache
	Peers           *Peers
	Routes          *Routes
	ConnectionMaker *ConnectionMaker
	GossipChannels  map[uint32]*GossipChannel
	TopologyGossip  Gossip
}

func NewRouter(config RouterConfig, name PeerName, nickName string) *Router {
	router := &Router{RouterConfig: config, GossipChannels: make(map[uint32]*GossipChannel)}

	// If the caller didn't set the intrahost, replace it with a
	// null implementation
	if router.IntraHost == nil {
		router.IntraHost = NullIntraHost{}
	}

	if router.InterHost == nil {
		router.InterHost = NullInterHost{}
	}

	onMacExpiry := func(mac net.HardwareAddr, peer *Peer) {
		log.Println("Expired MAC", mac, "at", peer)
	}
	onPeerGC := func(peer *Peer) {
		router.Macs.Delete(peer)
		log.Println("Removed unreachable peer", peer)
	}
	router.Ourself = NewLocalPeer(name, nickName, router)
	router.Macs = NewMacCache(macMaxAge, onMacExpiry)
	router.Peers = NewPeers(router.Ourself, onPeerGC)
	router.Routes = NewRoutes(router.Ourself, router.Peers)
	router.ConnectionMaker = NewConnectionMaker(router.Ourself, router.Peers, router.Port)
	router.TopologyGossip = router.NewGossip("topology", router)
	return router
}

// Start listening for packets from containers, TCP connections and
// packets forwarded over UDP.
func (router *Router) Start() {
	checkFatal(router.InterHost.ConsumePackets(router.Ourself.Peer, router.Peers, router.handleForwardedPacket))
	checkFatal(router.IntraHost.ConsumePackets(router))
	router.listenTCP(router.Port)
}

func (router *Router) Stop() error {
	// TODO: perform graceful shutdown...
	return nil
}

func (router *Router) UsingPassword() bool {
	return router.Password != nil
}

func (router *Router) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Our name is", router.Ourself)
	fmt.Fprintln(&buf, "Sniffing traffic on", router.IntraHost)
	fmt.Fprintf(&buf, "MACs:\n%s", router.Macs)
	fmt.Fprintf(&buf, "Peers:\n%s", router.Peers)
	fmt.Fprintf(&buf, "Routes:\n%s", router.Routes)
	fmt.Fprintf(&buf, "Reconnects:\n%s", router.ConnectionMaker)
	return buf.String()
}

func (router *Router) CapturedPacket(key PacketKey) FlowOp {
	//router.LogFrame("Sniffed", frameData, nil)
	srcMac := net.HardwareAddr(key.SrcMAC[:])
	srcPeer := router.Macs.Lookup(srcMac)

	// We need to filter out frames we injected ourselves. For such
	// frames, the srcMAC will have been recorded as associated with a
	// different peer.
	if srcPeer != nil && srcPeer != router.Ourself.Peer {
		return nil
	}

	if router.Macs.Enter(srcMac, router.Ourself.Peer) {
		log.Println("Discovered local MAC", srcMac)
	}

	// Discard STP broadcasts
	if key.DstMAC == [...]byte{0x01, 0x80, 0xC2, 0x00, 0x00, 0x00} {
		return nil
	}

	dstMac := net.HardwareAddr(key.DstMAC[:])
	switch dstPeer := router.Macs.Lookup(dstMac); dstPeer {
	case router.Ourself.Peer:
		return nil
	case nil:
		// If we don't know which peer corresponds to the dest
		// MAC, broadcast it.
		return router.Ourself.Broadcast(key)
	default:
		//router.LogFrame("Forwarding", frameData, dec)
		return router.Ourself.Forward(dstPeer, key)
	}
}

func (router *Router) listenTCP(localPort int) {
	localAddr, err := net.ResolveTCPAddr("tcp4", fmt.Sprint(":", localPort))
	checkFatal(err)
	ln, err := net.ListenTCP("tcp4", localAddr)
	checkFatal(err)
	go func() {
		defer ln.Close()
		for {
			tcpConn, err := ln.AcceptTCP()
			if err != nil {
				log.Println(err)
				continue
			}
			router.acceptTCP(tcpConn)
		}
	}()
}

func (router *Router) acceptTCP(tcpConn *net.TCPConn) {
	// someone else is dialing us, so our udp sender is the conn
	// on router.Port and we wait for them to send us something on UDP to
	// start.
	remoteAddrStr := tcpConn.RemoteAddr().String()
	log.Printf("->[%s] connection accepted\n", remoteAddrStr)
	connRemote := NewRemoteConnection(router.Ourself.Peer, nil, remoteAddrStr, false, false)
	StartLocalConnection(connRemote, tcpConn, nil, router, true)
}

func (router *Router) handleForwardedPacket(key ForwardPacketKey) FlowOp {
	if key.DstPeer != router.Ourself.Peer {
		// it's not for us, we're just relaying it
		//router.LogFrame("Relaying", frame, dec)
		return router.Ourself.Relay(key)
	}

	// At this point, it's either unicast to us, or a broadcast
	// (because the DstPeer on a forwarded broadcast packet is
	// always set to the peer being forwarded to)

	srcMac := net.HardwareAddr(key.SrcMAC[:])
	dstMac := net.HardwareAddr(key.DstMAC[:])
	if router.Macs.Enter(srcMac, key.SrcPeer) {
		log.Println("Discovered remote MAC", srcMac, "at", key.SrcPeer)
	}

	//router.LogFrame("Injecting", frame, dec)
	injectFop := router.IntraHost.InjectPacket(key.PacketKey)
	dstPeer := router.Macs.Lookup(dstMac)
	if dstPeer == router.Ourself.Peer {
		return injectFop
	}

	//router.LogFrame("Relaying broadcast", frame, dec)
	relayFop := router.Ourself.RelayBroadcast(key.SrcPeer, key.PacketKey)
	switch {
	case injectFop == nil:
		return relayFop

	case relayFop == nil:
		return injectFop

	default:
		mfop := NewMultiFlowOp(false)
		mfop.Add(injectFop)
		mfop.Add(relayFop)
		return mfop
	}
}

// Gossiper methods - the Router is the topology Gossiper

type TopologyGossipData struct {
	peers  *Peers
	update PeerNameSet
}

func NewTopologyGossipData(peers *Peers, update ...*Peer) *TopologyGossipData {
	names := make(PeerNameSet)
	for _, p := range update {
		names[p.Name] = void
	}
	return &TopologyGossipData{peers: peers, update: names}
}

func (d *TopologyGossipData) Merge(other GossipData) {
	for name := range other.(*TopologyGossipData).update {
		d.update[name] = void
	}
}

func (d *TopologyGossipData) Encode() []byte {
	return d.peers.EncodePeers(d.update)
}

func (router *Router) OnGossipUnicast(sender PeerName, msg []byte) error {
	return fmt.Errorf("unexpected topology gossip unicast: %v", msg)
}

func (router *Router) OnGossipBroadcast(update []byte) (GossipData, error) {
	origUpdate, _, err := router.applyTopologyUpdate(update)
	if err != nil || len(origUpdate) == 0 {
		return nil, err
	}
	return &TopologyGossipData{peers: router.Peers, update: origUpdate}, nil
}

func (router *Router) Gossip() GossipData {
	return &TopologyGossipData{peers: router.Peers, update: router.Peers.Names()}
}

func (router *Router) OnGossip(update []byte) (GossipData, error) {
	_, newUpdate, err := router.applyTopologyUpdate(update)
	if err != nil || len(newUpdate) == 0 {
		return nil, err
	}
	return &TopologyGossipData{peers: router.Peers, update: newUpdate}, nil
}

func (router *Router) applyTopologyUpdate(update []byte) (PeerNameSet, PeerNameSet, error) {
	origUpdate, newUpdate, err := router.Peers.ApplyUpdate(update)
	if _, ok := err.(UnknownPeerError); err != nil && ok {
		// That update contained a reference to a peer which wasn't
		// itself included in the update, and we didn't know about
		// already. We ignore this; eventually we should receive an
		// update containing a complete topology.
		log.Println("Topology gossip:", err)
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(newUpdate) > 0 {
		router.ConnectionMaker.Refresh()
		router.Routes.Recalculate()
	}
	return origUpdate, newUpdate, nil
}

func (router *Router) broadcastPeerUpdate(peers ...*Peer) {
	router.Routes.Recalculate()
	router.TopologyGossip.GossipBroadcast(NewTopologyGossipData(router.Peers, append(peers, router.Ourself.Peer)...))
}
