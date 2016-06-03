package tracker

import (
	"github.com/weaveworks/weave/net/address"
)

// LocalRangeTracker is an interface for tracking changes in ring allocations.
type LocalRangeTracker interface {
	// HandleUpdate is called whenever an address ring gets updated.
	//
	// prevRanges corresponds to ranges which were owned by a peer before
	// a change in the ring, while currRanges to the ones which are currently
	// owned by the peer.
	// Both slices have to be sorted in increasing order.
	// Adjacent ranges within each slice might appear as separate ranges.
	HandleUpdate(prevRanges, currRanges []address.Range) error
}
