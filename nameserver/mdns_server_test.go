package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"testing"
	"time"
)

var (
	containerID = "deadbeef"
	testAddr1   = "10.0.2.1/24"
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
	dockerIP := net.ParseIP("9.8.7.6")
	weaveIP, _, _ := net.ParseCIDR(testAddr1)
	zone.AddRecord(containerID, "test.weave.", dockerIP, weaveIP)

	mdnsServer, err := NewMDNSServer(zone)
	assertNoErr(t, err)
	err = mdnsServer.Start(nil)
	assertNoErr(t, err)

	var receivedAddr net.IP
	receivedCount := 0

	// Implement a minimal listener for responses
	multicast, err := LinkLocalMulticastListener(nil)
	assertNoErr(t, err)

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		// Only handle responses here
		if len(r.Answer) > 0 {
			for _, answer := range r.Answer {
				switch rr := answer.(type) {
				case *dns.A:
					receivedAddr = rr.A
					receivedCount++
				}
			}
		}
	}

	server := &dns.Server{Listener: nil, PacketConn: multicast, Handler: dns.HandlerFunc(handleMDNS)}
	go server.ActivateAndServe()
	defer server.Shutdown()

	time.Sleep(100 * time.Millisecond) // Allow for server to get going

	sendQuery("test.weave.", dns.TypeA)

	time.Sleep(time.Second)

	if receivedCount != 1 {
		t.Fatal("Unexpected result count for test.weave", receivedCount)
	}
	if !receivedAddr.Equal(weaveIP) {
		t.Fatal("Unexpected result for test.weave", receivedAddr)
	}

	receivedCount = 0

	sendQuery("testfail.weave.", dns.TypeA)

	if receivedCount != 0 {
		t.Fatal("Unexpected result count for testfail.weave", receivedCount)
	}
}
