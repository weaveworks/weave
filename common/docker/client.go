package docker

import (
	"regexp"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

// Regexp that matches container IDs
var containerIDregexp = regexp.MustCompile("^[a-f0-9]+$")

// IsContainerID returns True if the string provided is a valid container id
func IsContainerID(idStr string) bool {
	return containerIDregexp.MatchString(idStr)
}

// An observer for container events
type ContainerObserver interface {
	ContainerDied(ident string) error
}

type Client struct {
	*docker.Client
}

// NewClient creates a new Docker client
func NewClient(apiPath string) (*Client, error) {
	dc, err := docker.NewClient(apiPath)
	if err != nil {
		return nil, err
	}
	return &Client{dc}, nil
}

// Start starts the client
func (c *Client) Start(apiPath string) error {
	client, err := docker.NewClient(apiPath)
	if err != nil {
		Error.Printf("[docker] Unable to connect to Docker API on %s: %s", apiPath, err)
		return err
	}

	env, err := client.Version()
	if err != nil {
		Error.Printf("[docker] Unable to connect to Docker API on %s: %s", apiPath, err)
		return err
	}
	Info.Printf("[docker] Using Docker API on %s: %v", apiPath, env)
	return nil
}

// AddObserver adds an observer for docker events
func (c *Client) AddObserver(ob ContainerObserver) error {
	events := make(chan *docker.APIEvents)
	if err := c.AddEventListener(events); err != nil {
		Error.Printf("[docker] Unable to add listener to Docker API: %s", err)
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

// IsContainerRunning returns true if the container is up and running
func (c *Client) IsContainerRunning(idStr string) (bool, error) {
	// check with Docker whether the container is really running
	container, err := c.InspectContainer(idStr)
	if err != nil {
		if _, notThere := err.(*docker.NoSuchContainer); notThere {
			return false, nil
		}
		return false, err
	}

	return container.State.Running, nil
}
