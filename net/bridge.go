package net

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"

	"github.com/weaveworks/weave/common/odp"
)

type BridgeType int

const (
	WeaveBridgeName = "weave"
	DatapathName    = "datapath"
	BridgeIfName    = "vethwe-bridge"
	PcapIfaceName   = "vethwe-pcap"

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

var ErrSetBridgeMac = errors.New("Setting $DOCKER_BRIDGE MAC (mitigate https://github.com/docker/docker/issues/14908)")

func EnforceAddrAssignType(bridgeName string) error {
	addrAssignType, err := ioutil.ReadFile(fmt.Sprintf("/sys/class/net/%s/addr_assign_type", bridgeName))
	if err != nil {
		return err
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
			return err
		}

		mac, err := RandomMAC()
		if err != nil {
			return err
		}

		if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
			return err
		}
		return ErrSetBridgeMac
	}

	return nil
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
	IsAWSVPC         bool
	ExpectNPC        bool
	MTU              int
	Mac              string
	Port             int
}

func CreateBridge(config *BridgeConfig) (BridgeType, error) {
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
				return None, err
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
			return bridgeType, err
		}
	}

	if config.IsAWSVPC {
		bridgeType = AWSVPC
	}

	if bridgeType == Bridge {
		if err := EthtoolTXOff(config.WeaveBridgeName); err != nil {
			return bridgeType, err
		}
	}

	if err := linkSetUpByName(config.WeaveBridgeName); err != nil {
		return bridgeType, err
	}

	if err := ConfigureARPCache(config.WeaveBridgeName); err != nil {
		return bridgeType, err
	}

	return bridgeType, nil
}

func initBridgePrep(config *BridgeConfig) error {
	mac, err := net.ParseMAC(config.Mac)
	if err != nil {
		return err
	}

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = config.WeaveBridgeName
	linkAttrs.HardwareAddr = mac
	if config.MTU == 0 {
		config.MTU = 65535
	}
	linkAttrs.MTU = config.MTU // TODO this probably doesn't work - see weave script
	if err = netlink.LinkAdd(&netlink.Bridge{LinkAttrs: linkAttrs}); err != nil {
		return err
	}

	return nil
}

func initBridge(config *BridgeConfig) error {
	if err := initBridgePrep(config); err != nil {
		return err
	}
	if _, err := CreateAndAttachVeth(BridgeIfName, PcapIfaceName, config.WeaveBridgeName, config.MTU, true, func(veth netlink.Link) error {
		return netlink.LinkSetUp(veth)
	}); err != nil {
		return err
	}

	return nil
}

func initFastdp(config *BridgeConfig) error {
	datapath, err := netlink.LinkByName(config.DatapathName)
	if err != nil {
		return err
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
	return netlink.LinkSetMTU(datapath, config.MTU)
}

func initBridgedFastdp(config *BridgeConfig) error {
	if err := initFastdp(config); err != nil {
		return err
	}
	if err := initBridgePrep(config); err != nil {
		return err
	}
	if _, err := CreateAndAttachVeth("vethwe-bridge", "vethwe-datapath", config.WeaveBridgeName, config.MTU, true, func(veth netlink.Link) error {
		if err := netlink.LinkSetUp(veth); err != nil {
			return err
		}
		if err := odp.AddDatapathInterface(config.DatapathName, veth.Attrs().Name); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := linkSetUpByName(config.DatapathName); err != nil {
		return err
	}

	return nil
}

func configureIPTables(config *BridgeConfig) error {
	ipt, err := iptables.New()
	if err != nil {
		return err
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

	if config.ExpectNPC {
		// Steer traffic via the NPC
		ipt.NewChain("filter", "WEAVE-NPC") // ignoring errors such as 'chain already exists'
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
	ipt.NewChain("nat", "WEAVE") // ignoring errors such as 'chain already exists'
	if err = ipt.AppendUnique("nat", "POSTROUTING", "-j", "WEAVE"); err != nil {
		return err
	}

	return nil
}

func linkSetUpByName(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}
