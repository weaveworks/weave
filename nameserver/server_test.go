package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	"github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
	"time"
)

const (
	testRDNSsuccess  = "1.2.0.10.in-addr.arpa."
	testRDNSfail     = "4.3.2.1.in-addr.arpa."
	testRDNSnonlocal = "8.8.8.8.in-addr.arpa."
)

func TestDNSServer(t *testing.T) {
	const (
		port            = 17625
		successTestName = "test1.weave.local."
		failTestName    = "test2.weave.local."
		nonLocalName    = "weave.works."
		testAddr1       = "10.0.2.1"
	)
	dnsAddr := fmt.Sprintf("localhost:%d", port)
	testCIDR1 := testAddr1 + "/24"

	common.InitDefaultLogging(true)
	var zone = new(ZoneDb)
	ip, _, _ := net.ParseCIDR(testCIDR1)
	zone.AddRecord(containerID, successTestName, ip)

	// Run another DNS server for fallback
	s, fallbackAddr, err := RunLocalUDPServer("127.0.0.1:0")
	wt.AssertNoErr(t, err)
	defer s.Shutdown()

	_, fallbackPort, err := net.SplitHostPort(fallbackAddr)
	wt.AssertNoErr(t, err)

	config := &dns.ClientConfig{Servers: []string{"127.0.0.1"}, Port: fallbackPort}
	srv, err := NewDNSServerWithConfig(config, zone, nil, port, port)
	wt.AssertNoErr(t, err)
	go srv.Start()
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	c := new(dns.Client)
	c.UDPSize = UDPBufSize
	m := new(dns.Msg)
	m.SetQuestion(successTestName, dns.TypeA)
	m.RecursionDesired = true
	r, _, err := c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	wt.AssertEqualInt(t, len(r.Answer), 1, "Number of answers")
	wt.AssertType(t, r.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, r.Answer[0].(*dns.A).A.String(), testAddr1, "IP address")

	m.SetQuestion(failTestName, dns.TypeA)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeNameError, "DNS response code")
	wt.AssertEqualInt(t, len(r.Answer), 0, "Number of answers")

	m.SetQuestion(testRDNSsuccess, dns.TypePTR)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	wt.AssertEqualInt(t, len(r.Answer), 1, "Number of answers")
	wt.AssertType(t, r.Answer[0], (*dns.PTR)(nil), "DNS record")
	wt.AssertEqualString(t, r.Answer[0].(*dns.PTR).Ptr, successTestName, "IP address")
	m.SetQuestion(testRDNSfail, dns.TypePTR)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeNameError, "DNS response code")
	wt.AssertEqualInt(t, len(r.Answer), 0, "Number of answers")

	// This should fail because we don't handle MX records
	m.SetQuestion(successTestName, dns.TypeMX)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeNameError, "DNS response code")
	wt.AssertEqualInt(t, len(r.Answer), 0, "Number of answers")

	// This non-local query for an MX record should succeed by being
	// passed on to the fallback server
	m.SetQuestion(nonLocalName, dns.TypeMX)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	if !(len(r.Answer) > 0) {
		t.Fatal("Number of answers > 0")
	}
	// Now ask a query that we expect to return a lot of data.
	m.SetQuestion(nonLocalName, dns.TypeANY)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	if !(len(r.Extra) > 5) {
		t.Fatal("Number of answers > 5")
	}

	m.SetQuestion(testRDNSnonlocal, dns.TypePTR)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS success response code")
	if !(len(r.Answer) > 0) {
		t.Fatal("Number of answers > 0")
	}

	// Not testing MDNS functionality of server here (yet), since it
	// needs two servers, each listening on its own address
}

func fallbackHandler(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	if len(req.Question) == 1 {
		q := req.Question[0]
		if q.Name == "weave.works." && q.Qtype == dns.TypeMX {
			m.Answer = make([]dns.RR, 1)
			m.Answer[0] = &dns.MX{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 0}, Mx: "mail.weave.works."}
		} else if q.Name == "weave.works." && q.Qtype == dns.TypeANY {
			const N = 10
			m.Extra = make([]dns.RR, N)
			for i, _ := range m.Extra {
				m.Extra[i] = &dns.TXT{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}, Txt: []string{"Lots and lots and lots and lots and lots and lots and lots and lots and lots of data"}}
			}
		} else if q.Name == testRDNSnonlocal && q.Qtype == dns.TypePTR {
			m.Answer = make([]dns.RR, 1)
			m.Answer[0] = &dns.PTR{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 0}, Ptr: "ns1.google.com."}
		} else if q.Name == testRDNSfail && q.Qtype == dns.TypePTR {
			m.Rcode = dns.RcodeNameError
		}
	}
	w.WriteMsg(m)
}

func RunLocalUDPServer(laddr string) (*dns.Server, string, error) {
	pc, err := net.ListenPacket("udp", laddr)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(fallbackHandler)}

	go func() {
		server.ActivateAndServe()
		pc.Close()
	}()

	return server, pc.LocalAddr().String(), nil
}
