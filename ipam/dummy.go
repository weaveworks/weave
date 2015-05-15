package ipam

import (
	"errors"

	"github.com/weaveworks/weave/router"
)

// Exists solely for the purpose of detecting a network in which some
// peers are configured with IPAM on and some with IPAM off.
type DummyAllocator struct {
}

var (
	ErrMixedConfig = errors.New("Mixed IPAM/non-IPAM configuration not supported")
)

func (alloc *DummyAllocator) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	return ErrMixedConfig
}

func (alloc *DummyAllocator) OnGossipBroadcast(msg []byte) (router.GossipData, error) {
	return nil, ErrMixedConfig
}

func (alloc *DummyAllocator) OnGossip(msg []byte) (router.GossipData, error) {
	return nil, ErrMixedConfig
}

func (alloc *DummyAllocator) Gossip() router.GossipData {
	return nil
}

func (alloc *DummyAllocator) Encode() []byte {
	return nil
}
