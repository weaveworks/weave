package router

// Interface to intra-host (i.e. container) packet handling
type IntraHost interface {
	// Inject a packet to be delivered locally
	InjectPacket(PacketKey) FlowOp

	// Start consuming packets
	ConsumePackets(IntraHostConsumer) error
}

type IntraHostConsumer interface {
	// A locally captured packet.  The caller must supply an
	// EthernetDecoder specific to this thread, but it should not
	// have been used to decode the packet data.
	CapturedPacket(PacketKey) FlowOp
}

type NullIntraHost struct{}

func (NullIntraHost) InjectPacket(PacketKey) FlowOp {
	return nil
}

func (NullIntraHost) ConsumePackets(IntraHostConsumer) error {
	return nil
}

func (NullIntraHost) String() string {
	return "<no intra-host networking>"
}
