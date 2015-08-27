package ipam

import (
	"fmt"

	"github.com/weaveworks/weave/net/address"
)

type allocateSubnetResult struct {
	cidr address.CIDR
	err  error
}

type allocateSubnet struct {
	resultChan       chan<- allocateSubnetResult
	name             string
	size             int
	hasBeenCancelled func() bool
}

// Try returns true if the request is completed, false if pending
func (g *allocateSubnet) Try(alloc *Allocator) bool {
	return true
}

func (g *allocateSubnet) Cancel() {
	g.resultChan <- allocateSubnetResult{address.CIDR{}, fmt.Errorf("Allocate subnet '%s' request cancelled", g.name)}
}

func (g *allocateSubnet) ForContainer(ident string) bool {
	return false
}
