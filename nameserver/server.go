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

func queryHandler(zone Zone, mdnsClient *MDNSClient) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Local query: %+v", q)
		if q.Qtype == dns.TypeA {
			if ip, err := zone.LookupLocal(q.Name); err == nil {
				m := makeAddressReply(r, &q, []net.IP{ip})
				w.WriteMsg(m)
			} else {
				Debug.Printf("Failed lookup for %s; sending mDNS query", q.Name)
				// We don't know the answer; see if someone else does
				channel := make(chan *ResponseA)
				replies := make([]net.IP, 0)
				go func() {
					for resp := range channel {
						Debug.Printf("Got address response %s to query %s addr %s", resp.Name, q.Name, resp.Addr)
						replies = append(replies, resp.Addr)
					}
					var responseMsg *dns.Msg
					if len(replies) > 0 {
						responseMsg = makeAddressReply(r, &q, replies)
					} else {
						responseMsg = makeDNSFailResponse(r)
					}
					w.WriteMsg(responseMsg)
				}()
				mdnsClient.SendQuery(q.Name, dns.TypeA, channel)
			}
		} else {
			Warning.Printf("Local query not handled: %+v", q)
			m := makeDNSFailResponse(r)
			w.WriteMsg(m)
		}
		return
	}
}

func rdnsHandler(zone Zone, mdnsClient *MDNSClient) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Local rdns query: %+v", q)
		if q.Qtype == dns.TypePTR {
			if name, err := zone.ReverseLookupLocal(q.Name); err == nil {
				Debug.Printf("Found name: %s", name)
				m := makePTRReply(r, &q, []string{name})
				w.WriteMsg(m)
			} else {
				Debug.Printf("Failed lookup for %s; sending mDNS query", q.Name)
				// We don't know the answer; see if someone else does
				// TODO
			}
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

func StartServer(zone Zone, iface *net.Interface, dnsPort int, httpPort int, wait int) error {
	go ListenHttp(LOCAL_DOMAIN, zone, httpPort)

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
	LocalServeMux.HandleFunc(LOCAL_DOMAIN, queryHandler(zone, mdnsClient))
	LocalServeMux.HandleFunc(RDNS_DOMAIN, rdnsHandler(zone, mdnsClient))
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
