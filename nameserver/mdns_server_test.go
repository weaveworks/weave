package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
	"net"
	"testing"
	"time"
)

func TestServerSimpleQuery(t *testing.T) {
	var (
		testRecord1 = Record{"test.weave.local.", net.ParseIP("10.20.20.10"), 0, 0, 0}
		testRecord2 = Record{"test.weave.local.", net.ParseIP("10.20.20.20"), 0, 0, 0}
		testInAddr1 = "10.20.20.10.in-addr.arpa."
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestServerSimpleQuery starting")

	mzone := newMockedZoneWithRecords([]ZoneRecord{testRecord1, testRecord2})
	mdnsServer, err := NewMDNSServer(mzone)
	wt.AssertNoErr(t, err)
	err = mdnsServer.Start(nil)
	wt.AssertNoErr(t, err)
	defer mdnsServer.Stop()

	receivedAddrs := make([]net.IP, 0)
	receivedName := ""
	recvChan := make(chan interface{})
	receivedCount := 0

	// Implement a minimal listener for responses
	multicast, err := LinkLocalMulticastListener(nil)
	wt.AssertNoErr(t, err)

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		// Only handle responses here
		if len(r.Answer) > 0 {
			t.Logf("Received %d answer(s)", len(r.Answer))
			for _, answer := range r.Answer {
				switch rr := answer.(type) {
				case *dns.A:
					t.Logf("... A:\n%+v", rr)
					receivedAddrs = append(receivedAddrs, rr.A)
					receivedCount++
				case *dns.PTR:
					t.Logf("... PTR:\n%+v", rr)
					receivedName = rr.Ptr
					receivedCount++
				}
			}
			recvChan <- "ok"
		}
	}

	sendQuery := func(name string, querytype uint16) {
		receivedAddrs = make([]net.IP, 0)
		receivedName = ""
		receivedCount = 0
		recvChan = make(chan interface{})

		m := new(dns.Msg)
		m.SetQuestion(name, querytype)
		m.RecursionDesired = false
		buf, err := m.Pack()
		wt.AssertNoErr(t, err)
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		wt.AssertNoErr(t, err)
		Debug.Printf("Sending UDP packet to %s", ipv4Addr)
		_, err = conn.WriteTo(buf, ipv4Addr)
		wt.AssertNoErr(t, err)

		Debug.Printf("Waiting for response")
		select {
		case <-recvChan:
			return
		case <-time.After(100 * time.Millisecond):
			Debug.Printf("Timeout while waiting for response")
			return
		}
	}

	listener := &dns.Server{
		Unsafe:      true,
		PacketConn:  multicast,
		Handler:     dns.HandlerFunc(handleMDNS),
		ReadTimeout: 100 * time.Millisecond}
	go listener.ActivateAndServe()
	defer listener.Shutdown()

	time.Sleep(100 * time.Millisecond) // Allow for server to get going

	Debug.Printf("Checking that we get 2 IPs fo name '%s' [A]", testRecord1.Name())
	sendQuery(testRecord1.Name(), dns.TypeA)
	if receivedCount != 2 {
		t.Fatalf("Unexpected result count %d for %s", receivedCount, testRecord1.Name())
	}
	if !(receivedAddrs[0].Equal(testRecord1.IP()) || receivedAddrs[0].Equal(testRecord2.IP())) {
		t.Fatalf("Unexpected result %s for %s", receivedAddrs, testRecord1.Name())
	}
	if !(receivedAddrs[1].Equal(testRecord1.IP()) || receivedAddrs[1].Equal(testRecord2.IP())) {
		t.Fatalf("Unexpected result %s for %s", receivedAddrs, testRecord1.Name())
	}

	Debug.Printf("Checking that 'testfail.weave.' [A] gets no answers")
	sendQuery("testfail.weave.", dns.TypeA)
	if receivedCount != 0 {
		t.Fatalf("Unexpected result count %d for testfail.weave", receivedCount)
	}

	Debug.Printf("Checking that '%s' [PTR] gets one name", testInAddr1)
	sendQuery(testInAddr1, dns.TypePTR)
	if receivedCount != 1 {
		t.Fatalf("Expected an answer to %s, got %d answers", testInAddr1, receivedCount)
	} else if !(testRecord1.Name() == receivedName) {
		t.Fatalf("Expected answer %s to query for %s, got %s", testRecord1.Name(), testInAddr1, receivedName)
	}
}
