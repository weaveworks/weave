/* various weave docker network plugin operations */
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	docker "github.com/fsouza/go-dockerclient"
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

// Exits with 0 if the given plugin (v2) is enabled.
//
// Any failure due to missing plugin support is non-harmful as plugin (v2)
// cannot be enabled when Docker does not support it.
func isDockerPluginEnabled(args []string) error {
	if len(args) != 1 {
		cmdUsage("is-docker-plugin-enabled", "<plugin-name>")
	}

	pluginName := args[0]

	// This is messed up: we are using docker/docker/client instead of
	// fsouza/go-dockerclient because the latter does not support plugins.
	c, err := client.NewEnvClient()
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	ctx := context.Background()

	plugins, err := c.PluginList(ctx, filters.Args{})
	if err != nil {
		return err
	}

	for _, p := range plugins {
		if p.Enabled && strings.Contains(p.Name, pluginName) {
			fmt.Println(p.Name)
			return nil
		}
	}

	return fmt.Errorf("plugin %q not found", pluginName)
}

func newDockerClient() (*docker.Client, error) {
	// API 1.21 was the first version that supports docker network
	// commands. In March 2024, the minimum suupported API is 1.24
	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to docker: %s", err)
	}
	_, err = c.Version()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to docker: %s", err)
	}

	return c, nil
}
