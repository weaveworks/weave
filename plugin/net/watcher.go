package plugin

import (
	"fmt"

	weaveapi "github.com/weaveworks/weave/api"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
)

const (
	WeaveDomain = "weave.local"
)

type watcher struct {
	client *docker.Client
	weave  *weaveapi.Client
	driver *driver
}

type Watcher interface {
}

func NewWatcher(client *docker.Client, driver *driver) (Watcher, error) {
	resolver := func() (string, error) { return client.GetContainerIP(WeaveContainer) }
	w := &watcher{client: client, driver: driver,
		weave: weaveapi.NewClientWithResolver(resolver)}
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
	// check that it's on our network, via the endpointID
	for _, net := range info.NetworkSettings.Networks {
		if w.driver.HasEndpoint(net.EndpointID) {
			fqdn := fmt.Sprintf("%s.%s", info.Config.Hostname, info.Config.Domainname)
			if err := w.weave.RegisterWithDNS(id, fqdn, net.IPAddress); err != nil {
				Log.Warningf("unable to register with weaveDNS: %s", err)
			}
		}
	}
}

func (w *watcher) ContainerDied(id string) {
	// don't need to do this as WeaveDNS removes names on container died anyway
	// (note by the time we get this event we can't see the EndpointID)
}

func (w *watcher) ContainerDestroyed(id string) {}
