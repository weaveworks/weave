package plugin

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ipam"
	weaveapi "github.com/rajch/weave/api"
	weavenet "github.com/rajch/weave/net"
	ipamplugin "github.com/rajch/weave/plugin/ipam"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
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
		BrName:      weavenet.WeaveBridgeName,
		HairpinMode: true,
	}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return n, nil
}

func (c *CNIPlugin) getIP(ipamType string, args *skel.CmdArgs) (newResult *current.Result, err error) {
	var result types.Result
	// Default IPAM is Weave's own
	if ipamType == "" {
		result, err = ipamplugin.NewIpam(c.weave).Allocate(args)
	} else {
		result, err = ipam.ExecAdd(ipamType, args.StdinData)
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("Received no usable result from IPAM plugin")
	}
	newResult, err = current.NewResultFromResult(result)
	// Check if ipam returned no results without error
	if err == nil && len(newResult.IPs) == 0 {
		return nil, fmt.Errorf("IPAM plugin failed to allocate IP address")
	}
	return newResult, err
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
	// Only expecting one address
	ip := result.IPs[0]

	// If config says nothing about routes or gateway, default one will be via the bridge
	if len(result.Routes) == 0 && ip.Gateway == nil {
		bridgeIP, err := weavenet.FindBridgeIP(conf.BrName, &ip.Address)
		if err == weavenet.ErrBridgeNoIP {
			bridgeArgs := *args
			bridgeArgs.ContainerID = "weave:expose"
			// It would be better if libcni let us send just the desired parameters,
			// but there is a bug: https://github.com/containernetworking/cni/issues/410
			// so just blank out the one we want to change
			os.Setenv("CNI_CONTAINERID", bridgeArgs.ContainerID)
			bridgeIPResult, err := c.getIP(conf.IPAM.Type, &bridgeArgs)
			if err != nil {
				return fmt.Errorf("unable to allocate IP address for bridge: %s", err)
			}
			bridgeCIDR := bridgeIPResult.IPs[0].Address

			if err := c.weave.Expose(&bridgeCIDR); err != nil {
				return fmt.Errorf("unable to expose bridge %q: %s", conf.BrName, err)
			}

			bridgeIP = bridgeCIDR.IP
		} else if err != nil {
			return err
		}
		result.IPs[0].Gateway = bridgeIP
	}

	ns, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return fmt.Errorf("error accessing namespace %q: %s", args.Netns, err)
	}
	defer ns.Close()

	hostNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("error accessing host network namespace: %s", err)
	}
	defer hostNs.Close()

	if ns.Equal(hostNs) {
		return fmt.Errorf("can not specify host network namespace as network namespace to which container should be added")
	}

	id := args.ContainerID
	if len(id) < 5 {
		data := make([]byte, 5)
		_, err := rand.Reader.Read(data)
		if err != nil {
			return err
		}
		id = fmt.Sprintf("%x", data)
	}

	if err := weavenet.AttachContainer(args.Netns, id, args.IfName, conf.BrName, conf.MTU, false, []*net.IPNet{&ip.Address}, false, conf.HairpinMode); err != nil {
		return err
	}
	if err := weavenet.WithNetNSLink(ns, args.IfName, func(link netlink.Link) error {
		return setupRoutes(link, args.IfName, ip.Address, ip.Gateway, result.Routes)
	}); err != nil {
		return fmt.Errorf("error setting up routes: %s", err)
	}

	result.DNS = conf.DNS
	return types.PrintResult(result, conf.CNIVersion)
}

func setupRoutes(link netlink.Link, name string, ipnet net.IPNet, gw net.IP, routes []*types.Route) error {
	var err error
	if routes == nil { // If config says nothing about routes, add a default one
		if !ipnet.Contains(gw) {
			// The bridge IP is not on the same subnet; add a specific route to it
			gw32 := &net.IPNet{IP: gw, Mask: mask32}
			if err = weavenet.AddRoute(link, netlink.SCOPE_LINK, gw32, nil); err != nil {
				return err
			}
		}
		routes = []*types.Route{{Dst: zeroNetwork}}
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

func (c *CNIPlugin) CmdCheck(args *skel.CmdArgs) error {
	// TODO: Figure out how to do a proper CNI check here
	// For now, return not implemented error

	return errors.New("CNI plugin check not implemented")
}

// As of CNI 0.5 spec:
//
//	"Plugins should generally complete a DEL action without error even if some resources are missing"
//
// this method should therefore return nil in most, if not all, cases.
func (c *CNIPlugin) CmdDel(args *skel.CmdArgs) error {
	conf, err := loadNetConf(args.StdinData)
	if err != nil {
		logOnStderr(err)
		return nil
	}

	// As of CNI 0.3 spec, runtimes can send blank if they just want the address deallocated
	if args.Netns != "" {
		if err = weavenet.WithNetNSByPath(args.Netns, func() error {
			link, err := netlink.LinkByName(args.IfName)
			if err != nil {
				return err
			}
			return netlink.LinkDel(link)
		}); err != nil {
			// We log the error and carry on instead of returning nil, as there may still be resources to free in IPAM.
			logOnStderr(fmt.Errorf("error removing interface %q: %s", args.IfName, err))
		}
	}

	// Default IPAM is Weave's own
	if conf.IPAM.Type == "" {
		err = ipamplugin.NewIpam(c.weave).Release(args)
	} else {
		err = ipam.ExecDel(conf.IPAM.Type, args.StdinData)
	}
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("Delete: no addresses for %s", args.ContainerID)) {
			// ignore the error in the case where there are no IP addresses associated with container
			fmt.Fprintln(os.Stderr, "weave-cni:", fmt.Sprintf("Delete: no addresses for %s", args.ContainerID))
			return nil
		}
		logOnStderr(fmt.Errorf("unable to release IP address: %s", err))
		return err
	}
	return nil
}

// Standard error is inherited from the caller (e.g. Kubelet, when running Weave Net's CNI plugin
// under Kubernetes) hence provided errors should eventually appear in the caller's log files.
func logOnStderr(err error) {
	fmt.Fprintln(os.Stderr, "weave-cni:", err)
}

type NetConf struct {
	types.NetConf
	BrName      string `json:"bridge"`
	IsGW        bool   `json:"isGateway"`
	IPMasq      bool   `json:"ipMasq"`
	MTU         int    `json:"mtu"`
	HairpinMode bool   `json:"hairpinMode"`
}
