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
	"strings"
	"time"
)

const (
	mdnsQueryPeriod    = 10  // initial period for asking for new peers in a domain
	mdnsMaxQueryPeriod = 120 // max backoff the query period

	domainStrippedChars = "./+-=_"
)

// remove some forbidden chars fom a string
func stripchars(str, chr string) string {
	return strings.Map(func(r rune) rune {
		if strings.IndexRune(chr, r) < 0 {
			return r
		}
		return -1
	}, str)
}

func domainFromPath(p string) string {
	return stripchars(path.Base(p), domainStrippedChars) // use only the last part of the path
}

// Get the full URL we will use for announcing/searching in a domain (ie,
// for "mdns://some/path" it will be something like "mdns://eth0/path")
func MDnsWorkerUrl(u url.URL, iface *net.Interface) *url.URL {
	u.Host = iface.Name
	u.Path = domainFromPath(u.Path)
	return &u
}

type mDnsWorker struct {
	SimpleRendezvousWorker

	manager    *RendezvousManager
	fullDomain string
	stopChan   chan bool
	skippedIps map[string]bool // keep track the IPs we have notified about or our own IPs
}

// Create a new mDNS rendezvous service for a domain
func NewMDnsWorker(manager *RendezvousManager, domainUrl *url.URL) *mDnsWorker {
	domain := domainFromPath(domainUrl.Path)
	fullDomain := "weave.local."
	if len(domain) > 0 {
		fullDomain = fmt.Sprintf("%s.weave.local.", domain)
	}

	mdns := mDnsWorker{
		SimpleRendezvousWorker: SimpleRendezvousWorker{
			Domain: domain,
		},
		manager:    manager,
		fullDomain: fullDomain,
		stopChan:   make(chan bool),
		skippedIps: make(map[string]bool),
	}
	return &mdns
}

// start a mDNS worker for this domain/interface
func (mdns *mDnsWorker) Start(externals []externalIface, iface *net.Interface) error {
	name, err := os.Hostname()
	zone := new(nameserver.ZoneDb)
	for _, external := range externals {
		Debug.Printf("Announcing %s at %s on %s", mdns.fullDomain, external.ip, external.iface.Name)
		zone.AddRecord(name, mdns.fullDomain, external.ip)
		mdns.skippedIps[external.ip.String()] = true
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
		responsesChan := make(chan *nameserver.Response)
		minInt := func(x int, y int) int { return int(math.Min(float64(x), float64(y))) }

	outerloop:
		for {
			select {
			case <-mdns.stopChan:
				Debug.Printf("Received stop request in mDNS worker")
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
					if _, skipped := mdns.skippedIps[foundIpStr]; !skipped {
						Debug.Printf("Found peer \"%s\" with mDNS", foundIpStr)
						mdns.manager.notifyAbout(foundIpStr)
						mdns.skippedIps[foundIpStr] = true //TODO: maybe we should move skippedIps to the manager...
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
	Debug.Printf("Stoping mDNS worker on %s", mdns.fullDomain)
	mdns.stopChan <- true
	return nil
}
