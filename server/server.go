package weavedns

import (
	"fmt"
	"github.com/miekg/dns"
	"log"
	"net"
	"time"
)

const (
	LOCAL_DOMAIN = "weave"
)

func ensureInterface(ifaceName string, wait int) (iface *net.Interface, err error) {
	iface, err = findInterface(ifaceName)
	if err == nil || wait == 0 {
		return
	}
	log.Println("Waiting for interface", ifaceName, "to come up")
	for ; err != nil && wait > 0; wait -= 1 {
		time.Sleep(1 * time.Second)
		iface, err = findInterface(ifaceName)
	}
	if err == nil {
		log.Println("Interface", ifaceName, "is up")
	}
	return
}

func findInterface(ifaceName string) (iface *net.Interface, err error) {
	iface, err = net.InterfaceByName(ifaceName)
	if err != nil {
		return iface, fmt.Errorf("Unable to find interface %s", ifaceName)
	}
	if 0 == (net.FlagUp & iface.Flags) {
		return iface, fmt.Errorf("Interface %s is not up", ifaceName)
	}
	return
}

func queryHandler(zone Zone, mdnsClient *MDNSClient) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, ip)
			w.WriteMsg(m)
		} else {
			log.Printf("Failed lookup for %s; sending mDNS query", q.Name)
			// We don't know the answer; see if someone else does
			channel := make(chan *ResponseA, 4)
			go func() {
				// Loop terminates when channel is closed by MDNSClient on timeout
				for resp := range channel {
					log.Printf("Got address response %s to query %s addr %s", resp.Name, q.Name, resp.Addr)
					m := makeDNSReply(r, resp.Name, resp.Addr)
					w.WriteMsg(m)
				}
			}()
			mdnsClient.SendQuery(q.Name, dns.TypeA, channel)
		}
		return
	}
}

func StartServer(ifaceName string, dnsPort int, httpPort int, wait int) error {

	var zone = new(ZoneDb)
	go ListenHttp(zone)

	var iface *net.Interface = nil
	if ifaceName != "" {
		var err error
		iface, err = ensureInterface(ifaceName, wait)
		if err != nil {
			log.Fatal(err)
		}
	}

	mdnsClient, err := NewMDNSClient()
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Using mDNS on %s", ifaceName)
	}
	err = mdnsClient.Start(iface)
	if err != nil {
		log.Fatal(err)
	}

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc(LOCAL_DOMAIN, queryHandler(zone, mdnsClient))

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
