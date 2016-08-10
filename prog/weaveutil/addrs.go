package main

import (
	"fmt"

	"github.com/fsouza/go-dockerclient"

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

	peers, err := weavenet.ConnectedToBridgePeers(bridgeName)
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
			netDev, err := weavenet.GetBridgeNetDev(bridgeName)
			if err != nil {
				return err
			}
			printNetDevs(cid, []weavenet.Dev{netDev})
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

	for _, cid := range containerIDs {
		netDevs, err := getNetDevs(client, containers[cid], peers)
		if err != nil {
			return err
		}
		printNetDevs(cid, netDevs)
	}

	return nil
}

func getNetDevs(c *docker.Client, container *docker.Container, peers []int) ([]weavenet.Dev, error) {
	if container.State.Pid == 0 {
		return nil, nil
	}
	return weavenet.GetWeaveNetDevsByPeers(container.State.Pid, peers)
}

func printNetDevs(cid string, netDevs []weavenet.Dev) {
	for _, netDev := range netDevs {
		fmt.Printf("%12s %s %s", cid, netDev.Name, netDev.MAC.String())
		for _, cidr := range netDev.CIDRs {
			prefixLength, _ := cidr.Mask.Size()
			fmt.Printf(" %v/%v", cidr.IP, prefixLength)
		}
		fmt.Println()
	}
}
