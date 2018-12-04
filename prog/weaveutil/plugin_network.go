/* various weave docker network plugin operations */
package main

import (
	"fmt"

	docker "github.com/fsouza/go-dockerclient"

	"github.com/weaveworks/weave/plugin/net"
)

func createPluginNetwork(args []string) error {
	if len(args) != 3 {
		cmdUsage("create-plugin-network", "<network-name> <driver-name> <default-subnet>")
	}
	networkName := args[0]
	driverName := args[1]
	subnet := args[2]
	d, err := newDockerClient()
	if err != nil {
		return err
	}
	_, err = d.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:           networkName,
			CheckDuplicate: true,
			Driver:         driverName,
			IPAM: docker.IPAMOptions{
				Driver: driverName,
				Config: []docker.IPAMConfig{{Subnet: subnet}},
			},
			Options: map[string]interface{}{plugin.MulticastOption: "true"},
		})
	if err != docker.ErrNetworkAlreadyExists && err != nil {
		// Despite appearances to the contrary, CreateNetwork does
		// sometimes(always?) *not* return ErrNetworkAlreadyExists
		// when the network already exists. Hence we need to check for
		// this explicitly.
		if _, err2 := d.NetworkInfo(networkName); err2 != nil {
			return fmt.Errorf("unable to create network: %s", err)
		}
	}
	return nil
}

func removePluginNetwork(args []string) error {
	if len(args) != 1 {
		cmdUsage("remove-plugin-network", "<network-name>")
	}
	networkName := args[0]
	d, err := newDockerClient()
	if err != nil {
		return err
	}
	err = d.RemoveNetwork(networkName)
	if _, ok := err.(*docker.NoSuchNetwork); !ok && err != nil {
		if info, err2 := d.NetworkInfo(networkName); err2 == nil {
			if len(info.Containers) > 0 {
				containers := ""
				for container := range info.Containers {
					containers += fmt.Sprintf("  %.12s ", container)
				}
				return fmt.Errorf(`WARNING: the following containers are still attached to network %q:
%s
Docker operations involving those containers may pause or fail
while Weave is not running`, networkName, containers)
			}
		}
		return fmt.Errorf("unable to remove network: %s", err)
	}
	return nil
}

func newDockerClient() (*docker.Client, error) {
	// API 1.21 is the first version that supports docker network
	// commands
	c, err := docker.NewVersionedClientFromEnv("1.21")
	if err != nil {
		return nil, fmt.Errorf("unable to connect to docker: %s", err)
	}
	_, err = c.Version()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to docker: %s", err)
	}
	return c, nil
}
