package net

import (
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/go-iptables/iptables"
	"github.com/j-keck/arping"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"

	"github.com/weaveworks/weave/common/odp"
)

type BridgeType int

/* This code implements three possible configurations to connect
   containers to the Weave Net overlay:

1. Bridge
                 +-------+
(container-veth)-+ weave +-(vethwe-bridge)--(vethwe-pcap)
                 +-------+

"weave" is a Linux bridge. "vethwe-pcap" (end of veth pair) is used
to capture and inject packets, by router/pcap.go.

2. BridgedFastdp

                 +-------+                                    /----------\
(container-veth)-+ weave +-(vethwe-bridge)--(vethwe-datapath)-+ datapath +
                 +-------+                                    \----------/

"weave" is a Linux bridge and "datapath" is an Open vSwitch datapath;
they are connected via a veth pair. Packet capture and injection use
the "datapath" device, via "router/fastdp.go:fastDatapathBridge"

3. Fastdp

                 /-------\
(container-veth)-+ weave +
                 \-------/

"weave" is an Open vSwitch datapath, and capture/injection are as in
BridgedFastdp. Not used by default due to missing conntrack support in
datapath of old kernel versions (https://github.com/weaveworks/weave/issues/1577).
*/

const (
	WeaveBridgeName = "weave"
	DatapathName    = "datapath"
	DatapathIfName  = "vethwe-datapath"
	BridgeIfName    = "vethwe-bridge"
	PcapIfName      = "vethwe-pcap"

	None BridgeType = iota
	Bridge
	Fastdp
	BridgedFastdp
	AWSVPC
	Inconsistent
)

// Returns a string that is consistent with the weave script
func (t BridgeType) String() string {
	switch t {
	case None:
		return "none"
	case Bridge:
		return "bridge"
	case Fastdp:
		return "fastdp"
	case BridgedFastdp:
		return "bridged_fastdp"
	case AWSVPC:
		return "AWSVPC"
	case Inconsistent:
		return "inconsistent"
	}
	return "unknown"
}

func DetectBridgeType(weaveBridgeName, datapathName string) BridgeType {
	bridge, _ := netlink.LinkByName(weaveBridgeName)
	datapath, _ := netlink.LinkByName(datapathName)

	switch {
	case bridge == nil && datapath == nil:
		return None
	case isBridge(bridge) && datapath == nil:
		return Bridge
	case isDatapath(bridge) && datapath == nil:
		return Fastdp
	case isDatapath(datapath) && isBridge(bridge):
		return BridgedFastdp
		// We cannot detect AWSVPC here; it looks just like Bridge.
		// Anyone who cares will have to know some other way.
	default:
		return Inconsistent
	}
}

func EnforceAddrAssignType(bridgeName string) (setAddr bool, err error) {
	sysctlFilename := filepath.Join("/sys/class/net/", bridgeName, "/addr_assign_type")
	addrAssignType, err := ioutil.ReadFile(sysctlFilename)
	if err != nil {
		return false, errors.Wrapf(err, "reading %q", sysctlFilename)
	}

	// From include/uapi/linux/netdevice.h
	// #define NET_ADDR_PERM       0   /* address is permanent (default) */
	// #define NET_ADDR_RANDOM     1   /* address is generated randomly */
	// #define NET_ADDR_STOLEN     2   /* address is stolen from other device */
	// #define NET_ADDR_SET        3   /* address is set using dev_set_mac_address() */
	// Note the file typically has a newline at the end, so we just look at the first char
	if addrAssignType[0] != '3' {
		link, err := netlink.LinkByName(bridgeName)
		if err != nil {
			return false, errors.Wrapf(err, "EnforceAddrAssignType finding bridge %s", bridgeName)
		}

		mac, err := RandomMAC()
		if err != nil {
			return false, errors.Wrap(err, "creating random MAC")
		}

		if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
			return false, errors.Wrapf(err, "setting bridge %s address to %v", bridgeName, mac)
		}
		return true, nil
	}

	return false, nil
}

func isBridge(link netlink.Link) bool {
	_, isBridge := link.(*netlink.Bridge)
	return isBridge
}

func isDatapath(link netlink.Link) bool {
	switch link.(type) {
	case *netlink.GenericLink:
		return link.Type() == "openvswitch"
	case *netlink.Device:
		// Assume it's our openvswitch device, and the kernel has not been updated to report the kind.
		return true
	default:
		return false
	}
}

func DetectHairpin(portIfName string, log *logrus.Logger) error {
	link, err := netlink.LinkByName(portIfName)
	if err != nil {
		return fmt.Errorf("Unable to find link %q: %s", portIfName, err)
	}

	ch := make(chan netlink.LinkUpdate)
	// See EnsureInterface for why done channel is not passed
	if err := netlink.LinkSubscribe(ch, nil); err != nil {
		return fmt.Errorf("Unable to subscribe to netlink updates: %s", err)
	}

	pi, err := netlink.LinkGetProtinfo(link)
	if err != nil {
		return fmt.Errorf("Unable to get link protinfo %q: %s", portIfName, err)
	}
	if pi.Hairpin {
		return fmt.Errorf("Hairpin mode enabled on %q", portIfName)
	}

	go func() {
		for up := range ch {
			if up.Attrs().Name == portIfName && up.Attrs().Protinfo != nil &&
				up.Attrs().Protinfo.Hairpin {
				log.Errorf("Hairpin mode enabled on %q", portIfName)
			}
		}
	}()

	return nil
}

var ErrBridgeNoIP = fmt.Errorf("Bridge has no IP address")

func FindBridgeIP(bridgeName string, subnet *net.IPNet) (net.IP, error) {
	netdev, err := GetBridgeNetDev(bridgeName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get netdev for %q bridge: %s", bridgeName, err)
	}
	if len(netdev.CIDRs) == 0 {
		return nil, ErrBridgeNoIP
	}
	if subnet != nil {
		for _, cidr := range netdev.CIDRs {
			if subnet.Contains(cidr.IP) {
				return cidr.IP, nil
			}
		}
	}
	// No subnet, or none in the required subnet; just return the first one
	return netdev.CIDRs[0].IP, nil
}

type BridgeConfig struct {
	DockerBridgeName string
	WeaveBridgeName  string
	DatapathName     string
	NoFastdp         bool
	NoBridgedFastdp  bool
	AWSVPC           bool
	NPC              bool
	MTU              int
	Mac              string
	Port             int
}

func CreateBridge(procPath string, config *BridgeConfig) (BridgeType, error) {
	bridgeType := DetectBridgeType(config.WeaveBridgeName, config.DatapathName)

	if bridgeType == None {
		bridgeType = Bridge
		if !config.NoFastdp {
			bridgeType = BridgedFastdp
			if config.NoBridgedFastdp {
				bridgeType = Fastdp
				config.DatapathName = config.WeaveBridgeName
			}
			odpSupported, err := odp.CreateDatapath(config.DatapathName)
			if err != nil {
				return None, errors.Wrapf(err, "creating datapath %q", config.DatapathName)
			}
			if !odpSupported {
				bridgeType = Bridge
			}
		}

		var err error
		switch bridgeType {
		case Bridge:
			err = initBridge(config)
		case Fastdp:
			err = initFastdp(config)
		case BridgedFastdp:
			err = initBridgedFastdp(config)
		default:
			err = fmt.Errorf("Cannot initialise bridge type %v", bridgeType)
		}
		if err != nil {
			return None, err
		}

		if err = configureIPTables(config); err != nil {
			return bridgeType, errors.Wrap(err, "configuring iptables")
		}
	}

	if config.AWSVPC {
		bridgeType = AWSVPC
		// Set proxy_arp on the bridge, so that it could accept packets destined
		// to containers within the same subnet but running on remote hosts.
		// Without it, exact routes on each container are required.
		if err := sysctl(procPath, "net/ipv4/conf/"+config.WeaveBridgeName+"/proxy_arp", "1"); err != nil {
			return bridgeType, errors.Wrap(err, "setting proxy_arp")
		}
		// Avoid delaying the first ARP request. Also, setting it to 0 avoids
		// placing the request into a bounded queue as it can be seen:
		// https://git.kernel.org/cgit/linux/kernel/git/stable/linux-stable.git/tree/net/ipv4/arp.c?id=refs/tags/v4.6.1#n819
		if err := sysctl(procPath, "net/ipv4/neigh/"+config.WeaveBridgeName+"/proxy_delay", "0"); err != nil {
			return bridgeType, errors.Wrap(err, "setting proxy_arp")
		}
	}

	if bridgeType == Bridge {
		if err := EthtoolTXOff(config.WeaveBridgeName); err != nil {
			return bridgeType, errors.Wrap(err, "setting tx off")
		}
	}

	if err := linkSetUpByName(config.WeaveBridgeName); err != nil {
		return bridgeType, err
	}

	if err := ConfigureARPCache(procPath, config.WeaveBridgeName); err != nil {
		return bridgeType, errors.Wrapf(err, "configuring ARP cache on bridge %q", config.WeaveBridgeName)
	}

	return bridgeType, nil
}

func initBridgePrep(config *BridgeConfig) error {
	mac, err := net.ParseMAC(config.Mac)
	if err != nil {
		return errors.Wrapf(err, "parsing bridge MAC %q", config.Mac)
	}

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = config.WeaveBridgeName
	linkAttrs.HardwareAddr = mac
	if config.MTU == 0 {
		config.MTU = 65535
	}
	link := &netlink.Bridge{LinkAttrs: linkAttrs}
	if err = netlink.LinkAdd(link); err != nil {
		return errors.Wrapf(err, "creating bridge %q", config.WeaveBridgeName)
	}
	if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
		return errors.Wrapf(err, "setting bridge %q mac %v", config.WeaveBridgeName, mac)
	}
	// Attempting to set the bridge MTU to a high value directly
	// fails. Bridges take the lowest MTU of their interfaces. So
	// instead we create a temporary interface with the desired MTU,
	// attach that to the bridge, and then remove it again.
	dummy := &netlink.Dummy{LinkAttrs: netlink.NewLinkAttrs()}
	dummy.LinkAttrs.Name = "vethwedu"
	if err = netlink.LinkAdd(dummy); err != nil {
		return errors.Wrap(err, "creating dummy interface")
	}
	if err := netlink.LinkSetMTU(dummy, config.MTU); err != nil {
		return errors.Wrapf(err, "setting dummy interface mtu to %d", config.MTU)
	}
	if err := netlink.LinkSetMaster(dummy, link); err != nil {
		return errors.Wrap(err, "setting dummy interface master")
	}
	if err := netlink.LinkDel(dummy); err != nil {
		return errors.Wrap(err, "deleting dummy interface")
	}

	return nil
}

func initBridge(config *BridgeConfig) error {
	if err := initBridgePrep(config); err != nil {
		return err
	}
	if _, err := CreateAndAttachVeth(BridgeIfName, PcapIfName, config.WeaveBridgeName, config.MTU, true, func(veth netlink.Link) error {
		return netlink.LinkSetUp(veth)
	}); err != nil {
		return errors.Wrap(err, "creating pcap veth pair")
	}

	return nil
}

func initFastdp(config *BridgeConfig) error {
	datapath, err := netlink.LinkByName(config.DatapathName)
	if err != nil {
		return errors.Wrapf(err, "finding datapath %q", config.DatapathName)
	}
	if config.MTU == 0 {
		/* GCE has the lowest underlay network MTU we're likely to encounter on
		   a local network, at 1460 bytes.  To get the overlay MTU from that we
		   subtract 20 bytes for the outer IPv4 header, 8 bytes for the outer
		   UDP header, 8 bytes for the vxlan header, and 14 bytes for the inner
		   ethernet header.  In addition, we subtract 34 bytes for the ESP overhead
		   which is needed for the vxlan encryption. */
		config.MTU = 1376
	}
	if err := netlink.LinkSetMTU(datapath, config.MTU); err != nil {
		return errors.Wrapf(err, "setting datapath %q mtu %d", config.DatapathName, config.MTU)
	}
	return nil
}

func initBridgedFastdp(config *BridgeConfig) error {
	if err := initFastdp(config); err != nil {
		return err
	}
	if err := initBridgePrep(config); err != nil {
		return err
	}
	if _, err := CreateAndAttachVeth(BridgeIfName, DatapathIfName, config.WeaveBridgeName, config.MTU, true, func(veth netlink.Link) error {
		if err := netlink.LinkSetUp(veth); err != nil {
			return errors.Wrapf(err, "setting link up on %q", veth.Attrs().Name)
		}
		if err := odp.AddDatapathInterface(config.DatapathName, veth.Attrs().Name); err != nil {
			return errors.Wrapf(err, "adding interface %q to datapath %q", veth.Attrs().Name, config.DatapathName)
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "creating bridged fastdp veth pair")
	}

	if err := linkSetUpByName(config.DatapathName); err != nil {
		return err
	}

	return nil
}

func configureIPTables(config *BridgeConfig) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrap(err, "creating iptables object")
	}
	if config.DockerBridgeName != "" {
		if config.WeaveBridgeName != config.DockerBridgeName {
			err := ipt.Insert("filter", "FORWARD", 1, "-i", config.DockerBridgeName, "-o", config.WeaveBridgeName, "-j", "DROP")
			if err != nil {
				return err
			}
		}

		dockerBridgeIP, err := FindBridgeIP(config.DockerBridgeName, nil)
		if err != nil {
			return err
		}

		// forbid traffic to the Weave port from other containers
		if err = ipt.AppendUnique("filter", "INPUT", "-i", config.DockerBridgeName, "-p", "tcp", "--dst", dockerBridgeIP.String(), "--dport", fmt.Sprint(config.Port), "-j", "DROP"); err != nil {
			return err
		}
		if err = ipt.AppendUnique("filter", "INPUT", "-i", config.DockerBridgeName, "-p", "udp", "--dst", dockerBridgeIP.String(), "--dport", fmt.Sprint(config.Port), "-j", "DROP"); err != nil {
			return err
		}
		if err = ipt.AppendUnique("filter", "INPUT", "-i", config.DockerBridgeName, "-p", "udp", "--dst", dockerBridgeIP.String(), "--dport", fmt.Sprint(config.Port+1), "-j", "DROP"); err != nil {
			return err
		}

		// let DNS traffic to weaveDNS, since otherwise it might get blocked by the likes of UFW
		if err = ipt.AppendUnique("filter", "INPUT", "-i", config.DockerBridgeName, "-p", "udp", "--dport", "53", "-j", "ACCEPT"); err != nil {
			return err
		}
		if err = ipt.AppendUnique("filter", "INPUT", "-i", config.DockerBridgeName, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	if config.NPC {
		// Steer traffic via the NPC
		if err = ipt.ClearChain("nat", "WEAVE-NPC"); err != nil {
			return errors.Wrap(err, "clearing WEAVE-NPC chain")
		}
		if err = ipt.AppendUnique("filter", "FORWARD", "-o", config.WeaveBridgeName, "-j", "WEAVE-NPC"); err != nil {
			return err
		}
		if err = ipt.AppendUnique("filter", "FORWARD", "-o", config.WeaveBridgeName, "-m", "state", "--state", "NEW", "-j", "NFLOG", "--nflog-group", "86"); err != nil {
			return err
		}
		if err = ipt.AppendUnique("filter", "FORWARD", "-o", config.WeaveBridgeName, "-j", "DROP"); err != nil {
			return err
		}
	} else {
		// Work around the situation where there are no rules allowing traffic
		// across our bridge. E.g. ufw
		if err = ipt.AppendUnique("filter", "FORWARD", "-i", config.WeaveBridgeName, "-o", config.WeaveBridgeName, "-j", "ACCEPT"); err != nil {
			return err
		}
		// Forward from weave to the rest of the world
		if err = ipt.AppendUnique("filter", "FORWARD", "-i", config.WeaveBridgeName, "!", "-o", config.WeaveBridgeName, "-j", "ACCEPT"); err != nil {
			return err
		}
		// and allow replies back
		if err = ipt.AppendUnique("filter", "FORWARD", "-o", config.WeaveBridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	// create a chain for masquerading
	if err = ipt.ClearChain("nat", "WEAVE"); err != nil {
		return errors.Wrap(err, "clearing WEAVE chain")
	}
	if err = ipt.AppendUnique("nat", "POSTROUTING", "-j", "WEAVE"); err != nil {
		return err
	}

	return nil
}

func linkSetUpByName(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return errors.Wrapf(err, "setting link up on %q", linkName)
	}
	return netlink.LinkSetUp(link)
}

func AddBridgeAddr(bridgeName string, addr *net.IPNet, removeDefaultRoute bool) error {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return errors.Wrapf(err, "AddBridgeAddr finding bridge %q", bridgeName)
	}
	newAddresses, err := AddAddresses(link, []*net.IPNet{addr})
	if err != nil {
		return errors.Wrapf(err, "adding address %v to %q", addr, bridgeName)
	}
	for _, ipnet := range newAddresses {
		arping.GratuitousArpOverIfaceByName(ipnet.IP, bridgeName)
	}
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
			if err = netlink.RouteDel(&r); err != nil {
				return errors.Wrapf(err, "failed to delete default route %+v", r)
			}
		}
	}
	return nil
}
