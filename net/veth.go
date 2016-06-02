package net

import (
	"fmt"
	"net"
	"time"

	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"github.com/weaveworks/weave/common/odp"
)

// create and attach a veth to the Weave bridge
func CreateAndAttachVeth(name, peerName, bridgeName string, mtu int, keepTXOn bool, init func(peer netlink.Link) error) (*netlink.Veth, error) {
	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, fmt.Errorf(`bridge "%s" not present; did you launch weave?`, bridgeName)
	}

	if mtu == 0 {
		mtu = bridge.Attrs().MTU
	}
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  mtu},
		PeerName: peerName,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return nil, fmt.Errorf(`could not create veth pair %s-%s: %s`, name, peerName, err)
	}

	cleanup := func(format string, a ...interface{}) (*netlink.Veth, error) {
		netlink.LinkDel(veth)
		return nil, fmt.Errorf(format, a...)
	}

	switch bridgeType := DetectBridgeType(bridgeName, DatapathName); bridgeType {
	case Bridge, BridgedFastdp:
		if err := netlink.LinkSetMasterByIndex(veth, bridge.Attrs().Index); err != nil {
			return cleanup(`unable to set master of %s: %s`, name, err)
		}
		if bridgeType == Bridge && !keepTXOn {
			if err := EthtoolTXOff(peerName); err != nil {
				return cleanup(`unable to set tx off on %q: %s`, peerName, err)
			}
		}
	case Fastdp:
		if err := odp.AddDatapathInterface(bridgeName, name); err != nil {
			return cleanup(`failed to attach %s to device "%s": %s`, name, bridgeName, err)
		}
	default:
		return cleanup(`invalid bridge configuration`)
	}

	if init != nil {
		peer, err := netlink.LinkByName(peerName)
		if err != nil {
			return cleanup("unable to find peer veth %s: %s", peerName, err)
		}
		if err := init(peer); err != nil {
			return cleanup("initializing veth: %s", err)
		}
	}

	if err := netlink.LinkSetUp(veth); err != nil {
		return cleanup("unable to bring veth up: %s", err)
	}

	return veth, nil
}

func AddAddresses(link netlink.Link, cidrs []*net.IPNet) (newAddrs []*net.IPNet, err error) {
	existingAddrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP address for %q: %v", link.Attrs().Name, err)
	}
	for _, ipnet := range cidrs {
		if contains(existingAddrs, ipnet) {
			continue
		}
		if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: ipnet}); err != nil {
			return nil, fmt.Errorf("failed to add IP address to %q: %v", link.Attrs().Name, err)
		}
		newAddrs = append(newAddrs, ipnet)
	}
	return newAddrs, nil
}

func contains(addrs []netlink.Addr, addr *net.IPNet) bool {
	for _, x := range addrs {
		if addr.IP.Equal(x.IPNet.IP) {
			return true
		}
	}
	return false
}

const (
	VethName   = "ethwe"        // name inside container namespace
	vethPrefix = "v" + VethName // starts with "veth" to suppress UI notifications
)

func interfaceExistsInNamespace(ns netns.NsHandle, ifName string) bool {
	err := WithNetNS(ns, func() error {
		_, err := netlink.LinkByName(ifName)
		return err
	})
	return err == nil
}

func AttachContainer(ns netns.NsHandle, id, ifName, bridgeName string, mtu int, withMulticastRoute bool, cidrs []*net.IPNet, keepTXOn bool) error {
	if !interfaceExistsInNamespace(ns, ifName) {
		maxIDLen := IFNAMSIZ - 1 - len(vethPrefix+"pl")
		if len(id) > maxIDLen {
			id = id[:maxIDLen] // trim passed ID if too long
		}
		name, peerName := vethPrefix+"pl"+id, vethPrefix+"pg"+id
		_, err := CreateAndAttachVeth(name, peerName, bridgeName, mtu, keepTXOn, func(veth netlink.Link) error {
			if err := netlink.LinkSetNsFd(veth, int(ns)); err != nil {
				return fmt.Errorf("failed to move veth to container netns: %s", err)
			}
			if err := WithNetNS(ns, func() error {
				if err := netlink.LinkSetName(veth, ifName); err != nil {
					return err
				}
				if err := ConfigureARPCache(ifName); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return fmt.Errorf("error setting up interface: %s", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	if err := WithNetNSLink(ns, ifName, func(veth netlink.Link) error {
		newAddresses, err := AddAddresses(veth, cidrs)
		if err != nil {
			return err
		}
		if err := netlink.LinkSetUp(veth); err != nil {
			return err
		}
		for _, ipnet := range newAddresses {
			// If we don't wait for a bit here, we see the arp fail to reach the bridge.
			time.Sleep(1 * time.Millisecond)
			arping.GratuitousArpOverIfaceByName(ipnet.IP, ifName)
		}
		if withMulticastRoute {
			/* Route multicast packets across the weave network.
			This must come last in 'attach'. If you change this, change weavewait to match.

			TODO: Add the MTU lock to prevent PMTU discovery for multicast
			destinations. Without that, the kernel sets the DF flag on
			multicast packets. Since RFC1122 prohibits sending of ICMP
			errors for packets with multicast destinations, that causes
			packets larger than the PMTU to be dropped silently.  */

			_, multicast, _ := net.ParseCIDR("224.0.0.0/4")
			if err := AddRoute(veth, netlink.SCOPE_LINK, multicast, nil); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func DetachContainer(ns netns.NsHandle, id, ifName string, cidrs []*net.IPNet) error {
	return WithNetNSLink(ns, ifName, func(veth netlink.Link) error {
		existingAddrs, err := netlink.AddrList(veth, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("failed to get IP address for %q: %v", veth.Attrs().Name, err)
		}
		for _, ipnet := range cidrs {
			if !contains(existingAddrs, ipnet) {
				continue
			}
			if err := netlink.AddrDel(veth, &netlink.Addr{IPNet: ipnet}); err != nil {
				return fmt.Errorf("failed to remove IP address from %q: %v", veth.Attrs().Name, err)
			}
		}
		addrs, err := netlink.AddrList(veth, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("failed to get IP address for %q: %v", veth.Attrs().Name, err)
		}
		if len(addrs) == 0 { // all addresses gone: remove the interface
			if err := netlink.LinkDel(veth); err != nil {
				return err
			}
		}
		return nil
	})
}
