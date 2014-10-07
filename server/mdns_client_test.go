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
	sendconn  *net.UDPConn
	sendAddr  = &net.UDPAddr{
		IP:   net.ParseIP(ipv4mdns),
		Port: 0,
	}
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
	_, err = sendconn.WriteTo(buf, ipv4Addr)
	if err != nil {
		log.Fatal(err)
	}
}

func RunLocalMulticastServer() error {
	mux := dns.NewServeMux()
	mux.HandleFunc("weave", minimalServer)
	multicast, err := net.ListenMulticastUDP("udp", nil, ipv4Addr)
	if err != nil {
		return err
	}
	go dns.ActivateAndServe(nil, multicast, mux)
	return nil
}

func TestSimpleQuery(t *testing.T) {
	log.Println("TestSimpleQuery starting")
	mdnsClient, err := NewMDNSClient()
	if err != nil {
		t.Fatal(err)
	}
	mdnsClient.Start()
	mux := dns.NewServeMux()

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		//log.Println("test received:", r)
		// Only handle responses here
		if len(r.Answer) > 0 {
			mdnsClient.ResponseCallback(r)
		}
	}

	mux.HandleFunc("weave", handleMDNS)
	multicast, err := net.ListenMulticastUDP("udp", nil, ipv4Addr)
	if err != nil {
		t.Fatal(err)
	}
	go dns.ActivateAndServe(nil, multicast, mux)
	sendconn, err = net.ListenUDP("udp", sendAddr)
	if err != nil {
		t.Fatal(err)
	}

	err = RunLocalMulticastServer()
	if err != nil {
		t.Fatalf("Unable to run test server: %s", err)
	}

	var received_addr net.IP

	channel := make(chan *ResponseA, 4)
	go func() {
		for resp := range channel {
			log.Printf("Got address response %s addr %s", resp.Name, resp.Addr)
			received_addr = resp.Addr
		}
	}()
	mdnsClient.SendQuery("test.weave.", dns.TypeA, channel)

	time.Sleep(1 * time.Second)

	if !received_addr.Equal(test_addr) {
		t.Log("Unexpected result for test.weave", received_addr)
		t.Fail()
	}
}
