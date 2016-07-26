package router

import (
	"net"

	"github.com/weaveworks/mesh"
)

// Just enough flow machinery for the weave router

type MAC [6]byte

func (mac MAC) String() string {
	return net.HardwareAddr(mac[:]).String()
}

type PacketKey struct {
	SrcMAC MAC
	DstMAC MAC
}

type ForwardPacketKey struct {
	SrcPeer *mesh.Peer
	DstPeer *mesh.Peer
	PacketKey
}

type FlowOp interface {
	// The caller must supply an EthernetDecoder specific to this
	// thread, which has already been used to decode the frame.
	// The broadcast parameter is a hint whether the packet is
	// being broadcast.
	Process(frame []byte, dec *EthernetDecoder, broadcast bool)

	// Does the FlowOp discard the packet?
	Discards() bool
}

type DiscardingFlowOp struct{}

func (DiscardingFlowOp) Process([]byte, *EthernetDecoder, bool) {
}

func (DiscardingFlowOp) Discards() bool {
	return true
}

type NonDiscardingFlowOp struct{}

func (NonDiscardingFlowOp) Discards() bool {
	return false
}

type MultiFlowOp struct {
	broadcast bool
	ops       []FlowOp
}

func NewMultiFlowOp(broadcast bool, ops ...FlowOp) *MultiFlowOp {
	mfop := &MultiFlowOp{broadcast: broadcast}
	for _, op := range ops {
		mfop.Add(op)
	}
	return mfop
}

func (mfop *MultiFlowOp) Add(op FlowOp) {
	mfop.ops = append(mfop.ops, op)
}

func (mfop *MultiFlowOp) Process(frame []byte, dec *EthernetDecoder, broadcast bool) {
	for _, op := range mfop.ops {
		op.Process(frame, dec, mfop.broadcast)
	}
}

func (mfop *MultiFlowOp) Discards() bool {
	for _, op := range mfop.ops {
		if !op.Discards() {
			return false
		}
	}

	return true
}

// Flatten out a FlowOp to eliminate any MultiFlowOps and return a broadcast hint
// which is true if any MultiFlowOps has set it to true.
func FlattenFlowOp(fop FlowOp) (fops []FlowOp, broadcast bool) {
	return collectFlowOps(nil, false, fop)
}

func collectFlowOps(into []FlowOp, broadcast bool, fop FlowOp) ([]FlowOp, bool) {
	var bcast bool

	if mfop, ok := fop.(*MultiFlowOp); ok {
		broadcast = broadcast || mfop.broadcast
		for _, op := range mfop.ops {
			into, bcast = collectFlowOps(into, broadcast, op)
			broadcast = broadcast || bcast
		}

		return into, broadcast
	}

	return append(into, fop), broadcast
}
