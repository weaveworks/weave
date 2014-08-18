package weave

import (
	"bytes"
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"code.google.com/p/gopacket/pcap"
	"log"
	"net"
)

func NewEthernetDecoder() *EthernetDecoder {
	dec := &EthernetDecoder{}
	dec.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &dec.eth, &dec.ip)
	return dec
}

func (dec *EthernetDecoder) DecodeLayers(data []byte) error {
	return dec.parser.DecodeLayers(data, &dec.decoded)
}

func (dec *EthernetDecoder) CheckFrameTooBigFunc(srcMac net.HardwareAddr, localAddrs []net.Addr, handle *pcap.Handle) func(error) error {
	networks := make([]LocalAddress, len(localAddrs))
	idx := 0
	for _, addr := range localAddrs {
		ip, ipnet, err := net.ParseCIDR(addr.String())
		if err == nil && ip.To4() != nil {
			networks[idx] = LocalAddress{ip: ip.To4(), network: ipnet}
			idx++
		}
	}
	networks = networks[:idx]
	if len(networks) == 0 {
		log.Fatal("Interface has no addresses assigned. Cannot continue")
	}
	findIP := func(ip net.IP) net.IP {
		for _, la := range networks {
			if la.network.Contains(ip) {
				return la.ip
			}
		}
		// can't find anything, we'll have to rely on default gateway.
		return networks[0].ip
	}

	return func(err error) error {
		if ftbe, ok := err.(FrameTooBigError); ok {
			// we know: 1. ip is valid, 2. it was ip and DF was set
			srcIP := findIP(dec.ip.SrcIP)
			icmpFrame, err := dec.formICMPMTUPacket(srcMac, srcIP, ftbe.PMTU)
			if err != nil {
				return err
			}
			log.Println("Injecting ICMP 3,4. PMTU:", ftbe.PMTU)
			return handle.WritePacketData(icmpFrame)
		} else {
			return err
		}
	}
}

func (dec *EthernetDecoder) formICMPMTUPacket(srcMac net.HardwareAddr, srcIP net.IP, mtu int) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true}
	ipHeaderSize := int(dec.ip.IHL) * 4 // IHL is the number of 32-byte words in the header
	payload := gopacket.Payload(dec.ip.BaseLayer.Contents[:ipHeaderSize+8])
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       srcMac,
			DstMAC:       dec.eth.SrcMAC,
			EthernetType: dec.eth.EthernetType},
		&layers.IPv4{
			Version:    4,
			TOS:        dec.ip.TOS,
			Id:         0,
			Flags:      0,
			FragOffset: 0,
			TTL:        64,
			Protocol:   layers.IPProtocolICMPv4,
			DstIP:      dec.ip.SrcIP,
			SrcIP:      srcIP},
		&layers.ICMPv4{
			TypeCode: 0x304,
			Id:       0,
			Seq:      uint16(mtu)},
		&payload)
	if err == nil {
		return buf.Bytes(), nil
	} else {
		return []byte{}, err
	}
}

var (
	// see http://en.wikipedia.org/wiki/Multicast_address#Ethernet
	broadcastMAC, _        = net.ParseMAC("ff:ff:ff:ff:ff:ff")
	zeroMAC, _             = net.ParseMAC("00:00:00:00:00:00")
	stpMACPrefix           = []byte{0x01, 0x80, 0xC2, 0x00, 0x00}
	ipv4MulticastMACPrefix = []byte{0x01, 0x00, 0x5E}
	ipv6MulticastMACPrefix = []byte{0x33, 0x33}
)

func (dec *EthernetDecoder) DropFrame() bool {
	return bytes.Equal(stpMACPrefix, dec.eth.DstMAC[:len(stpMACPrefix)])
}

func (dec *EthernetDecoder) BroadcastFrame() bool {
	return bytes.Equal(broadcastMAC, dec.eth.DstMAC) ||
		// treat multicast frames as broadcast
		bytes.Equal(ipv4MulticastMACPrefix, dec.eth.DstMAC[:len(ipv4MulticastMACPrefix)]) ||
		bytes.Equal(ipv6MulticastMACPrefix, dec.eth.DstMAC[:len(ipv6MulticastMACPrefix)])
}

func (dec *EthernetDecoder) IsPMTUVerify() bool {
	return bytes.Equal(zeroMAC, dec.eth.SrcMAC) &&
		bytes.Equal(zeroMAC, dec.eth.DstMAC)
}
