package net

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// create and attach a veth to the Weave bridge
func CreateAndAttachVeth(procPath, name, peerName, bridgeName string, mtu int, keepTXOn bool, errIfLinkExist bool, init func(peer netlink.Link) error) (*netlink.Veth, error) {
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

	linkAdd := LinkAddIfNotExist
	if errIfLinkExist {
		linkAdd = netlink.LinkAdd
	}
	if err := linkAdd(veth); err != nil {
		return nil, fmt.Errorf(`could not create veth pair %s-%s: %s`, name, peerName, err)
	}

	cleanup := func(format string, a ...interface{}) (*netlink.Veth, error) {
		netlink.LinkDel(veth)
		return nil, fmt.Errorf(format, a...)
	}

	bridgeType, err := ExistingBridgeType(bridgeName, DatapathName)
	if err != nil {
		return cleanup("detect bridge type: %s", err)
	}
	if err := bridgeType.attach(veth); err != nil {
		return cleanup("attaching veth %q to %q: %s", name, bridgeName, err)
	}
	// No ipv6 router advertisments please
	if err := sysctl(procPath, "net/ipv6/conf/"+name+"/accept_ra", "0"); err != nil {
		return cleanup("setting accept_ra to 0: %s", err)
	}
	if err := sysctl(procPath, "net/ipv6/conf/"+peerName+"/accept_ra", "0"); err != nil {
		return cleanup("setting accept_ra to 0: %s", err)
	}
	if !bridgeType.IsFastdp() && !keepTXOn {
		if err := EthtoolTXOff(veth.PeerName); err != nil {
			return cleanup(`unable to set tx off on %q: %s`, peerName, err)
		}
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

func interfaceExistsInNamespace(netNSPath string, ifName string) bool {
	err := WithNetNSByPath(netNSPath, func() error {
		_, err := netlink.LinkByName(ifName)
		return err
	})
	return err == nil
}

func AttachContainer(netNSPath, id, ifName, bridgeName string, mtu int, withMulticastRoute bool, cidrs []*net.IPNet, keepTXOn bool, hairpinMode bool) error {
	// AttachContainer expects to be called in host pid namespace
	const procPath = "/proc"

	ns, err := netns.GetFromPath(netNSPath)
	if err != nil {
		return err
	}
	defer ns.Close()

	if !interfaceExistsInNamespace(netNSPath, ifName) {
		maxIDLen := IFNAMSIZ - 1 - len(vethPrefix+"pl")
		if len(id) > maxIDLen {
			id = id[:maxIDLen] // trim passed ID if too long
		}
		name, peerName := vethPrefix+"pl"+id, vethPrefix+"pg"+id
		veth, err := CreateAndAttachVeth(procPath, name, peerName, bridgeName, mtu, keepTXOn, true, func(veth netlink.Link) error {
			if err := netlink.LinkSetNsFd(veth, int(ns)); err != nil {
				return fmt.Errorf("failed to move veth to container netns: %s", err)
			}
			if err := WithNetNS(ns, func() error {
				return setupIface(procPath, peerName, ifName)
			}); err != nil {
				return fmt.Errorf("error setting up interface: %s", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if err = netlink.LinkSetHairpin(veth, hairpinMode); err != nil {
			return fmt.Errorf("unable to set hairpin mode to %t for bridge side of veth %s: %s", hairpinMode, name, err)
		}

	}

	if err := WithNetNSLink(ns, ifName, func(veth netlink.Link) error {
		return setupIfaceAddrs(veth, withMulticastRoute, cidrs)
	}); err != nil {
		return fmt.Errorf("error setting up interface addresses: %s", err)
	}
	return nil
}

// setupIfaceAddrs expects to be called in the container's netns
func setupIfaceAddrs(veth netlink.Link, withMulticastRoute bool, cidrs []*net.IPNet) error {
	newAddresses, err := AddAddresses(veth, cidrs)
	if err != nil {
		return err
	}

	ifName := veth.Attrs().Name
	ipt, err := iptables.New()
	if err != nil {
		return err
	}

	// Add multicast ACCEPT rules for new subnets
	for _, ipnet := range newAddresses {
		acceptRule := []string{"-i", ifName, "-s", subnet(ipnet), "-d", "224.0.0.0/4", "-j", "ACCEPT"}
		exists, err := ipt.Exists("filter", "INPUT", acceptRule...)
		if err != nil {
			return err
		}
		if !exists {
			if err := ipt.Insert("filter", "INPUT", 1, acceptRule...); err != nil {
				return err
			}
		}
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
}

// setupIface expects to be called in the container's netns
func setupIface(procPath, ifaceName, newIfName string) error {
	ipt, err := iptables.New()
	if err != nil {
		return err
	}

	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetName(link, newIfName); err != nil {
		return err
	}
	if err := configureARPCache(procPath, newIfName); err != nil {
		return err
	}
	return ipt.Append("filter", "INPUT", "-i", newIfName, "-d", "224.0.0.0/4", "-j", "DROP")
}

// configureARP is a helper for the Docker plugin which doesn't set the addresses itself
func ConfigureARP(prefix, procPath string) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		ifName := link.Attrs().Name
		if strings.HasPrefix(ifName, prefix) {
			configureARPCache(procPath, ifName)
			if addrs, err := netlink.AddrList(link, netlink.FAMILY_V4); err == nil {
				for _, addr := range addrs {
					arping.GratuitousArpOverIfaceByName(addr.IPNet.IP, ifName)
				}
			}
		}
	}
	return nil
}

func DetachContainer(netNSPath, id, ifName string, cidrs []*net.IPNet) error {
	ns, err := netns.GetFromPath(netNSPath)
	if err != nil {
		return err
	}
	defer ns.Close()

	ipt, err := iptables.New()
	if err != nil {
		return err
	}

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

		// Remove multicast ACCEPT rules for subnets we no longer have addresses in
		subnets := subnets(addrs)
		rules, err := ipt.List("filter", "INPUT")
		if err != nil {
			return err
		}
		for _, rule := range rules {
			ps := strings.Split(rule, " ")
			if len(ps) == 10 &&
				ps[0] == "-A" && ps[2] == "-s" && ps[4] == "-d" && ps[5] == "224.0.0.0/4" &&
				ps[6] == "-i" && ps[7] == ifName && ps[8] == "-j" && ps[9] == "ACCEPT" {

				if _, found := subnets[ps[3]]; !found {
					if err := ipt.Delete("filter", "INPUT", ps[2:]...); err != nil {
						return err
					}
				}
			}
		}

		if len(addrs) == 0 { // all addresses gone: remove the interface
			if err := ipt.Delete("filter", "INPUT", "-i", ifName, "-d", "224.0.0.0/4", "-j", "DROP"); err != nil {
				return err
			}
			if err := netlink.LinkDel(veth); err != nil {
				return err
			}
		}
		return nil
	})
}

func subnet(ipn *net.IPNet) string {
	ones, _ := ipn.Mask.Size()
	return fmt.Sprintf("%s/%d", ipn.IP.Mask(ipn.Mask).String(), ones)
}

func subnets(addrs []netlink.Addr) map[string]struct{} {
	subnets := make(map[string]struct{})
	for _, addr := range addrs {
		subnets[subnet(addr.IPNet)] = struct{}{}
	}
	return subnets
}
