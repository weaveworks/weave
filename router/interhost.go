package router

import (
	"net"
)

// Interface to inter-host (i.e. overlay network) packet handling
type InterHost interface {
	// Start consuming forwarded packets.
	ConsumePackets(*Peer, *Peers, InterHostConsumer) error

	// The routes have changed, so any cached information should
	// be discarded.
	InvalidateRoutes()

	// Form a packet-forwarding connection.  The remote UDPAddr
	// can be nil if unknown (in which case the implementation
	// needs to discover it).
	MakeForwarder(remotePeer *Peer, localIP net.IP, remote *net.UDPAddr,
		connUID uint64, crypto InterHostCrypto,
		sendControlMessage func([]byte) error) (InterHostForwarder, error)
}

// When a consumer is called, the decoder will already have been used
// to decode the frame.
type InterHostConsumer func(ForwardPacketKey) FlowOp

// Crypto settings for a forwarder.
type InterHostCrypto struct {
	Dec   Decryptor
	Enc   Encryptor
	EncDF Encryptor
}

// All of the machinery to forward packets to a particular peer
type InterHostForwarder interface {
	// Register a callback for forwarder state changes.
	// side-effect, calling this confirms that the connection is
	// really wanted, and so the provider should activate it.
	// However, Forward might be called before this is called
	// (e.g. on another thread).
	SetListener(InterHostForwarderListener)

	// Forward a packet across the connection.
	Forward(ForwardPacketKey) FlowOp

	Close()

	// Handle a message from the peer
	ControlMessage([]byte)
}

type InterHostForwarderListener interface {
	Established()
	Error(error)
}

type NullInterHost struct{}

func (NullInterHost) ConsumeInterHostPackets(*Peer, *Peers,
	InterHostConsumer) error {
	return nil
}

func (NullInterHost) InvalidateRoutes() {
}

func (NullInterHost) MakeForwarder(*Peer, net.IP, *net.UDPAddr, uint64,
	InterHostCrypto, func([]byte) error) (InterHostForwarder, error) {
	return NullInterHost{}, nil
}

func (NullInterHost) SetListener(InterHostForwarderListener) {
}

func (NullInterHost) Forward(ForwardPacketKey) FlowOp {
	return nil
}

func (NullInterHost) Close() {
}

func (NullInterHost) ControlMessage([]byte) {
}
