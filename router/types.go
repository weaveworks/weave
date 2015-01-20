package router

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
