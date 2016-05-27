package monitor

import (
	"github.com/weaveworks/weave/net/address"
)

// Monitor is an interface for tracking changes in ring allocations.
type Monitor interface {
	// HandleUpdate is called whenever an address ring gets updated.
	//
	// prevRanges corresponds to ranges which were owned by a peer before
	// a change in the ring, while currRanges to the ones which are currently
	// owned by the peer.
	HandleUpdate(prevRanges, currRanges []address.Range) error
	// String returns a user-friendly name of the monitor.
	String() string
}
