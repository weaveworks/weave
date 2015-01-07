package rendezvous

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"github.com/zettio/weave/nameserver"
	"math"
	"net"
	"net/url"
	"os"
	"path"
	"time"
)

const (
	mdnsQueryPeriod    = 5  // period for asking for new peers in a domain
	mdnsMaxQueryPeriod = 60 // the query period grows up to this
)

// Get the full URL we will use for announcing/searching in a domain (ie,
// for "mdns://some/path" it will be something like "mdns://eth0/path")
func MDnsWorkerUrl(u url.URL, iface *net.Interface) *url.URL {
	u.Host = iface.Name
	u.Path = path.Base(u.Path) // use only the last part of the path
	if u.Path == "." {
		u.Path = ""
	}
	return &u
}

type mDnsWorker struct {
	SimpleRendezvousService

	manager      *RendezvousManager
	fullDomain   string
	stopChan     chan bool
	announcedIps map[string]bool // keep track the IPs we finally announce
}

// Create a new mDNS rendezvous service for a domain
func NewMDnsWorker(manager *RendezvousManager, domainUrl *url.URL) *mDnsWorker {
	domain := path.Base(domainUrl.Path)
	fullDomain := "weave.local."
	if len(domain) > 0 {
		fullDomain = fmt.Sprintf("%s.weave.local.", domain)
	}

	mdns := mDnsWorker{
		SimpleRendezvousService: SimpleRendezvousService{
			Domain: domain,
		},
		manager:      manager,
		fullDomain:   fullDomain,
		stopChan:     make(chan bool),
		announcedIps: make(map[string]bool),
	}
	return &mdns
}

// start a mDNS worker for this domain/interface
func (mdns *mDnsWorker) Start(endpoints []RendezvousEndpoint, iface *net.Interface) error {
	name, err := os.Hostname()
	zone := new(nameserver.ZoneDb)
	for _, ep := range endpoints {
		Debug.Printf("Announcing %s at %s on %s", mdns.fullDomain, ep.ip, ep.iface.Name)
		zone.AddRecord(name, mdns.fullDomain, ep.ip)
		mdns.announcedIps[ep.ip.String()] = true
	}

	Debug.Printf("Starting mDNS server on %s...", iface.Name)
	mdnsServer, err := nameserver.NewMDNSServer(zone)
	err = mdnsServer.Start(iface)
	if err != nil {
		return fmt.Errorf("Error when starting mDNS service: %s", err)
	}

	// start the client
	Debug.Printf("Starting mDNS client on %s...", iface.Name)
	mdnsClient, err := nameserver.NewMDNSClient()
	if err != nil {
		return fmt.Errorf("Error when starting mDNS client: %s", err)
	}

	err = mdnsClient.Start(iface)
	if err != nil {
		return fmt.Errorf("Error when starting mDNS client: %s", err)
	}

	// query periodically (up to mdnsQueryPeriod seconds) for this name
	go func() {
		queryPeriod := mdnsQueryPeriod
		timer := time.NewTimer(0)
		responsesChan := make(chan *nameserver.ResponseA)
		minInt := func(x int, y int) int { return int(math.Min(float64(x), float64(y))) }

	outerloop:
		for {
			select {
			case <-mdns.stopChan:
				Debug.Printf("Stop request in mDNS worker")
				mdnsClient.Shutdown()
				break outerloop
			case <-timer.C:
				mdnsClient.BackgroundQuery(mdns.fullDomain, dns.TypeA, responsesChan)
				// increase the period, up to mdnsMaxQueryPeriod
				queryPeriod = minInt(queryPeriod*2, mdnsMaxQueryPeriod)
				Debug.Printf("Querying every %d seconds...", queryPeriod)
				timer.Reset(time.Duration(queryPeriod) * time.Second)
			case resp, ok := <-responsesChan:
				if ok {
					foundIpStr := resp.Addr.String()
					if _, ourselves := mdns.announcedIps[foundIpStr]; !ourselves {
						Debug.Printf("Found peer \"%s\" with mDNS", foundIpStr)
						mdns.manager.notifyAbout(foundIpStr)
					}
				}
			}
		}
	}()

	Debug.Printf("Looking for mDNS peers in the background...")
	return nil
}

// Stop the mDNS worker
func (mdns *mDnsWorker) Stop() error {
	mdns.stopChan <- true
	return nil
}
