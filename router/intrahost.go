package router

// Interface to intra-host (i.e. container) packet handling
type IntraHost interface {
	// Inject a packet to be delivered locally
	InjectPacket([]byte) error

	// Start consuming packets
	ConsumePackets(IntraHostConsumer) error
}

type IntraHostConsumer interface {
	// A locally captured packet.  The caller must supply an
	// EthernetDecoder specific to this thread, but it should not
	// have been used to decode the packet data.
	CapturedPacket([]byte, *EthernetDecoder)
}

type NullIntraHost struct{}

func (NullIntraHost) InjectPacket([]byte) error {
	return nil
}

func (NullIntraHost) ConsumePackets(IntraHostConsumer) error {
	return nil
}

func (NullIntraHost) String() string {
	return "<no intra-host networking>"
}
