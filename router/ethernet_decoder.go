package router

import (
	"bytes"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"log"
	"net"
)

type EthernetDecoder struct {
	eth     layers.Ethernet
	ip      layers.IPv4
	decoded []gopacket.LayerType
	parser  *gopacket.DecodingLayerParser
}

func NewEthernetDecoder() *EthernetDecoder {
	dec := &EthernetDecoder{}
	dec.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &dec.eth, &dec.ip)
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
	ipHeaderSize := int(dec.ip.IHL) * 4 // IHL is the number of 32-byte words in the header
	payload := gopacket.Payload(dec.ip.BaseLayer.Contents[:ipHeaderSize+8])
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       dec.eth.DstMAC,
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
			SrcIP:      dec.ip.DstIP},
		&layers.ICMPv4{
			TypeCode: 0x304,
			Id:       0,
			Seq:      uint16(mtu)},
		&payload)
	if err != nil {
		return err
	}

	log.Printf("Sending ICMP 3,4 (%v -> %v): PMTU= %v\n", dec.ip.DstIP, dec.ip.SrcIP, mtu)
	return sendFrame(buf.Bytes())
}

var (
	// see http://en.wikipedia.org/wiki/Multicast_address#Ethernet
	stpMACPrefix = []byte{0x01, 0x80, 0xC2, 0x00, 0x00}
	zeroMAC, _   = net.ParseMAC("00:00:00:00:00:00")
)

func (dec *EthernetDecoder) DropFrame() bool {
	return bytes.Equal(stpMACPrefix, dec.eth.DstMAC[:len(stpMACPrefix)])
}

func (dec *EthernetDecoder) IsSpecial() bool {
	return dec.eth.Length == 0 && dec.eth.EthernetType == layers.EthernetTypeLLC &&
		bytes.Equal(zeroMAC, dec.eth.SrcMAC) && bytes.Equal(zeroMAC, dec.eth.DstMAC)
}
