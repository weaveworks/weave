package ipam

import (
	"fmt"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

type claim struct {
	resultChan       chan<- error
	ident            string
	addr             address.Address
	noErrorOnUnknown bool
}

// Send an error (or nil for success) back to caller listening on resultChan
func (c *claim) sendResult(result error) {
	// Make sure we only send a result once, since listener stops listening after that
	if c.resultChan != nil {
		c.resultChan <- result
		close(c.resultChan)
		c.resultChan = nil
		return
	}
	if result != nil {
		common.Log.Errorln("[allocator] " + result.Error())
	}
}

// Try returns true for success (or failure), false if we need to try again later
func (c *claim) Try(alloc *Allocator) bool {
	if !alloc.ring.Contains(c.addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe", c.addr, c.ident)
		c.sendResult(nil)
		return true
	}

	alloc.establishRing()

	switch owner := alloc.ring.Owner(c.addr); owner {
	case alloc.ourName:
		// success
	case router.UnknownPeerName:
		// If our ring doesn't know, it must be empty.
		if c.noErrorOnUnknown {
			alloc.infof("Claim %s for %s: address allocator still initializing; will try later.", c.addr, c.ident)
			c.sendResult(nil)
		} else {
			c.sendResult(fmt.Errorf("%s is in the range %s, but the allocator is not initialized yet", c.addr, alloc.universe.AsCIDRString()))
		}
		return false
	default:
		alloc.debugf("requesting address %s from other peer %s", c.addr, owner)
		err := alloc.sendSpaceRequest(owner, address.NewRange(c.addr, 1))
		if err != nil { // can't speak to owner right now
			if c.noErrorOnUnknown {
				alloc.infof("Claim %s for %s: %s; will try later.", c.addr, c.ident, err)
				c.sendResult(nil)
			} else { // just tell the user they can't do this.
				c.deniedBy(alloc, owner)
			}
		}
		return false
	}

	// We are the owner, check we haven't given it to another container
	switch existingIdent := alloc.findOwner(c.addr); existingIdent {
	case "":
		if err := alloc.space.Claim(c.addr); err == nil {
			alloc.debugln("Claimed", c.addr, "for", c.ident)
			alloc.addOwned(c.ident, c.addr)
			c.sendResult(nil)
		} else {
			c.sendResult(err)
		}
	case c.ident:
		// same identifier is claiming same address; that's OK
		c.sendResult(nil)
	default:
		// Addr already owned by container on this machine
		c.sendResult(fmt.Errorf("address %s is already owned by %s", c.addr.String(), existingIdent))
	}
	return true
}

func (c *claim) deniedBy(alloc *Allocator, owner router.PeerName) {
	name, found := alloc.nicknames[owner]
	if found {
		name = " (" + name + ")"
	}
	c.sendResult(fmt.Errorf("address %s is owned by other peer %s%s", c.addr.String(), owner, name))
}

func (c *claim) Cancel() {
	c.sendResult(&errorCancelled{"Claim", c.ident})
}

func (c *claim) ForContainer(ident string) bool {
	return c.ident == ident
}
