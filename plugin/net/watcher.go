package plugin

import (
	"fmt"

	weaveapi "github.com/rajch/weave/api"
	"github.com/rajch/weave/common/docker"
	weavenet "github.com/rajch/weave/net"
)

type watcher struct {
	client *docker.Client
	weave  *weaveapi.Client
	driver *driver
}

type Watcher interface {
}

func NewWatcher(client *docker.Client, weave *weaveapi.Client, driver *driver) (Watcher, error) {
	w := &watcher{client: client, weave: weave, driver: driver}
	return w, client.AddObserver(w)
}

func (w *watcher) ContainerStarted(id string) {
	w.driver.debug("ContainerStarted", "%s", id)
	info, err := w.client.InspectContainer(id)
	if err != nil {
		w.driver.warn("ContainerStarted", "error inspecting container %s: %s", id, err)
		return
	}

	domain, err := w.weave.DNSDomain()
	if err != nil {
		w.driver.warn("ContainerStarted", "unable to get weave dns domain: %s", err)
	}

	// check that it's on our network
	for _, net := range info.NetworkSettings.Networks {
		network, err := w.driver.findNetworkInfo(net.NetworkID)
		if err != nil {
			w.driver.warn("ContainerStarted", "unable to find network %s info: %s", net.NetworkID, err)
			continue
		}
		if network.isOurs {
			if w.driver.dns {
				fqdn := fmt.Sprintf("%s.%s", info.Config.Hostname, info.Config.Domainname)

				aliases := make([]string, 0, len(net.Aliases)+2)
				aliases = append(aliases, fqdn)

				if len(domain) > 0 && len(info.Name) > 1 && info.Name[0] == '/' {
					name := fmt.Sprintf("%s.%s", info.Name[1:], domain)
					aliases = append(aliases, name)
				}

				aliases = append(aliases, net.Aliases...)

				for _, alias := range aliases {
					w.driver.debug("ContainerStarted", "going to register %s with weaveDNS", alias)
					if err := w.weave.RegisterWithDNS(id, alias, net.IPAddress); err != nil {
						w.driver.warn("ContainerStarted", "unable to register %s with weaveDNS: %s", id, err)
					}
				}
			}
			netNSPath := weavenet.NSPathByPidWithProc(w.driver.procPath, info.State.Pid)
			if err := weavenet.WithNetNSByPath(netNSPath, func() error {
				return weavenet.ConfigureARP(weavenet.VethName, w.driver.procPath)
			}); err != nil {
				w.driver.warn("ContainerStarted", "unable to configure interfaces: %s", err)
			}
		}
	}
}

func (w *watcher) ContainerDied(id string) {
	// don't need to do this as WeaveDNS removes names on container died anyway
	// (note by the time we get this event we can't see the EndpointID)
}

func (w *watcher) ContainerDestroyed(id string) {}
