package plugin

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/appc/cni/pkg/ipam"
	"github.com/appc/cni/pkg/skel"
	"github.com/appc/cni/pkg/types"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
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
		BrName: "weave",
	}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, errorf("failed to load netconf: %v", err)
	}
	return n, nil
}

func (c *CNIPlugin) CmdAdd(args *skel.CmdArgs) error {
	conf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if conf.IsGW {
		return errorf("Gateway functionality not supported")
	}
	if conf.IPMasq {
		return errorf("IP Masquerading functionality not supported")
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

	local, err := createAndAttach(id, conf.BrName, conf.MTU)
	if err != nil {
		return err
	}

	cleanup := func(err error) error {
		netlink.LinkDel(local)
		return err
	}
	guest, err := netlink.LinkByName(local.PeerName)
	if err != nil {
		return cleanup(err)
	}
	if err = netlink.LinkSetNsFd(guest, int(ns)); err != nil {
		return cleanup(errorf("failed to move veth to container netns: %s", err))
	}

	if err := netlink.LinkSetUp(local); err != nil {
		return cleanup(errorf("unable to bring veth up: %s", err))
	}

	var result *types.Result
	// Default IPAM is Weave's own
	if conf.IPAM.Type == "" {
		result, err = ipamplugin.NewIpam(c.weave).Allocate(args)
	} else {
		result, err = ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
	}
	if err != nil {
		return cleanup(errorf("unable to allocate IP address: %s", err))
	}
	if result.IP4 == nil {
		return cleanup(errorf("IPAM plugin failed to allocate IP address"))
	}

	// If config says nothing about routes or gateway, default one will be via the bridge
	if result.IP4.Routes == nil && result.IP4.Gateway == nil {
		bridgeIP, err := findBridgeIP(conf.BrName, result.IP4.IP)
		if err != nil {
			return cleanup(err)
		}
		result.IP4.Gateway = bridgeIP
	}

	err = common.WithNetNS(ns, func() error {
		return setupGuestIP4(local.PeerName, args.IfName, result.IP4.IP, result.IP4.Gateway, result.IP4.Routes)
	})
	if err != nil {
		return cleanup(errorf("error setting up interface: %s", err))
	}

	result.DNS = conf.DNS
	return result.Print()
}

func setupGuestIP4(origName, name string, ipnet net.IPNet, gw net.IP, routes []types.Route) error {
	guest, err := netlink.LinkByName(origName)
	if err != nil {
		return err
	}
	if err = netlink.LinkSetName(guest, name); err != nil {
		return err
	}
	if err = netlink.LinkSetUp(guest); err != nil {
		return err
	}
	if routes == nil { // If config says nothing about routes, add a default one
		if !ipnet.Contains(gw) {
			// The bridge IP is not on the same subnet; add a specific route to it
			gw32 := &net.IPNet{IP: gw, Mask: mask32}
			if err = addRoute(guest, netlink.SCOPE_LINK, gw32, nil); err != nil {
				return err
			}
		}
		routes = []types.Route{{Dst: zeroNetwork}}
	}
	if err = netlink.AddrAdd(guest, &netlink.Addr{IPNet: &ipnet}); err != nil {
		return fmt.Errorf("failed to add IP address to %q: %v", name, err)
	}
	for _, r := range routes {
		if r.GW != nil {
			err = addRoute(guest, netlink.SCOPE_UNIVERSE, &r.Dst, r.GW)
		} else {
			err = addRoute(guest, netlink.SCOPE_UNIVERSE, &r.Dst, gw)
		}
		if err != nil {
			return fmt.Errorf("failed to add route '%v via %v dev %v': %v", r.Dst, gw, name, err)
		}
	}
	return nil
}

func addRoute(link netlink.Link, scope netlink.Scope, dst *net.IPNet, gw net.IP) error {
	err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Scope:     scope,
		Dst:       dst,
		Gw:        gw,
	})
	if os.IsExist(err) { // squash duplicate route errors
		err = nil
	}
	return err
}

func findBridgeIP(bridgeName string, subnet net.IPNet) (net.IP, error) {
	netdevs, err := common.GetBridgeNetDev(bridgeName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get netdev for %q bridge: %s", bridgeName, err)
	}
	if len(netdevs) == 0 {
		return nil, fmt.Errorf("Could not find %q bridge", bridgeName)
	}
	if len(netdevs[0].CIDRs) == 0 {
		return nil, fmt.Errorf("Bridge %q has no IP addresses", bridgeName)
	}
	for _, cidr := range netdevs[0].CIDRs {
		if subnet.Contains(cidr.IP) {
			return cidr.IP, nil
		}
	}
	// None in the required subnet; just return the first one
	return netdevs[0].CIDRs[0].IP, nil
}

func (c *CNIPlugin) CmdDel(args *skel.CmdArgs) error {
	conf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	ns, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return err
	}
	defer ns.Close()
	err = common.WithNetNS(ns, func() error {
		link, err := netlink.LinkByName(args.IfName)
		if err != nil {
			return err
		}
		return netlink.LinkDel(link)
	})
	if err != nil {
		return errorf("error removing interface: %s", err)
	}

	// Default IPAM is Weave's own
	if conf.IPAM.Type == "" {
		err = ipamplugin.NewIpam(c.weave).Release(args)
	} else {
		err = ipam.ExecDel(conf.IPAM.Type, args.StdinData)
	}
	if err != nil {
		return errorf("unable to release IP address: %s", err)
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
