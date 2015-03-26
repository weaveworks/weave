package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"log"
	"net"
	"testing"
	"time"
)

var (
	successTestName = "test1.weave."
	failTestName    = "test2.weave."
	testAddr        = net.ParseIP("9.8.7.6")
	testInAddr      = "6.7.8.9.in-addr.arpa."
)

func minimalServer(w dns.ResponseWriter, req *dns.Msg) {

	sendAnswer := func(m *dns.Msg) {
		buf, err := m.Pack()
		CheckFatal(err)
		if buf == nil {
			log.Fatal("Nil buffer")
		}
		//log.Println("minimalServer sending:", buf)
		// This is a bit of a kludge - per the RFC we should send responses from 5353, but that doesn't seem to work
		sendconn, err := net.DialUDP("udp", nil, ipv4Addr)
		CheckFatal(err)

		_, err = sendconn.Write(buf)
		sendconn.Close()
		CheckFatal(err)
	}

	//log.Println("minimalServer received:", req)
	if len(req.Answer) > 0 {
		return // Only interested in questions.
	}
	if len(req.Question) != 1 {
		return // We only handle single-question messages
	}

	if req.Question[0].Qtype == dns.TypeA && req.Question[0].Name == successTestName {
		m := new(dns.Msg)
		m.SetReply(req)
		hdr := dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA,
			Class: dns.ClassINET, Ttl: 3600}
		a := &dns.A{Hdr: hdr, A: testAddr}
		m.Answer = append(m.Answer, a)
		sendAnswer(m)
	} else if req.Question[0].Qtype == dns.TypePTR && req.Question[0].Name == testInAddr {
		m := new(dns.Msg)
		m.SetReply(req)
		hdr := dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypePTR,
			Class: dns.ClassINET, Ttl: 3600}
		a := &dns.PTR{Hdr: hdr, Ptr: successTestName}
		m.Answer = append(m.Answer, a)
		sendAnswer(m)
	}
}

func RunLocalMulticastServer() (*dns.Server, error) {
	multicast, err := net.ListenMulticastUDP("udp", nil, ipv4Addr)
	if err != nil {
		return nil, err
	}
	server := &dns.Server{Listener: nil, PacketConn: multicast, Handler: dns.HandlerFunc(minimalServer)}
	go server.ActivateAndServe()
	return server, nil
}

func setup(t *testing.T) (*MDNSClient, *dns.Server, error) {
	mdnsClient, err := NewMDNSClient()
	wt.AssertNoErr(t, err)
	err = mdnsClient.Start(nil)
	wt.AssertNoErr(t, err)

	server, err := RunLocalMulticastServer()
	if err != nil {
		t.Fatalf("Unable to run test server: %s", err)
	}
	return mdnsClient, server, err
}

func (c *MDNSClient) Shutdown() {
	c.actionChan <- nil
}

type testContext struct {
	receivedAddr  net.IP
	receivedCount int
	channel       chan *Response
}

func newTestContext() *testContext {
	return &testContext{channel: make(chan *Response, 4)}
}

func (c *testContext) checkResponse(t *testing.T, channelOk bool, resp *Response) {
	if !channelOk {
		c.channel = nil
		return
	}
	wt.AssertNoErr(t, resp.Err)
	log.Printf("Got address response %s addr %s", resp.Name, resp.Addr)
	c.receivedAddr = resp.Addr
	c.receivedCount++
}

func TestSimpleQuery(t *testing.T) {
	log.Println("TestSimpleQuery starting")
	mdnsClient, server, _ := setup(t)
	defer mdnsClient.Shutdown()
	defer server.Shutdown()

	context := newTestContext()

	// First, a test we expect to succeed
	mdnsClient.SendQuery(successTestName, dns.TypeA, context.channel)
	for resp := range context.channel {
		context.checkResponse(t, true, resp)
	}

	if !context.receivedAddr.Equal(testAddr) || context.receivedCount != 1 {
		t.Fatal("Unexpected result for", successTestName, context.receivedAddr, context.receivedCount)
	}

	// Now, a test we expect to time out with no responses
	context = newTestContext()
	mdnsClient.SendQuery("test2.weave.", dns.TypeA, context.channel)
	for resp := range context.channel {
		context.checkResponse(t, true, resp)
	}

	if context.receivedCount > 0 {
		t.Fatal("Unexpected result for test2.weave", context.receivedAddr)
	}
}

func TestParallelQuery(t *testing.T) {
	log.Println("TestParallelQuery starting")
	mdnsClient, server, _ := setup(t)
	defer mdnsClient.Shutdown()
	defer server.Shutdown()

	context1 := newTestContext()
	context2 := newTestContext()

	go mdnsClient.SendQuery(successTestName, dns.TypeA, context1.channel)
	go mdnsClient.SendQuery(successTestName, dns.TypeA, context2.channel)
	timeout := time.After(2 * time.Second)
outerloop:
	for context1.channel != nil || context2.channel != nil {
		select {
		case resp, ok := <-context1.channel:
			context1.checkResponse(t, ok, resp)
		case resp, ok := <-context2.channel:
			context2.checkResponse(t, ok, resp)
		case <-timeout:
			break outerloop
		}
	}

	if !context1.receivedAddr.Equal(testAddr) || !context2.receivedAddr.Equal(testAddr) || context1.receivedCount != 1 || context2.receivedCount != 1 {
		t.Fatal("Unexpected result for", successTestName, context1.receivedAddr, context2.receivedAddr, context1.receivedCount, context2.receivedCount)
	}
}

func TestAsLookup(t *testing.T) {
	mdnsClient, server, _ := setup(t)
	defer mdnsClient.Shutdown()
	defer server.Shutdown()

	ip, err := mdnsClient.LookupName(successTestName)
	wt.AssertNoErr(t, err)
	if !testAddr.Equal(ip) {
		t.Fatalf("Returned address incorrect %s", ip)
	}

	ip, err = mdnsClient.LookupName("foo.example.com.")
	wt.AssertErrorType(t, err, (*LookupError)(nil), "unknown hostname")

	name, err := mdnsClient.LookupInaddr(testInAddr)
	wt.AssertNoErr(t, err)
	if !(successTestName == name) {
		t.Fatalf("Expected name %s, got %s", successTestName, name)
	}
}
