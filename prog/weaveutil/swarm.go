package main

import (
	"fmt"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

func swarmPeers(args []string) error {
	client, err := docker.NewVersionedClientFromEnv("1.26")
	if err != nil {
		return errors.Wrap(err, "docker.NewVersionedClientFromEnv")
	}

	filters := map[string][]string{
		"membership": []string{"accepted"},
		"role":       []string{"manager"},
	}
	nodes, err := client.ListNodes(docker.ListNodesOptions{Filters: filters})
	if err != nil {
		return errors.Wrap(err, "docker.ListNodes")
	}

	for _, n := range nodes {
		fmt.Println(n.Status.Addr)
	}

	return nil
}
