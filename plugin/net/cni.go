package plugin

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"

	"github.com/appc/cni/pkg/ipam"
	"github.com/appc/cni/pkg/skel"
	"github.com/appc/cni/pkg/types"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	weaveapi "github.com/weaveworks/weave/api"
	weavenet "github.com/weaveworks/weave/net"
	ipamplugin "github.com/weaveworks/weave/plugin/ipam"
)

var (
	zeroNetwork = net.IPNet{IP: net.IPv4zero, Mask: net.IPv4Mask(0, 0, 0, 0)}
	mask32      = net.IPv4Mask(0xff, 0xff, 0xff, 0xff)
)

type CNIPlugin struct {
	weave *weaveapi.Client
}

func NewCNIPlugin(weave *weaveapi.Client) *CNIPlugin {
	return &CNIPlugin{weave: weave}
}

func loadNetConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{
		BrName: weavenet.WeaveBridgeName,
	}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return n, nil
}

func (c *CNIPlugin) getIP(ipamType string, args *skel.CmdArgs) (result *types.Result, err error) {
	// Default IPAM is Weave's own
	if ipamType == "" {
		result, err = ipamplugin.NewIpam(c.weave).Allocate(args)
	} else {
		result, err = ipam.ExecAdd(ipamType, args.StdinData)
	}
	if err == nil && result.IP4 == nil {
		return nil, fmt.Errorf("IPAM plugin failed to allocate IP address")
	}
	return result, err
}

func (c *CNIPlugin) CmdAdd(args *skel.CmdArgs) error {
	conf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if conf.IsGW {
		return fmt.Errorf("Gateway functionality not supported")
	}
	if conf.IPMasq {
		return fmt.Errorf("IP Masquerading functionality not supported")
	}

	result, err := c.getIP(conf.IPAM.Type, args)
	if err != nil {
		return fmt.Errorf("unable to allocate IP address: %s", err)
	}

	// If config says nothing about routes or gateway, default one will be via the bridge
	if result.IP4.Routes == nil && result.IP4.Gateway == nil {
		bridgeIP, err := findBridgeIP(conf.BrName, result.IP4.IP)
		if err == errBridgeNoIP {
			bridgeArgs := *args
			bridgeArgs.ContainerID = "weave:expose"
			bridgeIPResult, err := c.getIP(conf.IPAM.Type, &bridgeArgs)
			if err != nil {
				return fmt.Errorf("unable to allocate IP address for bridge: %s", err)
			}
			if err := assignBridgeIP(conf.BrName, bridgeIPResult.IP4.IP); err != nil {
				return fmt.Errorf("unable to assign IP address to bridge: %s", err)
			}
			bridgeIP = bridgeIPResult.IP4.IP.IP
		} else if err != nil {
			return err
		}
		result.IP4.Gateway = bridgeIP
	}

	ns, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return err
	}
	defer ns.Close()

	id := args.ContainerID
	if len(id) < 5 {
		data := make([]byte, 5)
		_, err := rand.Reader.Read(data)
		if err != nil {
			return err
		}
		id = fmt.Sprintf("%x", data)
	}

	if err := weavenet.AttachContainer(ns, args.Netns, id, args.IfName, conf.BrName, conf.MTU, false, []*net.IPNet{&result.IP4.IP}, false); err != nil {
		return err
	}
	if err := weavenet.WithNetNSLinkUnsafe(ns, args.IfName, func(link netlink.Link) error {
		return setupRoutes(link, args.IfName, result.IP4.IP, result.IP4.Gateway, result.IP4.Routes)
	}); err != nil {
		return fmt.Errorf("error setting up routes: %s", err)
	}

	result.DNS = conf.DNS
	return result.Print()
}

func setupRoutes(link netlink.Link, name string, ipnet net.IPNet, gw net.IP, routes []types.Route) error {
	var err error
	if routes == nil { // If config says nothing about routes, add a default one
		if !ipnet.Contains(gw) {
			// The bridge IP is not on the same subnet; add a specific route to it
			gw32 := &net.IPNet{IP: gw, Mask: mask32}
			if err = weavenet.AddRoute(link, netlink.SCOPE_LINK, gw32, nil); err != nil {
				return err
			}
		}
		routes = []types.Route{{Dst: zeroNetwork}}
	}
	for _, r := range routes {
		if r.GW != nil {
			err = weavenet.AddRoute(link, netlink.SCOPE_UNIVERSE, &r.Dst, r.GW)
		} else {
			err = weavenet.AddRoute(link, netlink.SCOPE_UNIVERSE, &r.Dst, gw)
		}
		if err != nil {
			return fmt.Errorf("failed to add route '%v via %v dev %v': %v", r.Dst, gw, name, err)
		}
	}
	return nil
}

func assignBridgeIP(bridgeName string, ipnet net.IPNet) error {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return err
	}
	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: &ipnet}); err != nil {
		return fmt.Errorf("failed to add IP address to %q: %v", bridgeName, err)
	}
	return nil
}

var errBridgeNoIP = fmt.Errorf("Bridge has no IP address")

func findBridgeIP(bridgeName string, subnet net.IPNet) (net.IP, error) {
	netdev, err := weavenet.GetBridgeNetDev(bridgeName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get netdev for %q bridge: %s", bridgeName, err)
	}
	if len(netdev.CIDRs) == 0 {
		return nil, errBridgeNoIP
	}
	for _, cidr := range netdev.CIDRs {
		if subnet.Contains(cidr.IP) {
			return cidr.IP, nil
		}
	}
	// None in the required subnet; just return the first one
	return netdev.CIDRs[0].IP, nil
}

func (c *CNIPlugin) CmdDel(args *skel.CmdArgs) error {
	conf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if _, err = weavenet.WithNetNS(args.Netns, "del-iface", args.IfName); err != nil {
		return fmt.Errorf("error removing interface: %s", err)
	}

	// Default IPAM is Weave's own
	if conf.IPAM.Type == "" {
		err = ipamplugin.NewIpam(c.weave).Release(args)
	} else {
		err = ipam.ExecDel(conf.IPAM.Type, args.StdinData)
	}
	if err != nil {
		return fmt.Errorf("unable to release IP address: %s", err)
	}
	return nil
}

type NetConf struct {
	types.NetConf
	BrName string `json:"bridge"`
	IsGW   bool   `json:"isGateway"`
	IPMasq bool   `json:"ipMasq"`
	MTU    int    `json:"mtu"`
}
