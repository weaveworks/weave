package router

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"github.com/zettio/weave/nameserver"
	weavenet "github.com/zettio/weave/net"
	"log"
	"net"
	"os"
	"time"
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
		SimpleRendezvousService: SimpleRendezvousService{
			Domain:       domain,
			announcedIps: weavenet.NewExternalIps(),
		},
		cm:         cm,
		fullDomain: fullDomain,
		stopChan:   make(chan bool),
	}
	return &mdns
}

// start the mDNS rendezvous service for this domain
func (mdns *mDnsRendezvous) Start(announcedIps weavenet.ExternalIps) error {
	if len(announcedIps) == 0 {
		// we cannot go further with no external IPs
		return errors.New("No external IP addresses provided")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("Could not get interfaces list: %s", err)
	}
	log.Printf("Interfaces:")
	for _, i := range ifaces {
		log.Printf("    * %v", i)
		addrs, _ := i.Addrs()
		for _, ip := range addrs {
			log.Printf("      * %v", ip)
		}
	}

	name, err := os.Hostname()
	miface, _ := net.InterfaceByName("eth0")
	zone := new(nameserver.ZoneDb)
	for announcedIp := range announcedIps {
		log.Printf("Announcing %s at %s on %s", mdns.fullDomain, announcedIp, miface.Name)
		zone.AddRecord(name, mdns.fullDomain, net.ParseIP(announcedIp))
		mdns.announcedIps[announcedIp] = true
	}

	mdnsServer, err := nameserver.NewMDNSServer(zone)
	err = mdnsServer.Start(mdns.cm.router.Iface)
	if err != nil {
		return fmt.Errorf("Error when starting mDNS service: %s", err)
	}

	// start the client
	log.Printf("Starting mDNS client...")
	mdnsClient, err := nameserver.NewMDNSClient()
	if err != nil {
		return fmt.Errorf("Error when starting mDNS client: %s", err)
	}

	err = mdnsClient.Start(miface) //mdns.cm.router.Iface)
	if err != nil {
		return fmt.Errorf("Error when starting mDNS client: %s", err)
	}

	// query periodically (every MDNS_QUERY_PERIOD seconds) for this name
	go func() {
		timer := time.NewTimer(0)
		responsesChan := make(chan *nameserver.ResponseA)

	outerloop:
		for {
			select {
			case <-mdns.stopChan:
				log.Printf("Shutting down rendezvous")
				mdnsClient.Shutdown()
				break outerloop
			case <-timer.C:
				mdnsClient.BackgroundQuery(mdns.fullDomain, dns.TypeA, responsesChan)
				timer.Reset(MDNS_QUERY_PERIOD * time.Second)
			case resp, ok := <-responsesChan:
				if ok {
					foundIpStr := resp.Addr.String()
					if _, ourselves := mdns.announcedIps[foundIpStr]; ourselves {
						log.Printf("Found ourselves (%s) with mDNS", foundIpStr)
					} else {
						log.Printf("Found new peer %s with mDNS", foundIpStr)
						mdns.cm.InitiateConnection(foundIpStr)
					}
				}
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
