package main

import (
	"fmt"

	docker "github.com/fsouza/go-dockerclient"
)

func createVolumeContainer(args []string) error {
	if len(args) < 3 {
		cmdUsage("create-volume-container", "<name> <image-name-or-id> <label> [<bind-mount1> <bind-mount2> ...]")
	}
	containerName := args[0]
	image := args[1]
	label := args[2]
	bindMounts := args[3:]

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	container, err := c.InspectContainer(containerName)
	if err == nil {
		fmt.Println("volume container already exists " + containerName + ":" + container.ID)
		return nil
	}

	volumes := make(map[string]struct{})
	for _, m := range bindMounts {
		volumes[m] = struct{}{}
	}
	labels := map[string]string{label: ""}
	config := docker.Config{Image: image, Volumes: volumes, Labels: labels, Entrypoint: []string{"data-only"}}
	hostConfig := docker.HostConfig{}
	_, err = c.CreateContainer(docker.CreateContainerOptions{Name: containerName, Config: &config, HostConfig: &hostConfig})
	if err != nil {
		return fmt.Errorf("unable to create container: %s", err)
	}

	return nil
}
