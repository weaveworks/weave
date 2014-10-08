package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"testing"
	"time"
)

var (
	test_addr1 = "10.0.2.1/24"
)

func sendQuery(name string, querytype uint16) error {
	m := new(dns.Msg)
	m.SetQuestion(name, querytype)
	m.RecursionDesired = false
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(buf, ipv4Addr)
	return err
}

func TestServerSimpleQuery(t *testing.T) {
	log.Println("TestServerSimpleQuery starting")
	var zone = new(ZoneDb)
	docker_ip := net.ParseIP("9.8.7.6")
	weave_ip, subnet, _ := net.ParseCIDR(test_addr1)
	zone.AddRecord("test.weave.", docker_ip, weave_ip, subnet)

	mdnsServer, err := NewMDNSServer(zone)
	if err != nil {
		t.Fatal(err)
	}
	err = mdnsServer.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	var received_addr net.IP

	// Implement a minimal listener for responses
	multicast, err := LinkLocalMulticastListener(nil)
	if err != nil {
		t.Fatal(err)
	}

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		// Only handle responses here
		if len(r.Answer) > 0 {
			for _, answer := range r.Answer {
				switch rr := answer.(type) {
				case *dns.A:
					received_addr = rr.A
				}
			}
		}
	}

	server := &dns.Server{Listener: nil, PacketConn: multicast, Handler: dns.HandlerFunc(handleMDNS)}
	go server.ActivateAndServe()

	sendQuery("test.weave.", dns.TypeA)

	time.Sleep(time.Second)

	if !received_addr.Equal(weave_ip) {
		t.Log("Unexpected result for test.weave", received_addr)
		t.Fail()
	}
}
