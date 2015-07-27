package docker

import (
	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

// An observer for container events
type ContainerObserver interface {
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

	env, err := client.Version()
	if err != nil {
		return nil, err
	}
	Log.Infof("[docker] Using Docker API on %s: %v", apiPath, env)
	return client, nil
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
