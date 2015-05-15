package ipam

import (
	"fmt"

	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/router"
)

type claim struct {
	resultChan       chan<- error
	hasBeenCancelled func() bool
	ident            string
	addr             address.Address
}

// Try returns true for success (or failure), false if we need to try again later
func (c *claim) Try(alloc *Allocator) bool {
	if (c.hasBeenCancelled)() {
		c.Cancel()
		return true
	}

	if !alloc.ring.Contains(c.addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe\n", c.addr, c.ident)
		c.resultChan <- nil
		return true
	}

	// If our ring doesn't know, it must be empty.  We will have initiated the
	// bootstrap of the ring, so wait until we find some owner for this
	// range (might be us).
	owner := alloc.ring.Owner(c.addr)
	if owner == router.UnknownPeerName {
		alloc.infof("Ring is empty; will try later.\n", c.addr, owner)
		return false
	}
	if owner != alloc.ourName {
		c.resultChan <- fmt.Errorf("Address %s is owned by other peer %s", c.addr.String(), owner)
		return true
	}
	// We are the owner, check we haven't given it to another container
	existingIdent := alloc.findOwner(c.addr)
	if existingIdent == c.ident {
		// same identifier is claiming same address; that's OK
		c.resultChan <- nil
		return true
	}
	if existingIdent == "" {
		err := alloc.space.Claim(c.addr)
		if err != nil {
			c.resultChan <- err
			return true
		}
		alloc.addOwned(c.ident, c.addr)
		c.resultChan <- nil
		return true
	}
	// Addr already owned by container on this machine
	c.resultChan <- fmt.Errorf("Claimed address %s is already owned by %s", c.addr.String(), existingIdent)
	return true
}

func (c *claim) Cancel() {
	c.resultChan <- fmt.Errorf("Operation cancelled.")
}

func (c *claim) String() string {
	return fmt.Sprintf("Claim %s -> %s", c.ident, c.addr.String())
}

func (c *claim) ForContainer(ident string) bool {
	return c.ident == ident
}
