package weavedns

import (
	"github.com/miekg/dns"
	"net"
)

const (
	localTTL uint32 = 300 // somewhat arbitrary; we don't expect anyone
	// downstream to cache results
)

func makeDNSReply(r *dns.Msg, name string, qtype uint16, addrs []net.IP) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	hdr := dns.RR_Header{Name: name, Rrtype: qtype, Class: dns.ClassINET, Ttl: localTTL}
	for _, addr := range addrs {
		if qtype == dns.TypeA {
			if ip4 := addr.To4(); ip4 != nil {
				m.Answer = append(m.Answer, &dns.A{hdr, addr})
			}
		} else if qtype == dns.TypeAAAA {
			if ip4 := addr.To4(); ip4 == nil {
				m.Answer = append(m.Answer, &dns.AAAA{hdr, addr})
			}
		}
	}
	return m
}
