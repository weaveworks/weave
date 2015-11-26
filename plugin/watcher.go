package plugin

import (
	"fmt"
	"strings"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
)

const (
	WeaveDomain = "weave.local"
)

type watcher struct {
	dockerer
}

type Watcher interface {
}

func NewWatcher(client *docker.Client) (Watcher, error) {
	w := &watcher{
		dockerer: dockerer{
			client: client,
		},
	}
	err := client.AddObserver(w)
	if err != nil {
		return nil, err
	}

	return w, nil
}

func (w *watcher) ContainerStarted(id string) {
	Log.Debugf("Container started %s", id)
	info, err := w.client.InspectContainer(id)
	if err != nil {
		Log.Warningf("error inspecting container: %s", err)
		return
	}
	// FIXME: check that it's on our network; but, the docker client lib doesn't know about .NetworkID
	if isSubdomain(info.Config.Domainname, WeaveDomain) {
		// one of ours
		ip := info.NetworkSettings.IPAddress
		fqdn := fmt.Sprintf("%s.%s", info.Config.Hostname, info.Config.Domainname)
		if err := w.registerWithDNS(id, fqdn, ip); err != nil {
			Log.Warningf("unable to register with weaveDNS: %s", err)
		}
	}
}

func (w *watcher) ContainerDied(id string) {
	Log.Debugf("Container died %s", id)
	info, err := w.client.InspectContainer(id)
	if err != nil {
		Log.Warningf("error inspecting container: %s", err)
		return
	}
	if isSubdomain(info.Config.Domainname, WeaveDomain) {
		ip := info.NetworkSettings.IPAddress
		if err := w.deregisterWithDNS(id, ip); err != nil {
			Log.Warningf("unable to deregister with weaveDNS: %s", err)
		}
	}
}

// Cheap and cheerful way to check x is, or is a subdomain, of
// y. Neither are expected to start with a '.'.
func isSubdomain(x string, y string) bool {
	return x == y || strings.HasSuffix(x, "."+y)
}
