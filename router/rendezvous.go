package router

import (
	"github.com/miekg/dns"
	"github.com/zettio/weave/nameserver"
	"net"
	"time"
)

const MDNS_QUERY_PERIOD = 5

type mDnsRendezvous struct {
	SimpleRendezvousService

	cm         *ConnectionMaker
	fullDomain string
	stopChan   chan bool
}

// Create a new mDNS rendezvous service
func NewMDnsRendezvous(cm *ConnectionMaker, domain string) *mDnsRendezvous {
	mdns := mDnsRendezvous{
		SimpleRendezvousService: SimpleRendezvousService{Domain: domain},
		cm:         cm,
		fullDomain: domain, // TODO: something like "weave.local."
		stopChan:   make(chan bool),
	}
	return &mdns
}

// start the mDNS rendezvous service for a domain
func (mdns *mDnsRendezvous) Start() error {
	// create the entry we announce
	// TODO
	addresses, err := mdns.cm.router.Iface.Addrs()
	if err != nil {
		// TODO
	}

	var zone = new(nameserver.ZoneDb)
	zone.AddRecord("ident", mdns.fullDomain, net.ParseIP(addresses[0].String()))

	mdnsServer, err := nameserver.NewMDNSServer(zone)
	err = mdnsServer.Start(nil)

	// start the client
	mdnsClient, err := nameserver.NewMDNSClient()
	defer mdnsClient.Shutdown()
	if err != nil {
		// TODO
	}

	err = mdnsClient.Start(nil)
	if err != nil {
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
				mdns.cm.InitiateConnection(resp.Addr.String())
			}
		}
	}()


	return nil
}

func (mdns *mDnsRendezvous) Stop() error {
	mdns.stopChan <- true
	return nil
}
