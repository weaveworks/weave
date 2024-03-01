package main

import (
	"os"

	cni "github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"

	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	weaveapi "github.com/rajch/weave/api"
	"github.com/rajch/weave/common"
	ipamplugin "github.com/rajch/weave/plugin/ipam"
	netplugin "github.com/rajch/weave/plugin/net"
)

func cniNet(args []string) error {
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	n := netplugin.NewCNIPlugin(weave)
	cni.PluginMain(n.CmdAdd, n.CmdCheck, n.CmdDel, version.All, bv.BuildString("weave net"))
	return nil
}

func cniIPAM(args []string) error {
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	i := ipamplugin.NewIpam(weave)
	cni.PluginMain(i.CmdAdd, i.CmdCheck, i.CmdDel, version.All, bv.BuildString("weave net"))
	return nil
}
