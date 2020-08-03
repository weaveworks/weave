package net

import (
	"net"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/j-keck/arping"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

// Expose makes the network accessible from a host by assigning a given IP address
// to the weave bridge.
//
// List of params:
// * "bridgeName" - a name of the weave bridge.
// * "ipAddr" - IP addr to be assigned to the bridge.
// * "removeDefaultRoute" - whether to remove a default route installed by the kernel (used only in the AWSVPC mode).
// * "npc" - whether is Weave NPC running.
// * "skipNAT" - whether to skip adding iptables NAT rules
func Expose(bridgeName string, ipAddr *net.IPNet, removeDefaultRoute, npc bool, skipNAT bool) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrap(err, "iptables.New")
	}
	cidr := ipAddr.String()

	if err := addBridgeIPAddr(bridgeName, ipAddr, removeDefaultRoute); err != nil {
		return errors.Wrap(err, "addBridgeIPAddr")
	}

	if !skipNAT {
		if err := exposeNAT(ipt, cidr); err != nil {
			return errors.Wrap(err, "exposeNAT")
		}
	}

	if !npc {
		// Docker 1.13 has changed a default policy of FORWARD chain to DROP
		// (https://github.com/moby/moby/pull/28257) which makes containers
		// inaccessible from a remote host when the bridge is exposed.
		//
		// The change breaks e.g. the AWSVPC mode. To overcome this we install
		// an explicit rule for accepting forwarded ingress traffic to an
		// exposed subnet.
		if err := ipt.AppendUnique("filter", "WEAVE-EXPOSE", "-d", cidr, "-j", "ACCEPT"); err != nil {
			return errors.Wrap(err, "ipt.AppendUnique")
		}
	}

	return nil
}

func addBridgeIPAddr(bridgeName string, addr *net.IPNet, removeDefaultRoute bool) error {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return errors.Wrapf(err, "addBridgeIPAddr finding bridge %q", bridgeName)
	}
	err = netlink.AddrAdd(link, &netlink.Addr{IPNet: addr})
	// The IP addr might have been already set by a concurrent request, just ignore then
	if err != nil && err != syscall.Errno(syscall.EEXIST) {
		return errors.Wrapf(err, "adding address %v to %q", addr, bridgeName)
	}

	// Sending multiple ARP REQUESTs in the case of EEXIST above does not hurt
	arping.GratuitousArpOverIfaceByName(addr.IP, bridgeName)

	// Remove a default route installed by the kernel. Required by the AWSVPC mode.
	if removeDefaultRoute {
		routeFilter := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       &net.IPNet{IP: addr.IP.Mask(addr.Mask), Mask: addr.Mask},
			Protocol:  2, // RTPROT_KERNEL
		}
		filterMask := netlink.RT_FILTER_OIF | netlink.RT_FILTER_DST | netlink.RT_FILTER_PROTOCOL
		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, routeFilter, filterMask)
		if err != nil {
			return errors.Wrapf(err, "failed to get route list for bridge %q", bridgeName)
		}
		for _, r := range routes {
			err = netlink.RouteDel(&r)
			// Again, there might be a concurrent request for removing routes
			if err != nil && err != syscall.Errno(syscall.ESRCH) {
				return errors.Wrapf(err, "failed to delete default route %+v", r)
			}
		}
	}
	return nil
}

func exposeNAT(ipt *iptables.IPTables, cidr string) error {
	if err := addNatRule(ipt, "-s", cidr, "-d", "224.0.0.0/4", "-j", "RETURN"); err != nil {
		return err
	}
	if err := addNatRule(ipt, "-d", cidr, "!", "-s", cidr, "-j", "MASQUERADE"); err != nil {
		return err
	}
	return addNatRule(ipt, "-s", cidr, "!", "-d", cidr, "-j", "MASQUERADE")
}

func addNatRule(ipt *iptables.IPTables, rulespec ...string) error {
	// Loop until we get an exit code other than "temporarily unavailable"
	for {
		if err := ipt.AppendUnique("nat", "WEAVE", rulespec...); err != nil {
			if isResourceError(err) {
				continue
			}
			return err
		}
		return nil
	}
}
