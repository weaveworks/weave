package monitor

import (
	"github.com/weaveworks/weave/net/address"
)

// Monitor is an interface for tracking changes in ring allocations.
type Monitor interface {
	// HandleUpdate is called whenever an address ring gets updated.
	//
	// {old,new}Ranges correspond to address ranges owned by a peer which
	// executes the method.
	HandleUpdate(oldRanges, newRanges []address.Range) error
	// String returns a user-friendly name of the monitor.
	String() string
}
