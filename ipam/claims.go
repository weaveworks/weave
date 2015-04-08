package ipam

import (
	"fmt"
	"net"

	"github.com/zettio/weave/router"
)

type pendingClaim struct {
	resultChan chan<- error
	Ident      string
	IP         net.IP
}

type claimList []pendingClaim

func (aa *claimList) removeAt(pos int) {
	(*aa) = append((*aa)[:pos], (*aa)[pos+1:]...)
}

func (aa *claimList) find(addr net.IP) int {
	for i, a := range *aa {
		if a.IP.Equal(addr) {
			return i
		}
	}
	return -1
}

// Claim an address that we think we should own
func (alloc *Allocator) handleClaim(ident string, addr net.IP, resultChan chan<- error) {
	if !alloc.ring.Contains(addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe", addr, ident)
		resultChan <- nil
		return
	}
	// See if it's already claimed
	if pos := alloc.claims.find(addr); pos >= 0 && alloc.claims[pos].Ident != ident {
		resultChan <- fmt.Errorf("IP address %s already claimed by %s", addr, alloc.claims[pos].Ident)
		return
	}
	alloc.infof("Address %s claimed by %s", addr, ident)
	if owner, err := alloc.checkClaim(ident, addr); err != nil {
		resultChan <- err
	} else if owner != alloc.ourName {
		alloc.infof("Address %s owned by %s", addr, owner)
		alloc.claims = append(alloc.claims, pendingClaim{resultChan, ident, addr})
	} else {
		resultChan <- nil
	}
}

func (alloc *Allocator) handleCancelClaim(ident string, addr net.IP) {
	for i, claim := range alloc.claims {
		if claim.Ident == ident && claim.IP.Equal(addr) {
			alloc.claims = append(alloc.claims[:i], alloc.claims[i+1:]...)
			break
		}
	}
}

func (alloc *Allocator) checkClaim(ident string, addr net.IP) (owner router.PeerName, err error) {
	if owner := alloc.ring.Owner(addr); owner == alloc.ourName {
		// We own the space; see if we already have an owner for that particular address
		if existingIdent := alloc.findOwner(addr); existingIdent == "" {
			alloc.addOwned(ident, addr)
			err := alloc.spaceSet.Claim(addr)
			return alloc.ourName, err
		} else if existingIdent == ident { // same identifier is claiming same address; that's OK
			return alloc.ourName, nil
		} else {
			return alloc.ourName, fmt.Errorf("Claimed address %s is already owned by %s", addr, existingIdent)
		}
	} else {
		return owner, nil
	}
}

func (alloc *Allocator) checkClaims() {
	for i := 0; i < len(alloc.claims); i++ {
		owner, err := alloc.checkClaim(alloc.claims[i].Ident, alloc.claims[i].IP)
		if err != nil || owner == alloc.ourName {
			alloc.claims[i].resultChan <- err
			alloc.claims = append(alloc.claims[:i], alloc.claims[i+1:]...)
			i--
		}
	}
}
