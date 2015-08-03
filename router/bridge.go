package router

// Interface to packet handling on the local virtual bridge
type Bridge interface {
	// Inject a packet to be delivered locally
	InjectPacket(PacketKey) FlowOp

	// Start consuming packets from the bridge
	StartConsumingPackets(BridgeConsumer) error

	String() string
	Stats() map[string]int
}

// A function that determines how to handle locally captured packets.
type BridgeConsumer func(PacketKey) FlowOp

type NullBridge struct{}

func (NullBridge) InjectPacket(PacketKey) FlowOp {
	return nil
}

func (NullBridge) StartConsumingPackets(BridgeConsumer) error {
	return nil
}

func (NullBridge) String() string {
	return "<no bridge networking>"
}

func (NullBridge) Stats() map[string]int {
	return nil
}
