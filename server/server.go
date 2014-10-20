package weavedns

import (
	"fmt"
	"github.com/miekg/dns"
	"log"
	"net"
)

const (
	LOCAL_DOMAIN = "weave.local."
)

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
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, dns.TypeA, []net.IP{ip})
			w.WriteMsg(m)
		} else {
			log.Printf("Failed lookup for %s; sending mDNS query", q.Name)
			// We don't know the answer; see if someone else does
			channel := make(chan *ResponseA, ChannelSize)
			replies := make([]net.IP, 0)
			go func() {
				// Loop terminates when channel is closed by MDNSClient on timeout
				for resp := range channel {
					log.Printf("Got address response %s to query %s addr %s", resp.Name, q.Name, resp.Addr)
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
		addrs, err := net.LookupIP(q.Name)
		var response_msg *dns.Msg
		if err == nil {
			response_msg = makeDNSReply(r, q.Name, q.Qtype, addrs)
		} else {
			response_msg = makeDNSFailResponse(r)
		}
		w.WriteMsg(response_msg)
	}
}

func StartServer(zone Zone, iface *net.Interface, dnsPort int, httpPort int, wait int) error {
	go ListenHttp(LOCAL_DOMAIN, zone, httpPort)

	mdnsClient, err := NewMDNSClient()
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Using mDNS on %s", iface.Name)
	}
	err = mdnsClient.Start(iface)
	if err != nil {
		log.Fatal(err)
	}

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc(LOCAL_DOMAIN, queryHandler(zone, mdnsClient))
	LocalServeMux.HandleFunc(".", notUsHandler())

	mdnsServer, err := NewMDNSServer(zone)
	if err != nil {
		log.Fatal(err)
	}
	err = mdnsServer.Start(iface)
	if err != nil {
		log.Fatal(err)
	}

	address := fmt.Sprintf(":%d", dnsPort)
	err = dns.ListenAndServe(address, "udp", LocalServeMux)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Listening for DNS on %s", address)
	}
	return nil
}
