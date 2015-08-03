package router

// Interface to packet handling on the local virtual bridge
type Bridge interface {
	// Inject a packet to be delivered locally
	InjectPacket([]byte) error

	// Start consuming packets from the bridge
	ConsumePackets(BridgeConsumer) error

	String() string
	Stats() map[string]int
}

// A function that accepts locally captured packets.  The ethernet
// decoder is specific to this thread, and will already have been used
// to to decode the packet data.
type BridgeConsumer func([]byte, *EthernetDecoder)

type NullBridge struct{}

func (NullBridge) InjectPacket([]byte) error {
	return nil
}

func (NullBridge) ConsumePackets(BridgeConsumer) error {
	return nil
}

func (NullBridge) String() string {
	return "<no bridge networking>"
}

func (NullBridge) Stats() map[string]int {
	return nil
}
