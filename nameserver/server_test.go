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

func TestDNSServer(t *testing.T) {
	const (
		port            = 17625
		successTestName = "test1.weave.local."
		failTestName    = "test2.weave.local."
		nonLocalName    = "weave.works."
		testAddr1       = "10.0.2.1"
		testRDNSsuccess = "1.2.0.10.in-addr.arpa."
		testRDNSfail    = "4.3.2.1.in-addr.arpa."
	)
	dnsAddr := fmt.Sprintf("localhost:%d", port)
	testCIDR1 := testAddr1 + "/24"

	common.InitDefaultLogging(true)
	var zone = new(ZoneDb)
	ip, _, _ := net.ParseCIDR(testCIDR1)
	zone.AddRecord(containerID, successTestName, ip)

	go StartServer(zone, nil, port, 0)
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	c := new(dns.Client)
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
	// passed on to the configured (/etc/resolv.conf) DNS server.
	m.SetQuestion(nonLocalName, dns.TypeMX)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS response code")
	if !(len(r.Answer) > 0) {
		t.Fatal("Number of answers > 0")
	}

	m.SetQuestion("8.8.8.8.in-addr.arpa.", dns.TypePTR)
	r, _, err = c.Exchange(m, dnsAddr)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, r.Rcode, dns.RcodeSuccess, "DNS success response code")
	if !(len(r.Answer) > 0) {
		t.Fatal("Number of answers > 0")
	}

	// Not testing MDNS functionality of server here (yet), since it
	// needs two servers, each listening on its own address
}
