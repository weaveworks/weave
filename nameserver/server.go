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
			Debug.Printf("Query failed for %s", q.Name)
			w.WriteMsg(makeDNSFailResponse(r))
		} else {
			Warning.Printf("Query not handled: %+v", q)
			w.WriteMsg(makeDNSFailResponse(r))
		}
		return
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
			Debug.Printf("Reverse query failed for %s", q.Name)
			w.WriteMsg(makeDNSFailResponse(r))
		} else {
			Warning.Printf("Reverse query not handled: %+v", q)
			w.WriteMsg(makeDNSFailResponse(r))
		}
	}
}

/* When we receive a request for a name outside of our '.weave' domain, call
   the underlying lookup mechanism and return the answer(s) it gives.
   Unfortunately, this means that TTLs from a real DNS server are lost - FIXME.
*/
func notUsHandler() dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Non-local query: %+v", q)
		var responseMsg *dns.Msg
		if q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA {
			if addrs, err := net.LookupIP(q.Name); err == nil {
				responseMsg = makeAddressReply(r, &q, addrs)
			} else {
				responseMsg = makeDNSFailResponse(r)
				Debug.Print("Failed fallback lookup ", err)
			}
		} else {
			Warning.Printf("Non-local query not handled: %+v", q)
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
