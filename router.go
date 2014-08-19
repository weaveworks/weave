package weave

import (
	"bytes"
	"code.google.com/p/gopacket/layers"
	"code.google.com/p/gopacket/pcap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"
)

const macMaxAge = 10 * time.Minute

func NewRouter(iface *net.Interface, name PeerName, password []byte, connLimit int, bufSz int, logFrame func(string, []byte, *layers.Ethernet)) *Router {
	router := &Router{
		Iface:     iface,
		Macs:      NewMacCache(macMaxAge),
		Peers:     NewPeerCache(),
		ConnLimit: connLimit,
		BufSz:     bufSz,
		LogFrame:  logFrame}
	if len(password) > 0 {
		router.Password = &password
	}
	ourself := NewPeer(name, 0, 0, router)
	router.Ourself = router.Peers.FetchWithDefault(ourself)
	router.Ourself.StartLocalPeer()
	log.Println("Local identity is", router.Ourself.Name)

	return router
}

func (router *Router) UsingPassword() bool {
	return router.Password != nil
}

func (router *Router) Start() {
	handle := router.createPCap(true, 65535, router.BufSz)
	router.ConnectionMaker = StartConnectionMaker(router)
	router.Topology = StartTopology(router)
	router.UDPListener = router.listenUDP(Port, handle)
	router.listenTCP(Port)
	router.sniff(handle)
}

func (router *Router) Status() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintln("Local identity is", router.Ourself.Name))
	buf.WriteString(fmt.Sprintln("Sniffing traffic on", router.Iface))
	buf.WriteString(fmt.Sprintf("MACs:\n%s", router.Macs))
	buf.WriteString(fmt.Sprintf("Peers:\n%s", router.Peers))
	buf.WriteString(fmt.Sprintf("Topology:\n%s", router.Topology))
	buf.WriteString(fmt.Sprintf("Reconnects:\n%s", router.ConnectionMaker))
	return buf.String()
}

func (router *Router) sniff(handle *pcap.Handle) {
	log.Println("Sniffing traffic on", router.Iface)

	dec := NewEthernetDecoder()

	localAddrs, err := router.Iface.Addrs()
	checkFatal(err)
	checkFrameTooBig := dec.CheckFrameTooBigFunc(router.Iface.HardwareAddr, localAddrs, handle)

	mac := router.Iface.HardwareAddr
	if router.Macs.Enter(mac, router.Ourself) {
		log.Println("Discovered our MAC", mac)
	}
	go func() {
		defer handle.Close()
		for {
			pkt, _, err := handle.ReadPacketData()
			checkFatal(err)
			router.LogFrame("Sniffed", pkt, nil)
			checkWarn(router.handleCapturedPacket(pkt, dec, checkFrameTooBig))
		}
	}()
}

func (router *Router) handleCapturedPacket(frameData []byte, dec *EthernetDecoder, checkFrameTooBig func(error) error) error {
	dec.DecodeLayers(frameData)
	decodedLen := len(dec.decoded)
	if decodedLen == 0 {
		return nil
	}
	srcMac := dec.eth.SrcMAC
	srcPeer, found := router.Macs.Lookup(srcMac)
	// We need to filter out frames we injected ourselves. For such
	// frames, the srcMAC will have been recorded as associated with a
	// different peer.
	if found && srcPeer != router.Ourself {
		return nil
	}
	if router.Macs.Enter(srcMac, router.Ourself) {
		log.Println("Discovered local MAC", srcMac)
	}
	if dec.DropFrame() {
		return nil
	}
	dstMac := dec.eth.DstMAC
	dstPeer, found := router.Macs.Lookup(dstMac)
	if found && dstPeer == router.Ourself {
		return nil
	}
	df := decodedLen == 2 && (dec.ip.Flags&layers.IPv4DontFragment != 0)
	if df {
		router.LogFrame("Forwarding DF", frameData, &dec.eth)
	} else {
		router.LogFrame("Forwarding", frameData, &dec.eth)
	}
	if !found || dec.BroadcastFrame() {
		return checkFrameTooBig(router.Ourself.Broadcast(df, frameData, dec))
	} else {
		return checkFrameTooBig(router.Ourself.Forward(dstPeer, df, frameData, dec))
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
	// on Port and we wait for them to send us something on UDP to
	// start.
	connRemote := NewRemoteConnection(router.Ourself, nil, tcpConn.RemoteAddr().String())
	NewLocalConnection(connRemote, UnknownPeerName, tcpConn, nil, router)
}

	func (router *Router) listenUDP(localPort int, handle *pcap.Handle) *net.UDPConn {
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
	go router.udpReader(conn, handle)
	return conn
}

func (router *Router) udpReader(conn *net.UDPConn, handle *pcap.Handle) {
	defer conn.Close()
	dec := NewEthernetDecoder()
	handleUDPPacket := router.handleUDPPacketFunc(dec, handle)
	buf := make([]byte, MaxUDPPacketSize)
	for {
		n, sender, err := conn.ReadFromUDP(buf)
		if err == io.EOF {
			return
		} else if err != nil {
			log.Println("ignoring UDP read error", err)
			continue
		} else if n < NameSize {
			continue // TODO something different?
		} else {
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
			checkWarn(relayConn.Decryptor.IterateFrames(handleUDPPacket, udpPacket))
		}
	}
}

func (router *Router) handleUDPPacketFunc(dec *EthernetDecoder, handle *pcap.Handle) FrameConsumer {
	localAddrs, err := router.Iface.Addrs()
	checkFatal(err)
	checkFrameTooBig := dec.CheckFrameTooBigFunc(router.Iface.HardwareAddr, localAddrs, handle)

	return func(relayConn *LocalConnection, sender *net.UDPAddr, srcNameByte, dstNameByte []byte, frameLen uint16, frame []byte) error {
		srcName := PeerNameFromBin(srcNameByte)
		dstName := PeerNameFromBin(dstNameByte)
		srcPeer, found := router.Peers.Fetch(srcName)
		if !found {
			return nil
		}
		dstPeer, found := router.Peers.Fetch(dstName)
		if !found {
			return nil
		}

		dec.DecodeLayers(frame)
		decodedLen := len(dec.decoded)
		df := decodedLen == 2 && (dec.ip.Flags&layers.IPv4DontFragment != 0)
		srcMac := dec.eth.SrcMAC

		if dstPeer != router.Ourself {
			// it's not for us, we're just relaying it
			if decodedLen == 0 {
				return nil
			}
			if router.Macs.Enter(srcMac, srcPeer) {
				log.Println("Discovered remote MAC", srcMac, "at", srcPeer.Name)
			}
			return checkFrameTooBig(router.Ourself.Relay(srcPeer, dstPeer, df, frame, dec))
		}

		if relayConn.Remote().Name == srcPeer.Name {
			if frameLen == 0 {
				relayConn.SetRemoteUDPAddr(sender)
				return nil
			} else if frameLen == FragTestSize && bytes.Equal(frame, FragTest) {
				relayConn.SendTCP(ProtocolFragmentationReceivedByte)
				return nil
			} else if frameLen == PMTUDiscoverySize && bytes.Equal(frame, PMTUDiscovery) {
				return nil
			}
		}

		if decodedLen == 0 {
			return nil
		}

		if dec.IsPMTUVerify() && relayConn.Remote().Name == srcPeer.Name {
			frameLenBytes := []byte{0, 0}
			binary.BigEndian.PutUint16(frameLenBytes, uint16(frameLen-EthernetOverhead))
			relayConn.SendTCP(Concat(ProtocolPMTUVerifiedByte, frameLenBytes))
			return nil
		}

		if router.Macs.Enter(srcMac, srcPeer) {
			log.Println("Discovered remote MAC", srcMac, "at", srcPeer.Name)
		}
		router.LogFrame("Injecting", frame, &dec.eth)
		checkWarn(handle.WritePacketData(frame))
		dstPeer, found = router.Macs.Lookup(dec.eth.DstMAC)
		if !found || dec.BroadcastFrame() || dstPeer != router.Ourself {
			df := decodedLen == 2 && (dec.ip.Flags&layers.IPv4DontFragment != 0)
			checkFrameTooBig(router.Ourself.RelayBroadcast(srcPeer, df, frame, dec))
		}
		return nil
	}
}

func (router *Router) createPCap(promisc bool, snaplen int, buffSize int) *pcap.Handle {
	inactive, err := pcap.NewInactiveHandle(router.Iface.Name)
	checkFatal(err)
	defer inactive.CleanUp()
	checkFatal(inactive.SetPromisc(promisc))
	checkFatal(inactive.SetSnapLen(snaplen))
	checkFatal(inactive.SetTimeout(-1))
	checkFatal(inactive.SetImmediateMode(true))
	checkFatal(inactive.SetBufferSize(buffSize))
	active, err := inactive.Activate()
	checkFatal(err)
	checkFatal(active.SetDirection(pcap.DirectionIn))
	return active
}
