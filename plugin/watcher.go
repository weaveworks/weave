package plugin

import (
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

const (
	WeaveDNSContainer = "weavedns"
	WeaveDomain       = "weave.local"
)

type watcher struct {
	dockerer
	networks map[string]bool
	events   chan *docker.APIEvents
}

type Watcher interface {
}

func NewWatcher(client *docker.Client) (Watcher, error) {
	w := &watcher{
		dockerer: dockerer{
			client: client,
		},
		networks: make(map[string]bool),
		events:   make(chan *docker.APIEvents),
	}
	err := client.AddEventListener(w.events)
	if err != nil {
		return nil, err
	}

	go func() {
		for event := range w.events {
			switch event.Status {
			case "start":
				w.ContainerStart(event.ID)
			case "die":
				w.ContainerDied(event.ID)
			}
		}
	}()

	return w, nil
}

func (w *watcher) ContainerStart(id string) {
	Log.Debugf("Container started %s", id)
	info, err := w.InspectContainer(id)
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
	info, err := w.InspectContainer(id)
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
