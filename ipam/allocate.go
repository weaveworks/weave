package ipam

import (
	"fmt"

	"github.com/weaveworks/weave/net/address"
)

type allocateResult struct {
	addr address.Address
	err  error
}

type allocate struct {
	resultChan       chan<- allocateResult
	ident            string
	r                address.Range // Range we are trying to allocate within
	hasBeenCancelled func() bool
}

// Try returns true if the request is completed, false if pending
func (g *allocate) Try(alloc *Allocator) bool {
	if g.hasBeenCancelled() {
		g.Cancel()
		return true
	}

	if addrs := alloc.ownedInRange(g.ident, g.r); len(addrs) > 0 {
		// If we had heard that this container died, resurrect it
		delete(alloc.dead, g.ident) // delete is no-op if key not in map
		g.resultChan <- allocateResult{addrs[0], nil}
		return true
	}

	if !alloc.universe.Overlaps(g.r) {
		g.resultChan <- allocateResult{err: fmt.Errorf("range %s out of bounds: %s", g.r, alloc.universe)}
		return true
	}

	alloc.establishRing()

	if ok, addr := alloc.space.Allocate(g.r); ok {
		// If caller hasn't supplied a unique ID, file it under the IP address
		// which lets the caller then release the address using DELETE /ip/address
		if g.ident == "_" {
			g.ident = addr.String()
		}
		alloc.debugln("Allocated", addr, "for", g.ident, "in", g.r)
		alloc.addOwned(g.ident, addr)
		g.resultChan <- allocateResult{addr, nil}
		return true
	}

	// out of space
	donors := alloc.ring.ChoosePeersToAskForSpace(g.r.Start, g.r.End)
	for _, donor := range donors {
		if err := alloc.sendSpaceRequest(donor, g.r); err != nil {
			alloc.debugln("Problem asking peer", donor, "for space:", err)
		} else {
			alloc.debugln("Decided to ask peer", donor, "for space in range", g.r)
			break
		}
	}

	return false
}

func (g *allocate) Cancel() {
	g.resultChan <- allocateResult{err: &errorCancelled{"Allocate", g.ident}}
}

func (g *allocate) ForContainer(ident string) bool {
	return g.ident == ident
}
