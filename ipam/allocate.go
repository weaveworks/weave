package ipam

import (
	"fmt"
	"github.com/weaveworks/weave/ipam/address"
)

type allocateResult struct {
	addr address.Address
	err  error
}

type allocate struct {
	resultChan       chan<- allocateResult
	hasBeenCancelled func() bool
	ident            string
	subnet           address.CIDR
}

// Try returns true if the request is completed, false if pending
func (g *allocate) Try(alloc *Allocator) bool {
	if g.hasBeenCancelled() {
		g.Cancel()
		return true
	}

	// If we have previously stored an address for this container in this subnet, return it.
	if addrs, found := alloc.owned[g.ident]; found {
		for _, addr := range addrs {
			if g.subnet.Contains(addr) {
				g.resultChan <- allocateResult{addr, nil}
				return true
			}
		}
	}

	if !alloc.universe.Overlaps(g.subnet) {
		g.resultChan <- allocateResult{0, fmt.Errorf("Subnet %s out of bounds: %s", g.subnet, alloc.universe)}
		return true
	}

	// Respect RFC1122 exclusions of first and last addresses
	start, end := g.subnet.Start+1, g.subnet.End()-1
	if ok, addr := alloc.space.Allocate(start, end); ok {
		alloc.debugln("Allocated", addr, "for", g.ident, "in", g.subnet)
		alloc.addOwned(g.ident, addr)
		g.resultChan <- allocateResult{addr, nil}
		return true
	}

	// out of space
	if donor, err := alloc.ring.ChoosePeerToAskForSpace(start, end); err == nil {
		alloc.debugln("Decided to ask peer", donor, "for space in subnet", g.subnet)
		alloc.sendSpaceRequest(donor, g.subnet)
	}

	return false
}

func (g *allocate) Cancel() {
	g.resultChan <- allocateResult{0, fmt.Errorf("Allocate request for %s cancelled", g.ident)}
}

func (g *allocate) String() string {
	return fmt.Sprintf("Allocate for %s", g.ident)
}

func (g *allocate) ForContainer(ident string) bool {
	return g.ident == ident
}
