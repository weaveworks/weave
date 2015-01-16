package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
)

const (
	LOCAL_DOMAIN = "weave.local."
)

func checkFatal(e error) {
	if e != nil {
		Error.Fatal(e)
	}
}

func checkWarn(e error) {
	if e != nil {
		Warning.Println(e)
	}
}

func makeDNSFailResponse(r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Rcode = dns.RcodeNameError
	return m
}

func queryHandler(lookups []Lookup) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Query: %+v", q)
		if q.Qtype == dns.TypeA {
			for _, lookup := range lookups {
				if ip, err := lookup.LookupName(q.Name); err == nil {
					m := makeAddressReply(r, &q, ip)
					w.WriteMsg(m)
					return
				}
			}
		}
		Info.Printf("[dns msgid %d] No results for type %s query %s",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}

func rdnsHandler(lookups []Lookup) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Reverse query: %+v", q)
		if q.Qtype == dns.TypePTR {
			for _, lookup := range lookups {
				if name, err := lookup.LookupInaddr(q.Name); err == nil {
					m := makePTRReply(r, &q, []string{name})
					w.WriteMsg(m)
					return
				}
			}
			Info.Printf("[dns msgid %d] No results for type %s query %s",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
		} else {
			Warning.Printf("[dns msgid %d] Unexpected reverse query type %s: %+v",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q)
		}
		w.WriteMsg(makeDNSFailResponse(r))
	}
}

/* When we receive a request for a name outside of our '.weave.local.'
   domain, ask the configured DNS server as a fallback.
*/
func notUsHandler() dns.HandlerFunc {
	config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	checkFatal(err)
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("[dns msgid %d] Non-local query: %+v", r.MsgHdr.Id, q)
		for _, server := range config.Servers {
			reply, err := dns.Exchange(r, fmt.Sprintf("%s:%s", server, config.Port))
			if err != nil {
				Debug.Printf("[dns msgid %d] Network error trying %s (%s)",
					r.MsgHdr.Id, server, err)
				continue
			}
			if reply != nil && reply.Rcode != dns.RcodeSuccess {
				Debug.Printf("[dns msgid %d] Failure reported by %s for query %s",
					r.MsgHdr.Id, server, q.Name)
				continue
			}
			Debug.Printf("[dns msgid %d] Given answer by %s for query %s",
				r.MsgHdr.Id, server, q.Name)
			w.WriteMsg(reply)
			return
		}
		Warning.Printf("[dns msgid %d] Failed lookup for external name %s",
			r.MsgHdr.Id, q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}

func StartServer(zone Zone, iface *net.Interface, dnsPort int, wait int) error {
	mdnsClient, err := NewMDNSClient()
	checkFatal(err)

	ifaceName := "default interface"
	if iface != nil {
		ifaceName = iface.Name
	}
	Info.Printf("Using mDNS on %s", ifaceName)
	err = mdnsClient.Start(iface)
	checkFatal(err)

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc(LOCAL_DOMAIN, queryHandler([]Lookup{zone, mdnsClient}))
	LocalServeMux.HandleFunc(RDNS_DOMAIN, rdnsHandler([]Lookup{zone, mdnsClient}))
	LocalServeMux.HandleFunc(".", notUsHandler())

	mdnsServer, err := NewMDNSServer(zone)
	checkFatal(err)

	err = mdnsServer.Start(iface)
	checkFatal(err)

	address := fmt.Sprintf(":%d", dnsPort)
	Info.Printf("Listening for DNS on %s", address)
	err = dns.ListenAndServe(address, "udp", LocalServeMux)
	checkFatal(err)

	return nil
}
