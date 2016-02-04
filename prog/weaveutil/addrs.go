package main

import (
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/weaveworks/weave/common"
)

func containerAddrs(args []string) error {
	if len(args) < 2 {
		cmdUsage("container-addrs", "<procPath> <bridgeName> [containerID ...]")
	}

	procPath := args[0]
	bridgeName := args[1]

	c, err := docker.NewVersionedClientFromEnv("1.18")
	if err != nil {
		return err
	}

	for _, containerID := range args[2:] {
		netDevs, err := getNetDevs(procPath, bridgeName, c, containerID)
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

func getNetDevs(procPath, bridgeName string, c *docker.Client, containerID string) ([]common.NetDev, error) {
	if containerID == "weave:expose" {
		return common.GetBridgeNetDev(procPath, bridgeName)
	}

	container, err := c.InspectContainer(containerID)
	if err != nil {
		return nil, err
	}

	return common.GetWeaveNetDevs(procPath, container.State.Pid)
}
