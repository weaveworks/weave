package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

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
	_, ok := container.Labels[cniNetwork]
	return ok
}

func (proxy *Proxy) attachCNI(container *docker.Container, orDie bool) {
	network := container.Labels[cniNetwork]

	// read the json config to find the plugin exe
	conf, err := libcni.LoadConf(cniConfPath, network)
	if err != nil {
		Log.Warningf("Attaching container %s using CNI plugin failed: %s", container.ID, string(stderr))
		return errors.New(string(stderr))
	}

	// tell plugin to attach container
	c := RuntimeConf.CNIConfig{Path: cniPluginPath}
	_, err := c.AddNetwork(conf, libcni.RuntimeConf{
		ContainerID: container.ID,
		NetNS:       fmt.Printf("/proc/net/%s/ns", container.Status.PID),
		IfName:      cniIfName,
	})

	if err != nil {
		Log.Warningf("Attaching container %s using CNI plugin failed: %s", container.ID, string(stderr))
		return errors.New(string(stderr))
	}
	return nil
}
