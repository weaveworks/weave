package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"github.com/zettio/weavedns"
	"log"
	"net"
)

var zone = new(weavedns.ZoneDb)

func makeDNSReply(r *dns.Msg, name string, addr net.IP) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	hdr := dns.RR_Header{Name: name, Rrtype: dns.TypeA,
		Class: dns.ClassINET, Ttl: 3600}
	a := &dns.A{hdr, addr}
	m.Answer = append(m.Answer, a)
	return m
}

var mdnsClient *weavedns.MDNSClient

func handleMDNS(w dns.ResponseWriter, r *dns.Msg) {
	// Here we have to handle both requests incoming, responses to queries
	// we sent out, and responses to queries that other nodes sent
	if len(r.Answer) > 0 {
		mdnsClient.HandleResponse(r)
	} else if len(r.Question) > 0 {
		q := r.Question[0]
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, net.ParseIP(ip))
			mdnsClient.SendResponse(m)
		} else if mdnsClient.IsInflightQuery(r) {
			// ignore this - it's our own query received via multicast
		} else {
			log.Printf("Failed MDNS lookup for %s", q.Name)
		}
	}
}

func handleLocal(w dns.ResponseWriter, r *dns.Msg) {
	log.Printf("Received message: %s", r)
	q := r.Question[0]
	ip, err := zone.MatchLocal(q.Name)
	if err == nil {
		m := makeDNSReply(r, q.Name, net.ParseIP(ip))
		w.WriteMsg(m)
	} else {
		log.Printf("Failed lookup for %s", q.Name)
		// We don't know the answer; see if someone else does
		channel := make(chan *weavedns.ResponseA, 4)
		go func() {
			for resp := range channel {
				log.Printf("Got address response %s to query %s addr %s", resp.Name, q.Name, resp.Addr)
				m := makeDNSReply(r, resp.Name, resp.Addr)
				w.WriteMsg(m)
			}
			// FIXME: need to add a time-out either here or in mdnsClient
		}()
		mdnsClient.SendQuery(q.Name, dns.TypeA, channel)
	}
	return
}

func main() {
	var (
		dnsPort int
	)

	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests (defaults to 53)")
	flag.Parse()

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc("local", handleLocal)
	go weavedns.ListenHttp(zone)

	MDNSServeMux := dns.NewServeMux()
	MDNSServeMux.HandleFunc("local", handleMDNS)

	conn, err := weavedns.LinkLocalMulticastListener()
	if err != nil {
		log.Fatal(err)
	}
	mdnsClient, err = weavedns.NewMDNSClient()
	if err != nil {
		log.Fatal(err)
	}

	go dns.ActivateAndServe(nil, conn, MDNSServeMux)

	address := fmt.Sprintf(":%d", dnsPort)
	err = dns.ListenAndServe(address, "udp", LocalServeMux)
	if err != nil {
		log.Fatal(err)
	}
}
