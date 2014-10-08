package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"testing"
	"time"
)

var (
	test_addr = net.ParseIP("9.8.7.6")
)

func minimalServer(w dns.ResponseWriter, req *dns.Msg) {
	//log.Println("minimalServer received:", req)
	if len(req.Answer) > 0 {
		return // Only interested in questions.
	}

	m := new(dns.Msg)
	m.SetReply(req)

	hdr := dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA,
		Class: dns.ClassINET, Ttl: 3600}
	a := &dns.A{hdr, test_addr}
	m.Answer = append(m.Answer, a)

	buf, err := m.Pack()
	if err != nil {
		log.Fatal(err)
	}
	if buf == nil {
		log.Fatal("Nil buffer")
	}
	//log.Println("minimalServer sending:", buf)
	// This is a bit of a kludge - per the RFC we should send responses from 5353, but that doesn't seem to work
	sendconn, err := net.DialUDP("udp", nil, ipv4Addr)
	if err != nil {
		log.Fatal(err)
	}
	_, err = sendconn.Write(buf)
	sendconn.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func RunLocalMulticastServer() (*dns.Server, error) {
	mux := dns.NewServeMux()
	mux.HandleFunc("weave", minimalServer)
	multicast, err := net.ListenMulticastUDP("udp", nil, ipv4Addr)
	if err != nil {
		return nil, err
	}
	server := &dns.Server{Listener: nil, PacketConn: multicast, Handler: mux}
	go server.ActivateAndServe()
	return server, nil
}

func TestSimpleQuery(t *testing.T) {
	log.Println("TestSimpleQuery starting")
	mdnsClient, err := NewMDNSClient()
	if err != nil {
		t.Fatal(err)
	}
	err = mdnsClient.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	server, err := RunLocalMulticastServer()
	if err != nil {
		t.Fatalf("Unable to run test server: %s", err)
	}
	defer server.Shutdown()

	var received_addr net.IP

	channel := make(chan *ResponseA, 4)
	go func() {
		for resp := range channel {
			log.Printf("Got address response %s addr %s", resp.Name, resp.Addr)
			received_addr = resp.Addr
		}
	}()

	mdnsClient.SendQuery("test.weave.", dns.TypeA, channel)

	time.Sleep(200 * time.Millisecond)

	if !received_addr.Equal(test_addr) {
		t.Log("Unexpected result for test.weave", received_addr)
		t.Fail()
	}
}
