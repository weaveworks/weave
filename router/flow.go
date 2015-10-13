package router

import "net"

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
	SrcPeer *Peer
	DstPeer *Peer
	PacketKey
}

type FlowOp interface {
	// The caller must supply an EthernetDecoder specific to this
	// thread, which has already been used to decode the frame.
	// The broadcast parameter is a hint whether the packet is
	// being broadcast.
	Process(frame []byte, dec *EthernetDecoder, broadcast bool)
}

type MultiFlowOp struct {
	broadcast bool
	ops       []FlowOp
}

func NewMultiFlowOp(broadcast bool, ops ...FlowOp) *MultiFlowOp {
	mfop := &MultiFlowOp{broadcast: broadcast}
	mfop.Add(ops...)
	return mfop
}

func (mfop *MultiFlowOp) Add(ops ...FlowOp) {
	for _, op := range ops {
		if op != nil {
			mfop.ops = append(mfop.ops, op)
		}
	}
}

func (mfop *MultiFlowOp) Process(frame []byte, dec *EthernetDecoder,
	broadcast bool) {
	for _, op := range mfop.ops {
		op.Process(frame, dec, mfop.broadcast)
	}
}

func FlattenFlowOp(fop FlowOp) []FlowOp {
	return collectFlowOps(nil, fop)
}

func collectFlowOps(into []FlowOp, fop FlowOp) []FlowOp {
	if fop == nil {
		return into
	}

	if mfop, ok := fop.(*MultiFlowOp); ok {
		for _, op := range mfop.ops {
			into = collectFlowOps(into, op)
		}

		return into
	}

	return append(into, fop)
}
