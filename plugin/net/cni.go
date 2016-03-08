package plugin

import (
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/appc/cni/pkg/ipam"
	"github.com/appc/cni/pkg/skel"
	"github.com/appc/cni/pkg/types"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	ipamplugin "github.com/weaveworks/weave/plugin/ipam"
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

	err = common.WithNetNS(ns, func() error {
		guest, err := netlink.LinkByName(local.PeerName)
		if err != nil {
			return cleanup(err)
		}
		if err = netlink.LinkSetName(guest, args.IfName); err != nil {
			return err
		}
		return ipam.ConfigureIface(args.IfName, result)
	})
	if err != nil {
		return cleanup(errorf("error setting up interface: %s", err))
	}

	result.DNS = conf.DNS
	return result.Print()
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
