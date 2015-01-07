package rendezvous

import (
	"errors"
	"fmt"
	. "github.com/zettio/weave/common"
	"net"
	"net/http"
	"net/url"
)

var (
	errMalformedUrl  = errors.New("Malformed URL")  // malformed domain url error
	errUnknownScheme = errors.New("Unknown scheme") // unknown scheme error
)

// Factories for rendezvous services
type RendezvousServiceFactory struct {
	GetWorkerUrl func(url.URL, *net.Interface) *url.URL
	NewWorker    func(*RendezvousManager, *url.URL) RendezvousWorker
}

// All the rendezvous services
var rendezvousServices = map[string]RendezvousServiceFactory{
	"mdns": {
		MDnsWorkerUrl,
		func(r *RendezvousManager, u *url.URL) RendezvousWorker { return NewMDnsWorker(r, u) },
	},
}

// Common interface for all the rendezvous workers
// There can be several workers for the same domain, depending on the service. For example,
// when joining "mdns:///somegroup", we could start a worker for "eth0" (identified by
// "mdns://eth0/somegroup") and another one for "eth1" (identified by "mdns://eth1/somegroup")
type RendezvousWorker interface {
	Start([]externalIface, *net.Interface) error
	Stop() error
}

type SimpleRendezvousWorker struct {
	Domain string // something like "mdns:///somedomain"
}

type managerEntry struct {
	group  string
	worker RendezvousWorker
}

type RendezvousManager struct {
	endpoints  []externalIface
	workers    map[string]managerEntry
	notifyChan chan string
}

// Create a new rendezvous manager
func NewRendezvousManager(endpoints []externalIface, weaveUrl *url.URL) *RendezvousManager {
	r := &RendezvousManager{
		endpoints:  endpoints,
		workers:    make(map[string]managerEntry),
		notifyChan: make(chan string),
	}

	go func() {
		fullUrl := weaveUrl
		fullUrl.Path = "/connect"

		for ip := range r.notifyChan {
			Debug.Printf("Notifying \"%s\" about \"%s\"", fullUrl.String(), ip)
			_, err := http.PostForm(fullUrl.String(), url.Values{"peer": {ip}})
			if err != nil {
				Error.Printf("Could not notify about \"%s\": err", ip, err)
			}
		}
	}()

	return r
}

// Connect to a rendezvous service/domain (ie, "mdns:///somedomain")
func (r *RendezvousManager) Connect(group string) error {
	Debug.Printf("Connecting to \"%s\"", group)
	u, err := url.Parse(group)
	if err != nil {
		Error.Printf("Could not get service from group name \"%s\"", group)
		return errMalformedUrl
	}

	factory, found := rendezvousServices[u.Scheme]
	if !found {
		Error.Printf("Unknown rendezvous scheme %s", u.Scheme)
		return errUnknownScheme
	}

	// For some services we must start a worker per domain, but for some others we
	// must start a worker per domain _and_ public interface pair.
	// For example, we will start a "mdns" client and server on "eth0", "eth1", etc for
	// a domain "somegroup".
	// On each interface, we will announce all the IP addresses of all public interfaces
	for _, ep := range r.endpoints {
		iface := ep.iface
		workerUrl := factory.GetWorkerUrl(*u, iface)
		workerUrlStr := workerUrl.String()
		if _, found := r.workers[workerUrlStr]; found {
			continue
		}

		Debug.Printf("Starting new worker for \"%s\"", workerUrlStr)
		worker := factory.NewWorker(r, workerUrl)
		if err := worker.Start(r.endpoints, iface); err != nil {
			Error.Printf("Failed rendezvous worker initialization for \"%s\": %s", workerUrlStr, err)
			worker.Stop()
		} else {
			r.workers[workerUrlStr] = managerEntry{group, worker}
		}
	}

	return nil
}

// Leave a rendezvous service/domain (ie, "mdns:///somedomain")
func (r *RendezvousManager) Leave(group string) error {
	Debug.Printf("Leaving group %s", group)
	for key, entry := range r.workers {
		if entry.group == group  {
			Debug.Printf("... stopping %s", key)
			entry.worker.Stop()
			delete(r.workers, key)
		}
	}
	return nil
}

// Notify the router about a new peer
func (r *RendezvousManager) notifyAbout(ip string) error {
	r.notifyChan <- ip
	return nil
}

// Stop all the rendezvous workers
func (r *RendezvousManager) Stop() error {
	Debug.Printf("Stopping all workers in rendezvous manager")
	for key, entry := range r.workers {
		Debug.Printf("... stopping %s", key)
		entry.worker.Stop()
		delete(r.workers, key)
	}
	close(r.notifyChan)
	return nil
}

// Return a simple status
func (r *RendezvousManager) Status() string {
	return fmt.Sprintf("%d workers", len(r.workers))
}
