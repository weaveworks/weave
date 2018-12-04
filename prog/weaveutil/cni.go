package main

import (
	"os"

	cni "github.com/appc/cni/pkg/skel"

	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	ipamplugin "github.com/weaveworks/weave/plugin/ipam"
	netplugin "github.com/weaveworks/weave/plugin/net"
)

func cniNet(args []string) error {
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	n := netplugin.NewCNIPlugin(weave)
	cni.PluginMain(n.CmdAdd, n.CmdDel)
	return nil
}

func cniIPAM(args []string) error {
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	i := ipamplugin.NewIpam(weave)
	cni.PluginMain(i.CmdAdd, i.CmdDel)
	return nil
}
