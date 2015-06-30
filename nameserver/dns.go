package nameserver

import (
	"math/rand"
	"time"

	"github.com/miekg/dns"
)

const (
	minUDPSize = 512
	maxUDPSize = 65535
)

func makeHeader(r *dns.Msg, q *dns.Question, ttl int) *dns.RR_Header {
	return &dns.RR_Header{
		Name:   q.Name,
		Rrtype: q.Qtype,
		Class:  dns.ClassINET,
		Ttl:    uint32(ttl),
	}
}

func makeReply(r *dns.Msg, as []dns.RR) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Answer = as
	return m
}

func makeTruncatedReply(r *dns.Msg) *dns.Msg {
	// for truncated response, we create a minimal reply with the Truncated bit set
	reply := new(dns.Msg)
	reply.SetReply(r)
	reply.Truncated = true
	return reply
}

type DNSResponseBuilder func(r *dns.Msg, q *dns.Question, addrs []ZoneRecord, ttl int) *dns.Msg

func makeAddressReply(r *dns.Msg, q *dns.Question, addrs []ZoneRecord, ttl int) *dns.Msg {
	answers := make([]dns.RR, len(addrs))
	header := makeHeader(r, q, ttl)
	count := 0
	for _, addr := range addrs {
		ip := addr.IP()
		ip4 := ip.To4()

		switch q.Qtype {
		case dns.TypeA:
			if ip4 != nil {
				answers[count] = &dns.A{Hdr: *header, A: ip}
				count++
			}
		case dns.TypeAAAA:
			if ip4 == nil {
				answers[count] = &dns.AAAA{Hdr: *header, AAAA: ip}
				count++
			}
		}
	}
	return makeReply(r, answers[:count])
}

func makePTRReply(r *dns.Msg, q *dns.Question, names []ZoneRecord, ttl int) *dns.Msg {
	answers := make([]dns.RR, len(names))
	header := makeHeader(r, q, ttl)
	for i, name := range names {
		answers[i] = &dns.PTR{Hdr: *header, Ptr: name.Name()}
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

func failHandleFunc(w dns.ResponseWriter, r *dns.Msg) {
	w.WriteMsg(makeDNSFailResponse(r))
}

// get the maximum UDP-reply length
func getMaxReplyLen(r *dns.Msg, proto dnsProtocol) int {
	maxLen := minUDPSize
	if proto == protTCP {
		maxLen = maxUDPSize
	} else if opt := r.IsEdns0(); opt != nil {
		maxLen = int(opt.UDPSize())
	}
	return maxLen
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

// shuffleAnswers reorders answers for very basic load balancing
func shuffleAnswers(answers []dns.RR) []dns.RR {
	if len(answers) > 1 {
		for i := range answers {
			j := rand.Intn(i + 1)
			answers[i], answers[j] = answers[j], answers[i]
		}
	}

	return answers
}

// only take the first `num` answers
func pruneAnswers(answers []dns.RR, num int) []dns.RR {
	if num > 0 && len(answers) > num {
		// TODO: we should have some prefer locally-introduced answers, etc...
		return answers[:num]
	}
	return answers
}
