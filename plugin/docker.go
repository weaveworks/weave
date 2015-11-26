package plugin

import (
	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type dockerer struct {
	client *docker.Client
}

func (d *dockerer) getContainerBridgeIP(nameOrID string) (string, error) {
	Log.Debugf("Getting IP for container %s", nameOrID)
	info, err := d.InspectContainer(nameOrID)
	if err != nil {
		return "", err
	}
	return info.NetworkSettings.IPAddress, nil
}

func (d *dockerer) InspectContainer(nameOrId string) (*docker.Container, error) {
	return d.client.InspectContainer(nameOrId)
}
