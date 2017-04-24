package router

import (
	"net"
)

// Interface to packet handling on the local virtual bridge
type InjectorConsumer interface {
	// Inject a packet to be delivered locally
	InjectPacket(PacketKey) FlowOp

	// Start consuming packets from the bridge.  Injected packets
	// should not be included.
	StartConsumingPackets(Consumer) error

	Interface() *net.Interface
	String() string
	Stats() map[string]int
}

// A function that determines how to handle locally captured packets.
type Consumer func(PacketKey) FlowOp

type NullInjectorConsumer struct{}

func (NullInjectorConsumer) InjectPacket(PacketKey) FlowOp {
	return nil
}

func (NullInjectorConsumer) StartConsumingPackets(Consumer) error {
	return nil
}

func (NullInjectorConsumer) Interface() *net.Interface {
	return nil
}

func (NullInjectorConsumer) String() string {
	return "no overlay bridge"
}

func (NullInjectorConsumer) Stats() map[string]int {
	return nil
}
