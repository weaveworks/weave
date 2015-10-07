package router

import (
	"net"
)

// Interface to overlay network packet handling
type Overlay interface {
	// Start consuming forwarded packets.
	StartConsumingPackets(*Peer, *Peers, OverlayConsumer) error

	// Form a packet-forwarding connection.
	MakeForwarder(ForwarderParams) (OverlayForwarder, error)

	// The routes have changed, so any cached information should
	// be discarded.
	InvalidateRoutes()

	// A mapping of a short id to a peer has changed
	InvalidateShortIDs()

	// Enhance a features map with overlay-related features
	AddFeaturesTo(map[string]string)

	// Obtain diagnostic information specific to the overlay
	Diagnostics() interface{}
}

type ForwarderParams struct {
	RemotePeer *Peer

	// The local IP address to use for sending.  Derived from the
	// local address of the corresponding TCP socket, so may
	// differ for different forwarders.
	LocalIP net.IP

	// The remote address to send to.  nil if unknown, i.e. an
	// incoming connection, in which case the Overlay needs to
	// discover it (e.g. from incoming datagrams).
	RemoteAddr *net.UDPAddr

	// Unique identifier for this connection
	ConnUID uint64

	// Crypto bits.  Nil if not encrypting
	Crypto *OverlayCrypto

	// Function to send a control message to the counterpart
	// forwarder.
	SendControlMessage func(tag byte, msg []byte) error

	// Features passed at connection initiation
	Features map[string]string
}

// When a consumer is called, the decoder will already have been used
// to decode the frame.
type OverlayConsumer func(ForwardPacketKey) FlowOp

// Crypto settings for a forwarder.
type OverlayCrypto struct {
	Dec   Decryptor
	Enc   Encryptor
	EncDF Encryptor
}

// All of the machinery to forward packets to a particular peer
type OverlayForwarder interface {
	// Forward a packet across the connection.  May be called as
	// soon as the forwarder is created, in particular before
	// Confirm().
	Forward(ForwardPacketKey) FlowOp

	// Confirm that the connection is really wanted, and so the
	// Overlay should begin heartbeats etc. to verify the
	// operation of the forwarder.
	Confirm()

	// A channel indicating that the forwarder is established,
	// i.e. its operation has been confirmed.
	EstablishedChannel() <-chan struct{}

	// A channel indicating an error from the forwarder.  The
	// forwarder is not expected to be operational after the first
	// error, so the channel only needs to buffer a single error.
	ErrorChannel() <-chan error

	Stop()

	// Handle a message from the peer.  'tag' exists for
	// compatibility, and should always be
	// ProtocolOverlayControlMessage for non-sleeve overlays.
	ControlMessage(tag byte, msg []byte)

	// User facing overlay name
	DisplayName() string
}

type NullOverlay struct{}

func (NullOverlay) StartConsumingPackets(*Peer, *Peers, OverlayConsumer) error {
	return nil
}

func (NullOverlay) MakeForwarder(ForwarderParams) (OverlayForwarder, error) {
	return NullOverlay{}, nil
}

func (NullOverlay) InvalidateRoutes() {
}

func (NullOverlay) InvalidateShortIDs() {
}

func (NullOverlay) Confirm() {
}

func (NullOverlay) EstablishedChannel() <-chan struct{} {
	return nil
}

func (NullOverlay) ErrorChannel() <-chan error {
	return nil
}

func (NullOverlay) AddFeaturesTo(map[string]string) {
}

func (NullOverlay) Forward(ForwardPacketKey) FlowOp {
	return nil
}

func (NullOverlay) Stop() {
}

func (NullOverlay) ControlMessage(byte, []byte) {
}

func (NullOverlay) DisplayName() string {
	return "null"
}

func (NullOverlay) Diagnostics() interface{} {
	return nil
}
