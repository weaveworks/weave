package main

import (
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
	// And we also get to see our own queries!
	if len(r.Answer) > 0 {
		mdnsClient.HandleResponse(r)
	} else if len(r.Question) > 0 {
		q := r.Question[0]
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, net.ParseIP(ip))
			mdnsClient.SendResponse(m)
		}
	}
}

func handleLocal(w dns.ResponseWriter, r *dns.Msg) {
	log.Printf("Received message:", r)
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

	dns.ListenAndServe(":53", "udp", LocalServeMux)
}
