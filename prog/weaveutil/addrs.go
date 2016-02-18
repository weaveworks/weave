package main

import (
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/weaveworks/weave/common"
)

func containerAddrs(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-addrs", "<bridgeName> [containerID ...]")
	}
	bridgeName := args[0]

	c, err := docker.NewVersionedClientFromEnv("1.18")
	if err != nil {
		return err
	}

	for _, containerID := range args[1:] {
		netDevs, err := getNetDevs(bridgeName, c, containerID)
		if err != nil {
			if _, ok := err.(*docker.NoSuchContainer); ok {
				continue
			}
			return err
		}

		for _, netDev := range netDevs {
			fmt.Printf("%12s %s", containerID, netDev.MAC.String())
			for _, cidr := range netDev.CIDRs {
				prefixLength, _ := cidr.Mask.Size()
				fmt.Printf(" %v/%v", cidr.IP, prefixLength)
			}
			fmt.Println()
		}
	}

	return nil
}

func getNetDevs(bridgeName string, c *docker.Client, containerID string) ([]common.NetDev, error) {
	if containerID == "weave:expose" {
		return common.GetBridgeNetDev(bridgeName)
	}

	container, err := c.InspectContainer(containerID)
	if err != nil {
		return nil, err
	}

	return common.GetWeaveNetDevs(container.State.Pid)
}
