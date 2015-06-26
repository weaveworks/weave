package router

import (
	"bytes"
	"log"
	"net"

	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
)

type EthernetDecoder struct {
	Eth     layers.Ethernet
	IP      layers.IPv4
	decoded []gopacket.LayerType
	parser  *gopacket.DecodingLayerParser
}

func NewEthernetDecoder() *EthernetDecoder {
	dec := &EthernetDecoder{}
	dec.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &dec.Eth, &dec.IP)
	return dec
}

func (dec *EthernetDecoder) DecodeLayers(data []byte) error {
	return dec.parser.DecodeLayers(data, &dec.decoded)
}

func (dec *EthernetDecoder) sendICMPFragNeeded(mtu int, sendFrame func([]byte) error) error {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true}
	ipHeaderSize := int(dec.IP.IHL) * 4 // IHL is the number of 32-byte words in the header
	payload := gopacket.Payload(dec.IP.BaseLayer.Contents[:ipHeaderSize+8])
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       dec.Eth.DstMAC,
			DstMAC:       dec.Eth.SrcMAC,
			EthernetType: dec.Eth.EthernetType},
		&layers.IPv4{
			Version:    4,
			TOS:        dec.IP.TOS,
			Id:         0,
			Flags:      0,
			FragOffset: 0,
			TTL:        64,
			Protocol:   layers.IPProtocolICMPv4,
			DstIP:      dec.IP.SrcIP,
			SrcIP:      dec.IP.DstIP},
		&layers.ICMPv4{
			TypeCode: 0x304,
			Id:       0,
			Seq:      uint16(mtu)},
		&payload)
	if err != nil {
		return err
	}

	log.Printf("Sending ICMP 3,4 (%v -> %v): PMTU= %v\n", dec.IP.DstIP, dec.IP.SrcIP, mtu)
	return sendFrame(buf.Bytes())
}

var (
	// see http://en.wikipedia.org/wiki/Multicast_address#Ethernet
	stpMACPrefix = []byte{0x01, 0x80, 0xC2, 0x00, 0x00}
	zeroMAC, _   = net.ParseMAC("00:00:00:00:00:00")
)

func (dec *EthernetDecoder) DropFrame() bool {
	return bytes.Equal(stpMACPrefix, dec.Eth.DstMAC[:len(stpMACPrefix)])
}

func (dec *EthernetDecoder) IsSpecial() bool {
	return dec.Eth.Length == 0 && dec.Eth.EthernetType == layers.EthernetTypeLLC &&
		bytes.Equal(zeroMAC, dec.Eth.SrcMAC) && bytes.Equal(zeroMAC, dec.Eth.DstMAC)
}

func (dec *EthernetDecoder) DF() bool {
	return len(dec.decoded) == 2 && (dec.IP.Flags&layers.IPv4DontFragment != 0)
}
