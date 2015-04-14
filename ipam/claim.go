package ipam

import (
	"fmt"
	"net"

	"github.com/weaveworks/weave/router"
)

type claim struct {
	resultChan chan<- error
	ident      string
	addr       net.IP
}

// Try returns true for success (or failure), false if we need to try again later
func (c *claim) Try(alloc *Allocator) bool {
	if !alloc.ring.Contains(c.addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe\n", c.addr, c.ident)
		c.resultChan <- nil
		return true
	}

	// If our ring doesn't know, it must be empty.  We will have tried
	// to do a leader elect, so we want until we find some owner for this
	// range (might be us).
	owner := alloc.ring.Owner(c.addr)
	if owner == router.UnknownPeerName {
		alloc.infof("Ring is empty; will try later.\n", c.addr, owner)
		return false
	}
	if owner != alloc.ourName {
		c.resultChan <- fmt.Errorf("Address %s is owned by other peer %s", c.addr, owner)
		return true
	}
	// We are the owner, check we haven't given it to another containe
	existingIdent := alloc.findOwner(c.addr)
	if existingIdent == c.ident {
		// same identifier is claiming same address; that's OK
		c.resultChan <- nil
		return true
	}
	if existingIdent == "" {
		err := alloc.spaceSet.Claim(c.addr)
		if err != nil {
			c.resultChan <- err
			return true
		}
		alloc.addOwned(c.ident, c.addr)
		c.resultChan <- nil
		return true
	}
	// Addr already owned by container on this machine
	c.resultChan <- fmt.Errorf("Claimed address %s is already owned by %s", c.addr, existingIdent)
	return true
}

func (c *claim) Cancel() {
	c.resultChan <- fmt.Errorf("Operation cancelled.")
}

func (c *claim) String() string {
	return fmt.Sprintf("Claim %s -> %s", c.ident, c.addr)
}
