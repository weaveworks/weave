package router

import (
	"bytes"
	"code.google.com/p/gopacket/layers"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"
)

const macMaxAge = 10 * time.Minute // [1]

// [1] should be greater than typical ARP cache expiries, i.e. > 3/2 *
// /proc/sys/net/ipv4_neigh/*/base_reachable_time_ms on Linux

type Router struct {
	Iface           *net.Interface
	Ourself         *LocalPeer
	Macs            *MacCache
	Peers           *Peers
	Routes          *Routes
	ConnectionMaker *ConnectionMaker
	GossipChannels  map[uint32]*GossipChannel
	TopologyGossip  Gossip
	UDPListener     *net.UDPConn
	Password        []byte
	ConnLimit       int
	BufSz           int
	LogFrame        func(string, []byte, *layers.Ethernet)
}

type PacketSource interface {
	ReadPacket() ([]byte, error)
}

type PacketSink interface {
	WritePacket([]byte) error
}

type PacketSourceSink interface {
	PacketSource
	PacketSink
}

func NewRouter(iface *net.Interface, name PeerName, nickName string, password []byte, connLimit int, bufSz int, logFrame func(string, []byte, *layers.Ethernet)) *Router {
	router := &Router{
		Iface:          iface,
		GossipChannels: make(map[uint32]*GossipChannel),
		Password:       password,
		ConnLimit:      connLimit,
		BufSz:          bufSz,
		LogFrame:       logFrame}
	onMacExpiry := func(mac net.HardwareAddr, peer *Peer) {
		log.Println("Expired MAC", mac, "at", peer.Name)
	}
	onPeerGC := func(peer *Peer) {
		router.Macs.Delete(peer)
		log.Println("Removed unreachable", peer)
	}
	router.Ourself = NewLocalPeer(name, nickName, router)
	router.Macs = NewMacCache(macMaxAge, onMacExpiry)
	router.Peers = NewPeers(router.Ourself.Peer, onPeerGC)
	router.Peers.FetchWithDefault(router.Ourself.Peer)
	router.Routes = NewRoutes(router.Ourself.Peer, router.Peers)
	router.ConnectionMaker = NewConnectionMaker(router.Ourself, router.Peers)
	router.TopologyGossip = router.NewGossip("topology", router)
	return router
}

func (router *Router) Start() {
	// we need two pcap handles since they aren't thread-safe
	pio, err := NewPcapIO(router.Iface.Name, router.BufSz)
	checkFatal(err)
	po, err := NewPcapO(router.Iface.Name)
	checkFatal(err)
	router.Ourself.Start()
	router.Macs.Start()
	router.Routes.Start()
	router.ConnectionMaker.Start()
	router.UDPListener = router.listenUDP(Port, po)
	router.listenTCP(Port)
	router.sniff(pio)
}

func (router *Router) UsingPassword() bool {
	return router.Password != nil
}

func (router *Router) Status() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintln("Our name is", router.Ourself.Name, "("+router.Ourself.NickName+")"))
	buf.WriteString(fmt.Sprintln("Sniffing traffic on", router.Iface))
	buf.WriteString(fmt.Sprintf("MACs:\n%s", router.Macs))
	buf.WriteString(fmt.Sprintf("Peers:\n%s", router.Peers))
	buf.WriteString(fmt.Sprintf("Routes:\n%s", router.Routes))
	buf.WriteString(fmt.Sprintf("Reconnects:\n%s", router.ConnectionMaker))
	return buf.String()
}

func (router *Router) sniff(pio PacketSourceSink) {
	log.Println("Sniffing traffic on", router.Iface)

	dec := NewEthernetDecoder()
	mac := router.Iface.HardwareAddr
	if router.Macs.Enter(mac, router.Ourself.Peer) {
		log.Println("Discovered our MAC", mac)
	}
	go func() {
		for {
			pkt, err := pio.ReadPacket()
			checkFatal(err)
			router.LogFrame("Sniffed", pkt, nil)
			router.handleCapturedPacket(pkt, dec, pio)
		}
	}()
}

func (router *Router) handleCapturedPacket(frameData []byte, dec *EthernetDecoder, po PacketSink) {
	dec.DecodeLayers(frameData)
	decodedLen := len(dec.decoded)
	if decodedLen == 0 {
		return
	}
	srcMac := dec.eth.SrcMAC
	srcPeer, found := router.Macs.Lookup(srcMac)
	// We need to filter out frames we injected ourselves. For such
	// frames, the srcMAC will have been recorded as associated with a
	// different peer.
	if found && srcPeer != router.Ourself.Peer {
		return
	}
	if router.Macs.Enter(srcMac, router.Ourself.Peer) {
		log.Println("Discovered local MAC", srcMac)
	}
	if dec.DropFrame() {
		return
	}
	dstMac := dec.eth.DstMAC
	dstPeer, found := router.Macs.Lookup(dstMac)
	if found && dstPeer == router.Ourself.Peer {
		return
	}
	df := decodedLen == 2 && (dec.ip.Flags&layers.IPv4DontFragment != 0)
	if df {
		router.LogFrame("Forwarding DF", frameData, &dec.eth)
	} else {
		router.LogFrame("Forwarding", frameData, &dec.eth)
	}
	// at this point we are handing over the frame to forwarders, so
	// we need to make a copy of it in order to prevent the next
	// capture from overwriting the data
	frameLen := len(frameData)
	frameCopy := make([]byte, frameLen, frameLen)
	copy(frameCopy, frameData)

	if !found {
		router.Ourself.Broadcast(df, frameCopy, dec)
		return
	}
	err := router.Ourself.Forward(dstPeer, df, frameCopy, dec)
	if ftbe, ok := err.(FrameTooBigError); ok {
		err = dec.sendICMPFragNeeded(ftbe.EPMTU, po.WritePacket)
	}
	checkWarn(err)
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
	// on Port and we wait for them to send us something on UDP to
	// start.
	remoteAddrStr := tcpConn.RemoteAddr().String()
	log.Printf("->[%s] connection accepted\n", remoteAddrStr)
	connRemote := NewRemoteConnection(router.Ourself.Peer, nil, remoteAddrStr, false, false)
	connLocal := NewLocalConnection(connRemote, tcpConn, nil, router)
	connLocal.Start(true)
}

func (router *Router) listenUDP(localPort int, po PacketSink) *net.UDPConn {
	localAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprint(":", localPort))
	checkFatal(err)
	conn, err := net.ListenUDP("udp4", localAddr)
	checkFatal(err)
	f, err := conn.File()
	defer f.Close()
	checkFatal(err)
	fd := int(f.Fd())
	// This one makes sure all packets we send out do not have DF set on them.
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DONT)
	checkFatal(err)
	go router.udpReader(conn, po)
	return conn
}

type UDPPacket struct {
	Name   PeerName
	Packet []byte
	Sender *net.UDPAddr
}

func (packet UDPPacket) String() string {
	return fmt.Sprintf("UDP Packet\n name: %s\n sender: %v\n payload: % X", packet.Name, packet.Sender, packet.Packet)
}

func (router *Router) udpReader(conn *net.UDPConn, po PacketSink) {
	defer conn.Close()
	dec := NewEthernetDecoder()
	handleUDPPacket := router.handleUDPPacketFunc(dec, po)
	buf := make([]byte, MaxUDPPacketSize)
	for {
		n, sender, err := conn.ReadFromUDP(buf)
		if err == io.EOF {
			return
		} else if err != nil {
			log.Println("ignoring UDP read error", err)
			continue
		} else if n < NameSize {
			log.Println("ignoring too short UDP packet from", sender)
			continue
		}
		name := PeerNameFromBin(buf[:NameSize])
		packet := make([]byte, n-NameSize)
		copy(packet, buf[NameSize:n])
		udpPacket := &UDPPacket{
			Name:   name,
			Packet: packet,
			Sender: sender}
		peerConn, found := router.Ourself.ConnectionTo(name)
		if !found {
			continue
		}
		relayConn, ok := peerConn.(*LocalConnection)
		if !ok {
			continue
		}
		err = relayConn.Decryptor.IterateFrames(handleUDPPacket, udpPacket)
		if pde, ok := err.(PacketDecodingError); ok {
			if pde.Fatal {
				relayConn.Shutdown(pde)
			} else {
				relayConn.Log(pde.Error())
			}
		} else {
			checkWarn(err)
		}
	}
}

func (router *Router) handleUDPPacketFunc(dec *EthernetDecoder, po PacketSink) FrameConsumer {
	return func(relayConn *LocalConnection, sender *net.UDPAddr, srcNameByte, dstNameByte []byte, frameLen uint16, frame []byte) {
		srcName := PeerNameFromBin(srcNameByte)
		dstName := PeerNameFromBin(dstNameByte)
		srcPeer, found := router.Peers.Fetch(srcName)
		if !found {
			return
		}
		dstPeer, found := router.Peers.Fetch(dstName)
		if !found {
			return
		}

		dec.DecodeLayers(frame)
		decodedLen := len(dec.decoded)
		if decodedLen == 0 {
			return
		}
		// Handle special frames produced internally (rather than
		// captured/forwarded) by the remote router.
		//
		// We really shouldn't be decoding these above, since they are
		// not genuine Ethernet frames. However, it is actually more
		// efficient to do so, as we want to optimise for the common
		// (i.e. non-special) frames. These always need decoding, and
		// detecting special frames is cheaper post decoding than pre.
		if decodedLen == 1 && dec.IsSpecial() {
			if srcPeer != relayConn.Remote() || dstPeer != router.Ourself.Peer {
				// A special frame not originating from the remote, or
				// not for us? How odd; let's just drop it.
				return
			}
			switch {
			case frameLen == EthernetOverhead+8:
				relayConn.ReceivedHeartbeat(sender, binary.BigEndian.Uint64(frame[EthernetOverhead:]))
			case frameLen == FragTestSize && bytes.Equal(frame, FragTest):
				relayConn.SendProtocolMsg(ProtocolMsg{ProtocolFragmentationReceived, nil})
			case frameLen == PMTUDiscoverySize && bytes.Equal(frame, PMTUDiscovery):
			default:
				frameLenBytes := []byte{0, 0}
				binary.BigEndian.PutUint16(frameLenBytes, uint16(frameLen-EthernetOverhead))
				relayConn.SendProtocolMsg(ProtocolMsg{ProtocolPMTUVerified, frameLenBytes})
			}
			return
		}

		df := decodedLen == 2 && (dec.ip.Flags&layers.IPv4DontFragment != 0)

		if dstPeer != router.Ourself.Peer {
			// it's not for us, we're just relaying it
			if df {
				router.LogFrame("Relaying DF", frame, &dec.eth)
			} else {
				router.LogFrame("Relaying", frame, &dec.eth)
			}

			err := router.Ourself.Relay(srcPeer, dstPeer, df, frame, dec)
			if ftbe, ok := err.(FrameTooBigError); ok {
				err = dec.sendICMPFragNeeded(ftbe.EPMTU, func(icmpFrame []byte) error {
					return router.Ourself.Forward(srcPeer, false, icmpFrame, nil)
				})
			}

			checkWarn(err)
			return
		}

		srcMac := dec.eth.SrcMAC
		dstMac := dec.eth.DstMAC

		if router.Macs.Enter(srcMac, srcPeer) {
			log.Println("Discovered remote MAC", srcMac, "at", srcName)
		}
		router.LogFrame("Injecting", frame, &dec.eth)
		checkWarn(po.WritePacket(frame))

		dstPeer, found = router.Macs.Lookup(dstMac)
		if !found || dstPeer != router.Ourself.Peer {
			router.LogFrame("Relaying broadcast", frame, &dec.eth)
			router.Ourself.RelayBroadcast(srcPeer, df, frame, dec)
		}
	}
}

// Gossiper methods - the Router is the topology Gossiper

func (router *Router) OnGossipUnicast(sender PeerName, msg []byte) error {
	return fmt.Errorf("unexpected topology gossip unicast: %v", msg)
}

func (router *Router) OnGossipBroadcast(msg []byte) error {
	return fmt.Errorf("unexpected topology gossip broadcast: %v", msg)
}

type PeerNameSet map[PeerName]bool

type TopologyGossipData struct {
	peers  *Peers
	update PeerNameSet
}

func NewTopologyGossipData(peers *Peers, update ...*Peer) *TopologyGossipData {
	names := make(PeerNameSet)
	for _, p := range update {
		names[p.Name] = true
	}
	return &TopologyGossipData{peers: peers, update: names}
}

func (d *TopologyGossipData) Merge(other GossipData) {
	for name, _ := range other.(*TopologyGossipData).update {
		d.update[name] = true
	}
}

func (d *TopologyGossipData) Encode() []byte {
	return d.peers.EncodePeers(d.update)
}

func (router *Router) Gossip() GossipData {
	return &TopologyGossipData{peers: router.Peers, update: router.Peers.Names()}
}

func (router *Router) OnGossip(buf []byte) (GossipData, error) {
	newUpdate, err := router.Peers.ApplyUpdate(buf)
	if _, ok := err.(UnknownPeerError); err != nil && ok {
		// That update contained a reference to a peer which wasn't
		// itself included in the update, and we didn't know about
		// already. We ignore this; eventually we should receive an
		// update containing a complete topology.
		log.Println("Topology gossip:", err)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(newUpdate) == 0 {
		return nil, nil
	}
	router.ConnectionMaker.Refresh()
	router.Routes.Recalculate()
	return &TopologyGossipData{peers: router.Peers, update: newUpdate}, nil
}
