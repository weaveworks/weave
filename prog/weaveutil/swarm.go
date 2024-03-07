package main

import (
	"fmt"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

func swarmManagerPeers(args []string) error {
	info, err := dockerInfo()
	if err != nil {
		return err
	}

	ips := make([]string, 0)

	for _, managerNode := range info.Swarm.RemoteManagers {
		ip, err := ipFromAddr(managerNode.Addr)
		if err != nil {
			return errors.Wrap(err, "ipFromAddr")
		}
		ips = append(ips, ip)
	}

	fmt.Println(strings.Join(ips, " "))

	return nil
}

func dockerInfo() (*docker.DockerInfo, error) {
	client, err := newVersionedDockerClientFromEnv(swarmDockerAPIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "docker.NewVersionedClientFromEnv")
	}

	info, err := client.Info()
	if err != nil {
		return nil, errors.Wrap(err, "docker.Info")
	}

	return info, nil
}

func ipFromAddr(addr string) (string, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid address: %q", addr)
	}

	return parts[0], nil
}
