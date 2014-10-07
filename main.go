package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"github.com/zettio/weavedns/server"
	"log"
	"net"
	"time"
)

var zone = new(weavedns.ZoneDb)

func makeDNSReply(r *dns.Msg, name string, addr net.IP) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	hdr := dns.RR_Header{Name: name, Rrtype: dns.TypeA,
		Class: dns.ClassINET, Ttl: 3600}
	a := &dns.A{hdr, addr}
	m.Answer = append(m.Answer, a)
	return m
}

var mdnsClient *weavedns.MDNSClient

func handleMDNS(w dns.ResponseWriter, r *dns.Msg) {
	// Here we have to handle both requests incoming, responses to queries
	// we sent out, and responses to queries that other nodes sent
	if len(r.Answer) > 0 {
		mdnsClient.ResponseCallback(r)
	} else if len(r.Question) > 0 {
		q := r.Question[0]
		ip, err := zone.MatchLocal(q.Name)
		if err == nil {
			m := makeDNSReply(r, q.Name, net.ParseIP(ip))
			mdnsClient.SendResponse(m)
		} else if mdnsClient.IsInflightQuery(r) {
			// ignore this - it's our own query received via multicast
		} else {
			log.Printf("Failed MDNS lookup for %s", q.Name)
		}
	}
}

func handleLocal(w dns.ResponseWriter, r *dns.Msg) {
	q := r.Question[0]
	ip, err := zone.MatchLocal(q.Name)
	if err == nil {
		m := makeDNSReply(r, q.Name, net.ParseIP(ip))
		w.WriteMsg(m)
	} else {
		log.Printf("Failed lookup for %s; sending mDNS query", q.Name)
		// We don't know the answer; see if someone else does
		channel := make(chan *weavedns.ResponseA, 4)
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

func main() {
	var (
		ifaceName string
		dnsPort   int
		wait      int
	)

	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (defaults to 0)")
	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests (defaults to 53)")
	flag.Parse()

	var iface *net.Interface = nil
	if ifaceName != "" {
		var err error
		iface, err = ensureInterface(ifaceName, wait)
		if err != nil {
			log.Fatal(err)
		}
	}

	LocalServeMux := dns.NewServeMux()
	LocalServeMux.HandleFunc("local", handleLocal)
	go weavedns.ListenHttp(zone)

	MDNSServeMux := dns.NewServeMux()
	MDNSServeMux.HandleFunc("local", handleMDNS)

	conn, err := weavedns.LinkLocalMulticastListener(iface)
	if err != nil {
		log.Fatal(err)
	}
	mdnsClient, err = weavedns.NewMDNSClient()
	if err != nil {
		log.Fatal(err)
	}
	mdnsClient.Start()

	go dns.ActivateAndServe(nil, conn, MDNSServeMux)

	address := fmt.Sprintf(":%d", dnsPort)
	err = dns.ListenAndServe(address, "udp", LocalServeMux)
	if err != nil {
		log.Fatal(err)
	}
}
