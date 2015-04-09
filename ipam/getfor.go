package ipam

import (
	"fmt"
	"net"
)

type getfor struct {
	resultChan chan<- net.IP
	ident      string
}

// Try returns true if the request is completed, false if pending
func (g *getfor) Try(alloc *Allocator) bool {
	// If we have previously stored an address for this container, return it.
	// FIXME: the data structure allows multiple addresses per (allocator per) ident but the code doesn't
	if addrs, found := alloc.owned[g.ident]; found && len(addrs) > 0 {
		g.resultChan <- addrs[0]
		return true
	}

	if addr := alloc.spaceSet.Allocate(); addr != nil {
		alloc.debugln("Allocated", addr, "for", g.ident)
		alloc.addOwned(g.ident, addr)
		g.resultChan <- addr
		return true
	}

	// out of space
	if donor, err := alloc.ring.ChoosePeerToAskForSpace(); err == nil {
		alloc.debugln("Decided to ask peer", donor, "for space")
		alloc.sendRequest(donor, msgSpaceRequest)
	}

	return false
}

func (g *getfor) String() string {
	return fmt.Sprintf("GetFor %s", g.ident)
}
