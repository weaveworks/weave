package nameserver

import (
	"encoding/binary"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
	"time"
)

const (
	testRDNSsuccess  = "1.2.2.10.in-addr.arpa."
	testRDNSfail     = "4.3.2.1.in-addr.arpa."
	testRDNSnonlocal = "8.8.8.8.in-addr.arpa."
	testUDPBufSize   = 16384
)

var (
	testPort = 17625
)

func setupForTest() {
	testPort++
}

func TestUDPDNSServer(t *testing.T) {
	setupForTest()
	const (
		successTestName = "test1.weave.local."
		failTestName    = "test2.weave.local."
		nonLocalName    = "weave.works."
		testAddr1       = "10.2.2.1"
	)
	testCIDR1 := testAddr1 + "/24"

	InitDefaultLogging(true)
	var zone = new(ZoneDb)
	ip, _, _ := net.ParseCIDR(testCIDR1)
	zone.AddRecord(containerID, successTestName, ip)

	fallbackHandler := func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		if len(req.Question) == 1 {
			q := req.Question[0]
			if q.Name == nonLocalName && q.Qtype == dns.TypeMX {
				m.Answer = make([]dns.RR, 1)
				m.Answer[0] = &dns.MX{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 0}, Mx: "mail." + nonLocalName}
			} else if q.Name == nonLocalName && q.Qtype == dns.TypeANY {
				m.Answer = make([]dns.RR, 512/len("mailn."+nonLocalName)+1)
				for i := range m.Answer {
					m.Answer[i] = &dns.MX{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 0}, Mx: fmt.Sprintf("mail%d.%s", i, nonLocalName)}
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

	// Run another DNS server for fallback
	s, fallbackAddr, err := runLocalUDPServer(t, "127.0.0.1:0", fallbackHandler)
	wt.AssertNoErr(t, err)
	defer s.Shutdown()

	_, fallbackPort, err := net.SplitHostPort(fallbackAddr)
	wt.AssertNoErr(t, err)

	config := &dns.ClientConfig{Servers: []string{"127.0.0.1"}, Port: fallbackPort}
	srv, err := NewDNSServer(DNSServerConfig{UpstreamCfg: config, Port: testPort}, zone, nil)
	wt.AssertNoErr(t, err)
	defer srv.Stop()
	go srv.Start()
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	var r *dns.Msg

	r = assertExchange(t, successTestName, dns.TypeA, 1, 1, 0)
	wt.AssertType(t, r.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, r.Answer[0].(*dns.A).A.String(), testAddr1, "IP address")

	assertExchange(t, failTestName, dns.TypeA, 0, 0, dns.RcodeNameError)

	r = assertExchange(t, testRDNSsuccess, dns.TypePTR, 1, 1, 0)
	wt.AssertType(t, r.Answer[0], (*dns.PTR)(nil), "DNS record")
	wt.AssertEqualString(t, r.Answer[0].(*dns.PTR).Ptr, successTestName, "IP address")

	assertExchange(t, testRDNSfail, dns.TypePTR, 0, 0, dns.RcodeNameError)

	// This should fail because we don't handle MX records
	assertExchange(t, successTestName, dns.TypeMX, 0, 0, dns.RcodeNotImplemented)

	// This non-local query for an MX record should succeed by being
	// passed on to the fallback server
	assertExchange(t, nonLocalName, dns.TypeMX, 1, -1, 0)

	// Now ask a query that we expect to return a lot of data.
	assertExchange(t, nonLocalName, dns.TypeANY, 5, -1, 0)

	assertExchange(t, testRDNSnonlocal, dns.TypePTR, 1, -1, 0)

	// Not testing MDNS functionality of server here (yet), since it
	// needs two servers, each listening on its own address
}

func TestTCPDNSServer(t *testing.T) {
	setupForTest()
	const (
		numAnswers   = 512
		nonLocalName = "weave.works."
	)
	dnsAddr := fmt.Sprintf("localhost:%d", testPort)

	InitDefaultLogging(true)
	var zone = new(ZoneDb)

	// generate a list of `numAnswers` IP addresses
	var addrs []net.IP
	bs := make([]byte, 4)
	for i := 0; i < numAnswers; i++ {
		binary.LittleEndian.PutUint32(bs, uint32(i))
		addrs = append(addrs, net.IPv4(bs[0], bs[1], bs[2], bs[3]))
	}

	// handler for the fallback server: it will just return a very long response
	fallbackUDPHandler := func(w dns.ResponseWriter, req *dns.Msg) {
		maxLen := getMaxReplyLen(req, protUDP)

		t.Logf("Fallback UDP server got asked: returning %d answers", numAnswers)
		q := req.Question[0]
		m := makeAddressReply(req, &q, addrs)
		mLen := m.Len()
		m.SetEdns0(uint16(maxLen), false)

		if mLen > maxLen {
			t.Logf("... truncated response (%d > %d)", mLen, maxLen)
			m.Truncated = true
		}
		w.WriteMsg(m)
	}
	fallbackTCPHandler := func(w dns.ResponseWriter, req *dns.Msg) {
		t.Logf("Fallback TCP server got asked: returning %d answers", numAnswers)
		q := req.Question[0]
		m := makeAddressReply(req, &q, addrs)
		w.WriteMsg(m)
	}

	t.Logf("Running a DNS fallback server with UDP")
	us, fallbackUDPAddr, err := runLocalUDPServer(t, "127.0.0.1:0", fallbackUDPHandler)
	wt.AssertNoErr(t, err)
	defer us.Shutdown()

	_, fallbackPort, err := net.SplitHostPort(fallbackUDPAddr)
	wt.AssertNoErr(t, err)

	t.Logf("Starting another fallback server, with TCP, on the same port as the UDP server")
	fallbackTCPAddr := fmt.Sprintf("127.0.0.1:%s", fallbackPort)
	ts, fallbackTCPAddr, err := runLocalTCPServer(t, fallbackTCPAddr, fallbackTCPHandler)
	wt.AssertNoErr(t, err)
	defer ts.Shutdown()

	t.Logf("Creating a WeaveDNS server instance, falling back to 127.0.0.1:%s", fallbackPort)
	config := &dns.ClientConfig{Servers: []string{"127.0.0.1"}, Port: fallbackPort}
	srv, err := NewDNSServer(DNSServerConfig{UpstreamCfg: config, Port: testPort}, zone, nil)
	wt.AssertNoErr(t, err)
	defer srv.Stop()
	go srv.Start()
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	t.Logf("Creating a UDP and a TCP client")
	uc := new(dns.Client)
	uc.UDPSize = minUDPSize
	tc := new(dns.Client)
	tc.Net = "tcp"

	t.Logf("Creating DNS query message")
	m := new(dns.Msg)
	m.RecursionDesired = true
	m.SetQuestion(nonLocalName, dns.TypeA)

	t.Logf("Checking the fallback server at %s returns a truncated response with UDP", fallbackUDPAddr)
	r, _, err := uc.Exchange(m, fallbackUDPAddr)
	t.Logf("Got response from fallback server (UDP) with %d answers", len(r.Answer))
	t.Logf("Response:\n%+v\n", r)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, r.MsgHdr.Truncated, "DNS truncated reponse flag")
	wt.AssertNotEqualInt(t, len(r.Answer), numAnswers, "number of answers (UDP)")

	t.Logf("Checking the WeaveDNS server at %s returns a truncated reponse with UDP", dnsAddr)
	r, _, err = uc.Exchange(m, dnsAddr)
	t.Logf("UDP Response:\n%+v\n", r)
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, r, "response")
	t.Logf("%d answers", len(r.Answer))
	wt.AssertTrue(t, r.MsgHdr.Truncated, "DNS truncated reponse flag")
	wt.AssertNotEqualInt(t, len(r.Answer), numAnswers, "number of answers (UDP)")

	t.Logf("Checking the WeaveDNS server at %s does not return a truncated reponse with TCP", dnsAddr)
	r, _, err = tc.Exchange(m, dnsAddr)
	t.Logf("TCP Response:\n%+v\n", r)
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, r, "response")
	t.Logf("%d answers", len(r.Answer))
	wt.AssertFalse(t, r.MsgHdr.Truncated, "DNS truncated response flag")
	wt.AssertEqualInt(t, len(r.Answer), numAnswers, "number of answers (TCP)")

	t.Logf("Checking the WeaveDNS server at %s does not return a truncated reponse with UDP with a bigger buffer", dnsAddr)
	m.SetEdns0(testUDPBufSize, false)
	r, _, err = uc.Exchange(m, dnsAddr)
	t.Logf("UDP-large Response:\n%+v\n", r)
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, r, "response")
	t.Logf("%d answers", len(r.Answer))
	wt.AssertNoErr(t, err)
	wt.AssertFalse(t, r.MsgHdr.Truncated, "DNS truncated response flag")
	wt.AssertEqualInt(t, len(r.Answer), numAnswers, "number of answers (UDP-long)")
}

//////////////////////////////////////////////////////////////////////////////

// perform a DNS query and assert the reply code, number or answers, etc
func assertExchange(t *testing.T, z string, ty uint16, minAnswers int, maxAnswers int, expErr int) *dns.Msg {
	c := new(dns.Client)
	c.UDPSize = testUDPBufSize
	m := new(dns.Msg)
	m.RecursionDesired = true
	m.SetQuestion(z, ty)
	m.SetEdns0(testUDPBufSize, false) // we don't want to play with truncation here...

	r, _, err := c.Exchange(m, fmt.Sprintf("127.0.0.1:%d", testPort))
	t.Logf("Response:\n%+v\n", r)
	wt.AssertNoErr(t, err)
	if minAnswers == 0 && maxAnswers == 0 {
		wt.AssertStatus(t, r.Rcode, expErr, "DNS response code")
	} else {
		wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	}
	answers := len(r.Answer)
	if minAnswers >= 0 && answers < minAnswers {
		wt.Fatalf(t, "Number of answers >= %d", minAnswers)
	}
	if maxAnswers >= 0 && answers > maxAnswers {
		wt.Fatalf(t, "Number of answers <= %d", maxAnswers)
	}
	return r
}

// run a UDP fallback server
func runLocalUDPServer(t *testing.T, laddr string, handler dns.HandlerFunc) (*dns.Server, string, error) {
	t.Logf("Starting fallback UDP server at %s", laddr)
	pc, err := net.ListenPacket("udp", laddr)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{PacketConn: pc, Handler: handler}

	go func() {
		server.ActivateAndServe()
		pc.Close()
	}()

	t.Logf("Fallback UDP server listening at %s", pc.LocalAddr())
	return server, pc.LocalAddr().String(), nil
}

// run a TCP fallback server
func runLocalTCPServer(t *testing.T, laddr string, handler dns.HandlerFunc) (*dns.Server, string, error) {
	t.Logf("Starting fallback TCP server at %s", laddr)
	laddrTCP, err := net.ResolveTCPAddr("tcp", laddr)
	if err != nil {
		return nil, "", err
	}

	l, err := net.ListenTCP("tcp", laddrTCP)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{Listener: l, Handler: handler}

	go func() {
		server.ActivateAndServe()
		l.Close()
	}()

	t.Logf("Fallback TCP server listening at %s", l.Addr().String())
	return server, l.Addr().String(), nil
}
