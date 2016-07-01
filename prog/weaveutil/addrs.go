package main

import (
	"fmt"

	"github.com/fsouza/go-dockerclient"
	"github.com/vishvananda/netlink"
	"github.com/weaveworks/weave/common"
)

func containerAddrs(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-addrs", "<bridgeName> [containerID ...]")
	}
	bridgeName := args[0]
	containerIDs := args[1:]

	client, err := docker.NewVersionedClientFromEnv("1.18")
	if err != nil {
		return err
	}

	pred, err := common.ConnectedToBridgePredicate(bridgeName)
	if err != nil {
		return err
	}

	containers := make(map[string]*docker.Container)
	for _, cid := range containerIDs {
		if containers[cid], err = client.InspectContainer(cid); err != nil {
			containers[cid] = nil
			if _, ok := err.(*docker.NoSuchContainer); !ok {
				return err
			}
		}
	}

	// NB: Because network namespaces (netns) are changed many times inside the loop,
	//     it's NOT safe to exec a code depending on a netns ns after the loop.
	for _, cid := range containerIDs {
		netDevs, err := getNetDevs(bridgeName, client, containers[cid], cid, pred)
		if err != nil {
			return err
		}
		printNetDevs(cid, netDevs)
	}

	return nil
}

func printNetDevs(cid string, netDevs []common.NetDev) {
	for _, netDev := range netDevs {
		fmt.Printf("%12s %s %s", cid, netDev.Name, netDev.MAC.String())
		for _, cidr := range netDev.CIDRs {
			prefixLength, _ := cidr.Mask.Size()
			fmt.Printf(" %v/%v", cidr.IP, prefixLength)
		}
		fmt.Println()
	}
}

func getNetDevs(bridgeName string, c *docker.Client, container *docker.Container, cid string, pred func(netlink.Link) bool) ([]common.NetDev, error) {
	if cid == "weave:expose" {
		netDev, err := common.GetBridgeNetDev(bridgeName)
		if err != nil {
			return nil, err
		}
		return []common.NetDev{netDev}, nil
	}

	if container == nil || container.State.Pid == 0 {
		return nil, nil
	}

	return common.GetNetDevsWithPredicate(container.State.Pid, pred)
}
