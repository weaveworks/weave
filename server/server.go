package weavedns

import (
	"fmt"
	"github.com/miekg/dns"
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
		Debug.Printf("Local query: %s", q)
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, dns.TypeA, []net.IP{ip})
			w.WriteMsg(m)
		} else {
			Debug.Printf("Failed lookup for %s; sending mDNS query", q.Name)
			// We don't know the answer; see if someone else does
			channel := make(chan *ResponseA, ChannelSize)
			replies := make([]net.IP, 0)
			go func() {
				// Loop terminates when channel is closed by MDNSClient on timeout
				for resp := range channel {
					Debug.Printf("Got address response %s to query %s addr %s", resp.Name, q.Name, resp.Addr)
					replies = append(replies, resp.Addr)
				}
				var response_msg *dns.Msg
				if len(replies) > 0 {
					response_msg = makeDNSReply(r, q.Name, dns.TypeA, replies)
				} else {
					response_msg = makeDNSFailResponse(r)
				}
				w.WriteMsg(response_msg)
			}()
			mdnsClient.SendQuery(q.Name, dns.TypeA, channel)
		}
		return
	}
}

/* When we receive a request for a name outside of our '.weave' domain, call
   the underlying lookup mechanism and return the answer(s) it gives.
   Unfortunately, this means that TTLs from a real DNS server are lost - FIXME.
*/
func notUsHandler() dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Non-local query: %s", q)
		addrs, err := net.LookupIP(q.Name)
		var response_msg *dns.Msg
		if err == nil {
			response_msg = makeDNSReply(r, q.Name, q.Qtype, addrs)
		} else {
			response_msg = makeDNSFailResponse(r)
			Debug.Print("Failed fallback lookup", err)
		}
		w.WriteMsg(response_msg)
	}
}

func StartServer(zone Zone, iface *net.Interface, dnsPort int, httpPort int, wait int) error {
	go ListenHttp(LOCAL_DOMAIN, zone, httpPort)

	mdnsClient, err := NewMDNSClient()
	checkFatal(err)

	Info.Printf("Using mDNS on %s", iface.Name)
	err = mdnsClient.Start(iface)
	checkFatal(err)

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc(LOCAL_DOMAIN, queryHandler(zone, mdnsClient))
	LocalServeMux.HandleFunc(".", notUsHandler())

	mdnsServer, err := NewMDNSServer(zone)
	checkFatal(err)

	err = mdnsServer.Start(iface)
	checkFatal(err)

	address := fmt.Sprintf(":%d", dnsPort)
	err = dns.ListenAndServe(address, "udp", LocalServeMux)
	checkFatal(err)

	Info.Printf("Listening for DNS on %s", address)
	return nil
}
