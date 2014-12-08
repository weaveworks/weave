package router

import (
	"github.com/miekg/dns"
	"github.com/zettio/weave/nameserver"
	weavenet "github.com/zettio/weave/net"
	"net"
	"time"
	"fmt"
	"log"
	"os"
)

// period for asking for new peers in a domain
const MDNS_QUERY_PERIOD = 5

type mDnsRendezvous struct {
	SimpleRendezvousService

	cm         *ConnectionMaker
	fullDomain string
	stopChan   chan bool
}

// Create a new mDNS rendezvous service for a domain
func NewMDnsRendezvous(cm *ConnectionMaker, domain string) *mDnsRendezvous {
	fullDomain := "weave.local."
	if len(domain) > 0 {
		fullDomain = fmt.Sprintf("%s.weave.local.", domain)
	}

	mdns := mDnsRendezvous{
		SimpleRendezvousService: SimpleRendezvousService{Domain: domain},
		cm:         cm,
		fullDomain: fullDomain,
		stopChan:   make(chan bool),
	}
	return &mdns
}

// start the mDNS rendezvous service for this domain
func (mdns *mDnsRendezvous) Start(announcedIps weavenet.ExternalIps) error {
	if len(announcedIps) == 0 {
		// ok, no external IPs have been provided, so this is a hackish solution for
		// discovering our IP addresses: we use them all! we should implement something
		// like ICE at the protocol level, so we could negotiate the best IP at the handshake...
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			log.Fatalf("Couod not obtain local IP addresses: %s", err)
		}

		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.IsGlobalUnicast() {
				if ipnet.IP.To4() != nil {
					announcedIps = append(announcedIps, ipnet.IP)
				}
			}
		}
	}

	name, err := os.Hostname()
	zone := new(nameserver.ZoneDb)
	for _, a := range announcedIps {
		log.Printf("Announcing %s at %s", mdns.fullDomain, a)
		zone.AddRecord(name, mdns.fullDomain, a)
	}

	mdnsServer, err := nameserver.NewMDNSServer(zone)
	err = mdnsServer.Start(mdns.cm.router.Iface)

	// start the client
	log.Printf("Starting mDNS client...")
	mdnsClient, err := nameserver.NewMDNSClient()
	defer mdnsClient.Shutdown()
	if err != nil {
		log.Printf("Error when starting mDNS service: %s", err)
		// TODO
	}

	err = mdnsClient.Start(mdns.cm.router.Iface)
	if err != nil {
		log.Printf("Error when starting mDNS client: %s", err)
		// TODO
	}

	// query periodically (every MDNS_QUERY_PERIOD seconds) for this name
	go func() {
		responsesChan := make(chan *nameserver.ResponseA, 4)
		timer := time.NewTimer(0)

		outerloop:
		for responsesChan != nil || responsesChan != nil {
			select {
			case <-mdns.stopChan:
				break outerloop
			case <-timer.C:
				mdnsClient.SendQuery(mdns.fullDomain, dns.TypeA, responsesChan)
				timer.Reset(MDNS_QUERY_PERIOD * time.Second)
			case resp := <-responsesChan:
				log.Printf("Found new peer %s with mDNS", resp.Addr.String())
				mdns.cm.InitiateConnection(resp.Addr.String())
			}
		}
	}()

	log.Printf("Looking for mDNS peers in the background...")
	return nil
}

func (mdns *mDnsRendezvous) Stop() error {
	mdns.stopChan <- true
	return nil
}
