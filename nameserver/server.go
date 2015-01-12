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
				if ip, err := lookup.LookupLocal(q.Name); err == nil {
					m := makeAddressReply(r, &q, []net.IP{ip})
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
				if name, err := lookup.ReverseLookupLocal(q.Name); err == nil {
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

/* When we receive a request for a name outside of our '.weave' domain, call
   the underlying lookup mechanism and return the answer(s) it gives.
   Unfortunately, this means that TTLs from a real DNS server are lost - FIXME.
*/
func notUsHandler() dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("[dns msgid %d] Non-local query: %+v", r.MsgHdr.Id, q)
		var responseMsg *dns.Msg
		if q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA {
			if addrs, err := net.LookupIP(q.Name); err == nil {
				responseMsg = makeAddressReply(r, &q, addrs)
			} else {
				responseMsg = makeDNSFailResponse(r)
				Debug.Printf("[dns msgid %d] Failed fallback: %s", r.MsgHdr.Id, err)
			}
		} else {
			Warning.Printf("[dns msgid %d] Non-local query not handled: %+v",
				r.MsgHdr.Id, q)
			responseMsg = makeDNSFailResponse(r)
		}
		w.WriteMsg(responseMsg)
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
