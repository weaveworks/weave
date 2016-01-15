/* various weave docker network plugin operations */
package main

import (
	"fmt"

	"github.com/fsouza/go-dockerclient"
)

func removePluginNetwork(args []string) error {
	if len(args) != 1 {
		cmdUsage("remove-plugin-network", "<network-name>")
	}
	networkName := args[0]
	d, err := newDockerClient()
	if err != nil {
		return err
	}
	_, err = d.NetworkInfo(networkName)
	if err != nil {
		// network probably doesn't exist; TODO check this better
		return nil
	}
	err = d.RemoveNetwork(networkName)
	if err != nil {
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
