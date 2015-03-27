package nameserver

import (
	"github.com/miekg/dns"
	"github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"log"
	"net"
	"testing"
	"time"
)

var (
	containerID = "deadbeef"
	testName    = "test.weave.local."
	testAddr1   = "10.2.2.1/24"
	testInAddr1 = "1.2.2.10.in-addr.arpa."
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
	// The ff can be handy for debugging (obvs)
	common.InitDefaultLogging(true)

	log.Println("TestServerSimpleQuery starting")
	var zone = new(ZoneDb)
	ip, _, _ := net.ParseCIDR(testAddr1)
	zone.AddRecord(containerID, testName, ip)

	mdnsServer, err := NewMDNSServer(zone)
	wt.AssertNoErr(t, err)
	err = mdnsServer.Start(nil, DefaultLocalDomain)
	wt.AssertNoErr(t, err)

	var receivedAddr net.IP
	var receivedName string
	var recvChan chan interface{}
	receivedCount := 0

	reset := func() {
		receivedAddr = nil
		receivedName = ""
		receivedCount = 0
		recvChan = make(chan interface{})
	}

	wait := func() {
		select {
		case <-recvChan:
			return
		case <-time.After(100 * time.Millisecond):
			return
		}
	}

	// Implement a minimal listener for responses
	multicast, err := LinkLocalMulticastListener(nil)
	wt.AssertNoErr(t, err)

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		// Only handle responses here
		if len(r.Answer) > 0 {
			for _, answer := range r.Answer {
				switch rr := answer.(type) {
				case *dns.A:
					receivedAddr = rr.A
					receivedCount++
				case *dns.PTR:
					receivedName = rr.Ptr
					receivedCount++
				}
			}
			recvChan <- "ok"
		}
	}

	listener := &dns.Server{Unsafe: true, PacketConn: multicast, Handler: dns.HandlerFunc(handleMDNS)}
	go listener.ActivateAndServe()
	defer listener.Shutdown()

	time.Sleep(100 * time.Millisecond) // Allow for server to get going

	reset()
	sendQuery(testName, dns.TypeA)
	wait()

	if receivedCount != 1 {
		t.Fatalf("Unexpected result count %d for %s", receivedCount, testName)
	}
	if !receivedAddr.Equal(ip) {
		t.Fatalf("Unexpected result %s for %s", receivedAddr, testName)
	}

	reset()
	sendQuery("testfail.weave.", dns.TypeA)
	wait()

	if receivedCount != 0 {
		t.Fatalf("Unexpected result count %d for testfail.weave", receivedCount)
	}

	reset()
	sendQuery(testInAddr1, dns.TypePTR)
	wait()

	if receivedCount != 1 {
		t.Fatalf("Expected an answer to %s, got %d answers", testInAddr1, receivedCount)
	} else if !(testName == receivedName) {
		t.Fatalf("Expected answer %s to query for %s, got %s", testName, testInAddr1, receivedName)
	}
}
