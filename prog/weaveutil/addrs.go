package main

import (
	"fmt"

	"github.com/fsouza/go-dockerclient"
	"github.com/vishvananda/netlink"

	"github.com/weaveworks/weave/common"
	weavenet "github.com/weaveworks/weave/net"
)

func containerAddrs(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-addrs", "<bridgeName> [containerID ...]")
	}
	bridgeName := args[0]

	client, err := docker.NewVersionedClientFromEnv("1.18")
	if err != nil {
		return err
	}

	pred, err := common.ConnectedToBridgePredicate(bridgeName)
	if err != nil {
		if err == weavenet.ErrLinkNotFound {
			return nil
		}
		return err
	}

	var containerIDs []string
	containers := make(map[string]*docker.Container)

	for _, cid := range args[1:] {
		if cid == "weave:expose" {
			netDev, err := common.GetBridgeNetDev(bridgeName)
			if err != nil {
				return err
			}
			printNetDevs(cid, []common.NetDev{netDev})
			continue
		}
		if containers[cid], err = client.InspectContainer(cid); err != nil {
			if _, ok := err.(*docker.NoSuchContainer); ok {
				continue
			}
			return err
		}
		// To output in the right order, we keep the list of container IDs
		containerIDs = append(containerIDs, cid)
	}

	// NB: Because network namespaces (netns) are changed many times inside the loop,
	//     it's NOT safe to exec any code depending on the root netns without
	//     wrapping with WithNetNS*.
	for _, cid := range containerIDs {
		netDevs, err := getNetDevs(client, containers[cid], pred)
		if err != nil {
			return err
		}
		printNetDevs(cid, netDevs)
	}

	return nil
}

func getNetDevs(c *docker.Client, container *docker.Container, pred func(netlink.Link) bool) ([]common.NetDev, error) {
	if container.State.Pid == 0 {
		return nil, nil
	}
	return common.GetNetDevsWithPredicate(container.State.Pid, pred)
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
