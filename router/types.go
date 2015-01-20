package router

import (
	"net"
)

type UDPPacket struct {
	Name   PeerName
	Packet []byte
	Sender *net.UDPAddr
}

type MsgTooBigError struct {
	PMTU int // actual pmtu, i.e. what the kernel told us
}

type FrameTooBigError struct {
	EPMTU int // effective pmtu, i.e. what we tell packet senders
}

type UnknownPeersError struct {
}

type NameCollisionError struct {
	Name PeerName
}

type PacketDecodingError struct {
	Fatal bool
	Desc  string
}

type LocalAddress struct {
	ip      net.IP
	network *net.IPNet
}

type ForwardedFrame struct {
	srcPeer *Peer
	dstPeer *Peer
	frame   []byte
}

type Interaction struct {
	code       int
	resultChan chan<- interface{}
}

// Packet capture/inject interfaces

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
