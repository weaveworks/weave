package proxy

import (
	"fmt"

	"github.com/appc/cni/libcni"
	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

var (
	cniScript     = "./cni.sh"
	cniNetwork    = "cni.network"
	cniConfPath   = "/etc/cni/net.d"
	cniPluginPath = []string{"/home/weave", "/usr/bin", "/"}
	cniIfName     = "ethwe" // /w/w is waiting for an interface of this name
)

func useCNI(container *docker.Container) bool {
	_, ok := container.Config.Labels[cniNetwork]
	return ok
}

func (proxy *Proxy) attachCNI(container *docker.Container, orDie bool) error {
	network := container.Config.Labels[cniNetwork]

	// read the json config to find the plugin exe
	conf, err := libcni.LoadConf(cniConfPath, network)
	if err != nil {
		Log.Warningf("Attaching container %s using CNI plugin failed: %s", container.ID, err)
		return err
	}

	// tell plugin to attach container
	c := libcni.CNIConfig{Path: cniPluginPath}
	r := &libcni.RuntimeConf{
		ContainerID: container.ID,
		NetNS:       fmt.Sprintf("/proc/net/%s/ns", container.State.Pid),
		IfName:      cniIfName,
	}

	if _, err := c.AddNetwork(conf, r); err != nil  {
		Log.Warningf("Attaching container %s using CNI plugin failed: %s", container.ID, err)
		return err
	}
	return nil
}
