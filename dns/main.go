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

func handleLocal(w dns.ResponseWriter, r *dns.Msg) {
	q := r.Question[0]
	ip, err := zone.MatchLocal(q.Name)
	if err == nil {
		m := makeDNSReply(r, q.Name, net.ParseIP(ip))
		w.WriteMsg(m)
	} else {
		log.Printf("Failed lookup for %s", q.Name)
	}
	return
}

func main() {
	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc("local", handleLocal)
	go weavedns.ListenHttp(zone)
	dns.ListenAndServe(":5300", "udp", LocalServeMux)
}
