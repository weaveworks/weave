package main

import (
	"fmt"
	"os"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
)

func inspectContainer(containerNameOrID string) (*docker.Container, error) {
	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to docker: %s", err)
	}

	container, err := c.InspectContainer(containerNameOrID)
	if err != nil {
		return nil, fmt.Errorf("unable to inspect container %s: %s", containerNameOrID, err)
	}
	return container, nil
}

func containerPid(containerID string) (int, error) {
	container, err := inspectContainer(containerID)
	if err != nil {
		return 0, err

	}

	if container.State.Pid == 0 {
		return 0, fmt.Errorf("container %s not running", containerID)
	}
	return container.State.Pid, nil
}

func containerID(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-id", "<container-name-or-short-id>")
	}

	container, err := inspectContainer(args[0])
	if err != nil {
		return err
	}

	fmt.Print(container.ID)
	return nil
}

func containerState(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-state", "<container-id> [<image-prefix>]")
	}

	container, err := inspectContainer(args[0])
	if err != nil {
		return err
	}

	if len(args) == 1 {
		fmt.Print(container.State.StateString())
	} else {
		imagePrefix := args[1]
		if strings.HasPrefix(container.Config.Image, imagePrefix) {
			fmt.Print(container.State.StateString())
		} else {
			if container.State.Running {
				fmt.Print("running image mismatch: ", container.Config.Image)
			} else {
				fmt.Print("image mismatch: ", container.Config.Image)
			}
		}
	}
	return nil
}

func containerFQDN(args []string) error {
	if len(args) < 1 {
		cmdUsage("container-fqdn", "<container-id>")
	}

	container, err := inspectContainer(args[0])
	if err != nil {
		return err
	}

	fmt.Print(container.Config.Hostname, ".", container.Config.Domainname)
	return nil
}

func parseContainerArgs(args []string) docker.CreateContainerOptions {
	env := []string{}
	labels := map[string]string{}
	name := ""
	net := ""
	pid := ""
	privileged := false
	restart := docker.NeverRestart()
	volumes := []string{}
	volumesFrom := []string{}

	done := false
	for i := 0; i < len(args) && !done; {
		switch args[i] {
		case "-e", "--env":
			if v := os.Getenv(args[i+1]); v != "" {
				env = append(env, args[i+1]+"="+v)
			} else {
				env = append(env, args[i+1])
			}
			args = append(args[:i], args[i+2:]...)
		case "-l", "--label":
			key, value := parseLabel(args[i+1])
			labels[key] = value
			args = append(args[:i], args[i+2:]...)
		case "--name":
			name = args[i+1]
			args = append(args[:i], args[i+2:]...)
		case "--net":
			net = args[i+1]
			args = append(args[:i], args[i+2:]...)
		case "--pid":
			pid = args[i+1]
			args = append(args[:i], args[i+2:]...)
		case "--privileged":
			privileged = true
			args = append(args[:i], args[i+1:]...)
		case "--restart":
			restart = docker.RestartPolicy{Name: args[i+1]}
			args = append(args[:i], args[i+2:]...)
		case "-v", "--volume":
			// Must dedup binds, otherwise, container fails creation
			skip := false
			for _, v := range volumes {
				if v == args[i+1] {
					skip = true
				}
			}
			if !skip {
				volumes = append(volumes, args[i+1])
			}
			args = append(args[:i], args[i+2:]...)
		case "--volumes-from":
			volumesFrom = append(volumesFrom, args[i+1])
			args = append(args[:i], args[i+2:]...)
		default:
			done = true
		}
	}

	if len(args) < 2 {
		cmdUsage("run-container", `[options] <image> <cmd> [[<cmd-options>] <cmd-arg1> [<cmd-arg2> ...]]

  -e, --env list                       Set environment variables
      --name string                    Assign a name to the container
      --net string                     Network Mode
      --pid string                     PID namespace to use
      --privileged                     Give extended privileges to this container
      --restart string                 Restart policy to apply when a container exits (default "no")
  -v, --volume list                    Bind mount a volume
      --volumes-from list              Mount volumes from the specified container(s)
`)
	}

	image := args[0]
	cmds := args[1:]

	config := docker.Config{Image: image, Env: env, Cmd: cmds, Labels: labels}
	hostConfig := docker.HostConfig{NetworkMode: net, PidMode: pid, Privileged: privileged, RestartPolicy: restart, Binds: volumes, VolumesFrom: volumesFrom}
	return docker.CreateContainerOptions{Name: name, Config: &config, HostConfig: &hostConfig}
}

func runContainer(args []string) error {
	containerOptions := parseContainerArgs(args)

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	container, err := c.CreateContainer(containerOptions)
	if err != nil {
		return fmt.Errorf("unable to create container: %s", err)
	}

	err = c.StartContainer(container.ID, containerOptions.HostConfig)
	if err != nil {
		return fmt.Errorf("unable to start container: %s", err)
	}

	fmt.Print(container.ID)
	return nil
}

func parseLabel(s string) (key, value string) {
	pos := strings.Index(s, "=")
	if pos == -1 { // no value - set it to blank
		return s, ""
	}
	return s[:pos], s[pos+1:]
}

func listContainers(args []string) error {
	if len(args) > 1 {
		cmdUsage("list-containers", "[<label>]")
	}

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	opts := docker.ListContainersOptions{All: true}
	if len(args) == 1 {
		label := args[0]
		opts.Filters = map[string][]string{"label": {label}}
	}

	containers, err := c.ListContainers(opts)
	if err != nil {
		return fmt.Errorf("unable to list containers: %s", err)
	}

	for _, container := range containers {
		fmt.Println(container.ID)
	}
	return nil
}

func stopContainer(args []string) error {
	if len(args) < 1 {
		cmdUsage("stop-container", "<container-id> [<container-id2> ...]")
	}

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	for _, containerID := range args {
		err = c.StopContainer(containerID, 10)
		if err != nil {
			return fmt.Errorf("unable to stop container %s: %s", containerID, err)
		}
	}
	return nil
}

func killContainer(args []string) error {
	if len(args) < 1 {
		cmdUsage("kill-container", "<container-id> [<container-id2> ...]")
	}

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	for _, containerID := range args {
		err = c.KillContainer(docker.KillContainerOptions{ID: containerID, Signal: docker.SIGKILL})
		if err != nil {
			return fmt.Errorf("unable to stop container %s: %s", containerID, err)
		}
	}
	return nil
}

func removeContainer(args []string) error {
	if len(args) < 1 {
		cmdUsage("remove-container", "[-f | --force]  [-v | --volumes] <container-id> [<container-id2> ...]")
	}

	force := false
	volumes := false
	for i := 0; i < len(args); {
		switch args[i] {
		case "--force":
		case "-f":
			force = true
			args = append(args[:i], args[i+1:]...)
		case "--volumes":
		case "-v":
			volumes = true
			args = append(args[:i], args[i+1:]...)
		default:
			i++
		}
	}

	c, err := newVersionedDockerClientFromEnv(defaulDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("unable to connect to docker: %s", err)
	}

	for _, containerID := range args {
		err = c.RemoveContainer(docker.RemoveContainerOptions{
			ID: containerID, Force: force, RemoveVolumes: volumes})
		if err != nil {
			return fmt.Errorf("unable to stop container %s: %s", containerID, err)
		}
	}
	return nil
}
