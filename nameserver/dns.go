package nameserver

import (
	"github.com/miekg/dns"
	"net"
)

const (
	localTTL uint32 = 300 // somewhat arbitrary; we don't expect anyone
	// downstream to cache results
)

func makeHeader(r *dns.Msg, q *dns.Question) *dns.RR_Header {
	return &dns.RR_Header{
		Name: q.Name, Rrtype: q.Qtype,
		Class: dns.ClassINET, Ttl: localTTL}
}

func makeReply(r *dns.Msg, as []dns.RR) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Answer = as
	return m
}

func makeAddressReply(r *dns.Msg, q *dns.Question, addrs []net.IP) *dns.Msg {
	answers := make([]dns.RR, len(addrs))
	header := makeHeader(r, q)
	count := 0
	for _, addr := range addrs {
		switch q.Qtype {
		case dns.TypeA:
			if ip4 := addr.To4(); ip4 != nil {
				answers[count] = &dns.A{*header, addr}
				count++
			}
		case dns.TypeAAAA:
			if ip4 := addr.To4(); ip4 == nil {
				answers[count] = &dns.AAAA{*header, addr}
				count++
			}
		}
	}
	return makeReply(r, answers[:count])
}

func makePTRReply(r *dns.Msg, q *dns.Question, names []string) *dns.Msg {
	answers := make([]dns.RR, len(names))
	header := makeHeader(r, q)
	for i, name := range names {
		answers[i] = &dns.PTR{*header, name}
	}
	return makeReply(r, answers)
}

func makeDNSFailResponse(r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Rcode = dns.RcodeNameError
	return m
}
