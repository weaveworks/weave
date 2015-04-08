package ipam

import (
	"net"
)

func (alloc *Allocator) addOwned(ident string, addr net.IP) {
	alloc.owned[ident] = append(alloc.owned[ident], addr)
}

func (alloc *Allocator) findOwner(addr net.IP) string {
	for ident, addrs := range alloc.owned {
		for _, ip := range addrs {
			if ip.Equal(addr) {
				return ident
			}
		}
	}
	return ""
}

func (alloc *Allocator) removeOwned(ident string, addr net.IP) bool {
	if addrs, found := alloc.owned[ident]; found {
		for i, ip := range addrs {
			if ip.Equal(addr) {
				alloc.owned[ident] = append(addrs[:i], addrs[i+1:]...)
				return true
			}
		}
	}
	return false
}
