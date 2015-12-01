package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"

	. "github.com/weaveworks/weave/common"
)

// An observer for container events
type ContainerObserver interface {
	ContainerStarted(ident string)
	ContainerDied(ident string)
}

type Client struct {
	*docker.Client
}

// NewClient creates a new Docker client and checks we can talk to Docker
func NewClient(apiPath string) (*Client, error) {
	dc, err := docker.NewClient(apiPath)
	if err != nil {
		return nil, err
	}
	client := &Client{dc}

	return client, client.checkWorking(apiPath)
}

func NewVersionedClient(apiPath string, apiVersionString string) (*Client, error) {
	dc, err := docker.NewVersionedClient(apiPath, apiVersionString)
	if err != nil {
		return nil, err
	}
	client := &Client{dc}

	return client, client.checkWorking(apiPath)
}

func (c *Client) checkWorking(apiPath string) error {
	env, err := c.Version()
	if err != nil {
		return err
	}
	Log.Infof("[docker] Using Docker API on %s: %v", apiPath, env)
	return nil
}

// AddObserver adds an observer for docker events
func (c *Client) AddObserver(ob ContainerObserver) error {
	events := make(chan *docker.APIEvents)
	if err := c.AddEventListener(events); err != nil {
		Log.Errorf("[docker] Unable to add listener to Docker API: %s", err)
		return err
	}

	go func() {
		for event := range events {
			switch event.Status {
			case "start":
				id := event.ID
				ob.ContainerStarted(id)
			case "die":
				id := event.ID
				ob.ContainerDied(id)
			}
		}
	}()
	return nil
}

// IsContainerNotRunning returns true if we have checked with Docker that the ID is not running
func (c *Client) IsContainerNotRunning(idStr string) bool {
	container, err := c.InspectContainer(idStr)
	if err == nil {
		return !container.State.Running
	}
	if _, notThere := err.(*docker.NoSuchContainer); notThere {
		return true
	}
	Log.Errorf("[docker] Could not check container status: %s", err)
	return false
}

// This is intended to find an IP address that we can reach the container on;
// if it is on the Docker bridge network then that address; if on the host network
// then localhost
func (c *Client) GetContainerIP(nameOrID string) (string, error) {
	Log.Debugf("Getting IP for container %s", nameOrID)
	info, err := c.InspectContainer(nameOrID)
	if err != nil {
		return "", err
	}
	if info.NetworkSettings.Networks != nil {
		Log.Debugln("Networks: ", info.NetworkSettings.Networks)
		if bridgeNetwork, ok := info.NetworkSettings.Networks["bridge"]; ok {
			return bridgeNetwork.IPAddress, nil
		} else if _, ok := info.NetworkSettings.Networks["host"]; ok {
			return "127.0.0.1", nil
		}
	} else if info.HostConfig.NetworkMode == "host" {
		return "127.0.0.1", nil
	}
	if info.NetworkSettings.IPAddress == "" {
		return "", errors.New("No IP address found for container " + nameOrID)
	}
	return info.NetworkSettings.IPAddress, nil
}
