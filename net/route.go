package net

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// A network is considered free if it does not overlap any existing
// routes on this host. This is the same approach taken by Docker.
func CheckNetworkFree(subnet *net.IPNet, ignoreIfName string) error {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.Dst != nil && overlaps(route.Dst, subnet) {
			if link, err := netlink.LinkByIndex(route.LinkIndex); err == nil && link.Attrs().Name == ignoreIfName {
				continue
			}

			return fmt.Errorf("Network %s overlaps with existing route %s on host.", subnet, route.Dst)
		}
	}
	return nil
}

// Two networks overlap if the start-point of one is inside the other.
func overlaps(n1, n2 *net.IPNet) bool {
	return n1.Contains(n2.IP) || n2.Contains(n1.IP)
}

// For a specific address, we only care if it is actually *inside* an
// existing route, because weave-local traffic never hits IP routing.
func CheckAddressOverlap(addr net.IP, ignoreIfName string) error {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.Dst != nil && route.Dst.Contains(addr) {
			if link, err := netlink.LinkByIndex(route.LinkIndex); err == nil && link.Attrs().Name == ignoreIfName {
				continue
			}
			return fmt.Errorf("Address %s overlaps with existing route %s on host.", addr, route.Dst)
		}
	}
	return nil
}
