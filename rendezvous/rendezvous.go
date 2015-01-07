package rendezvous

import (
	"errors"
	. "github.com/zettio/weave/common"
	"net"
	"net/url"
	"fmt"
	"net/http"
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
type RendezvousWorker interface {
	Start([]RendezvousEndpoint, *net.Interface) error
	Stop() error
}

type SimpleRendezvousWorker struct {
	Domain string // something like "mdns:///somedomain"
}

type RendezvousManager struct {
	endpoints []RendezvousEndpoint
	workers   map[string]RendezvousWorker
	notifyChan chan string
}

// Create a new rendezvous manager
func NewRendezvousManager(endpoints []RendezvousEndpoint, weaveUrl *url.URL) *RendezvousManager {
	r := &RendezvousManager{
		endpoints: endpoints,
		workers:   make(map[string]RendezvousWorker),
		notifyChan: make(chan string),
	}

	go func() {
		fullUrl := weaveUrl
		fullUrl.Path = "/connect"

		for ip := range r.notifyChan {
			Debug.Printf("Notifying %s about %s", fullUrl.String(), ip)
			_, err := http.PostForm(fullUrl.String(), url.Values{"peer": {ip}})
			if err != nil {
				Error.Printf("Could not notify about \"%s\": err", ip, err)
			}
		}
	}()

	return r
}

// Connect to a rendezvous domain (ie, "mdns:///somedomain")
func (r *RendezvousManager) Connect(domain string) error {
	Debug.Printf("Connecting to \"%s\"", domain)
	u, err := url.Parse(domain)
	if err != nil {
		Error.Printf("Could not parse rendezvous domain url \"%s\"", domain)
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
			r.workers[workerUrlStr] = worker
		}
	}

	return nil
}

// Leave a rendezvous service/domain (ie, "mdns:///somedomain")
func (r *RendezvousManager) Leave(domain string) error {
	// TODO
	return nil
}

// Notify the router about a new peer
func (r *RendezvousManager) notifyAbout(ip string) error {
	r.notifyChan <- ip
	return nil
}

func (r *RendezvousManager) Stop() error {
	Debug.Printf("Stoping all workers in rendezvous manager")
	for key, worker := range r.workers {
		worker.Stop()
		delete(r.workers, key)
	}
	close(r.notifyChan)
	return nil
}

// Return a simple status
func (r *RendezvousManager) Status() string {
	return fmt.Sprintf("%d workers", len(r.workers))
}
