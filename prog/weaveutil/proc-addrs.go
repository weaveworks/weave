package main

import (
	"strconv"

	"github.com/weaveworks/weave/common"
	weavenet "github.com/weaveworks/weave/net"
)

func processAddrs(args []string) error {
	if len(args) < 1 {
		cmdUsage("process-addrs", "<bridgeName>")
	}
	bridgeName := args[0]

	pred, err := common.ConnectedToBridgePredicate(bridgeName)
	if err != nil {
		if err == weavenet.ErrLinkNotFound {
			return nil
		}
		return err
	}

	pids, err := common.AllPids("/proc")
	if err != nil {
		return err
	}

	// NB: Because network namespaces (netns) are changed many times inside the loop,
	//     it's NOT safe to exec any code depending on the root netns without
	//     wrapping with WithNetNS*.
	for _, pid := range pids {
		netDevs, err := common.GetNetDevsWithPredicate(pid, pred)
		if err != nil {
			return err
		}
		printNetDevs(strconv.Itoa(pid), netDevs)
	}
	return nil
}
