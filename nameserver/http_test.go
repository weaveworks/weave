package nameserver

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	wt "github.com/weaveworks/weave/testing"
)

func genForm(method string, url string, data url.Values) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return http.DefaultClient.Do(req)
}

func TestHttp(t *testing.T) {
	var (
		containerID     = "deadbeef"
		testDomain      = "weave.local."
		successTestName = "test1." + testDomain
		testAddr1       = "10.2.2.1/24"
		testAddr2       = "10.2.2.2/24"
		dockerIP        = "9.8.7.6"
	)

	zone, err := NewZoneDb(ZoneConfig{Domain: testDomain})
	wt.AssertNoErr(t, err)
	err = zone.Start()
	wt.AssertNoErr(t, err)
	defer zone.Stop()

	httpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal("Unable to create http listener: ", err)
	}

	// Create a useless server... it will not even be started
	srv, _ := NewDNSServer(DNSServerConfig{Zone: zone})

	port := httpListener.Addr().(*net.TCPAddr).Port
	go ServeHTTP(httpListener, "", srv)

	// Ask the http server to add our test address into the database
	addrParts := strings.Split(testAddr1, "/")
	addrURL := fmt.Sprintf("http://localhost:%d/name/%s/%s", port, containerID, addrParts[0])
	resp, err := genForm("PUT", addrURL,
		url.Values{"fqdn": {successTestName}, "local_ip": {dockerIP}, "routing_prefix": {addrParts[1]}})
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")

	// Check that the address is now there.
	ip, _, _ := net.ParseCIDR(testAddr1)
	foundIP, err := zone.LookupName(successTestName)
	wt.AssertNoErr(t, err)
	if !foundIP[0].IP().Equal(ip) {
		t.Fatalf("Unexpected result for %s: received %s, expected %s", successTestName, foundIP, ip)
	}

	// Adding exactly the same address should be OK
	resp, err = genForm("PUT", addrURL,
		url.Values{"fqdn": {successTestName}, "local_ip": {dockerIP}, "routing_prefix": {addrParts[1]}})
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http success response for duplicate add")

	// Now try adding the same address again with a different ident --
	// again should be fine
	otherURL := fmt.Sprintf("http://localhost:%d/name/%s/%s", port, "other", addrParts[0])
	resp, err = genForm("PUT", otherURL,
		url.Values{"fqdn": {successTestName}, "local_ip": {dockerIP}, "routing_prefix": {addrParts[1]}})
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")

	// Adding a new IP for the same name should be OK
	addrParts2 := strings.Split(testAddr2, "/")
	addrURL2 := fmt.Sprintf("http://localhost:%d/name/%s/%s", port, containerID, addrParts2[0])
	resp, err = genForm("PUT", addrURL2,
		url.Values{"fqdn": {successTestName}, "local_ip": {dockerIP}, "routing_prefix": {addrParts2[1]}})
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http success response for second IP")

	// Check that we can get two IPs for that name
	ip2, _, _ := net.ParseCIDR(testAddr2)
	foundIP, err = zone.LookupName(successTestName)
	wt.AssertNoErr(t, err)
	if len(foundIP) != 2 {
		t.Logf("IPs found: %s", foundIP)
		t.Fatalf("Unexpected result length: received %d responses", len(foundIP))
	}
	if !(foundIP[0].IP().Equal(ip) || foundIP[0].IP().Equal(ip2)) {
		t.Logf("IPs found: %s", foundIP)
		t.Fatalf("Unexpected result for %s: received %s, expected %s", successTestName, foundIP, ip)
	}
	if !(foundIP[1].IP().Equal(ip) || foundIP[1].IP().Equal(ip2)) {
		t.Logf("IPs found: %s", foundIP)
		t.Fatalf("Unexpected result for %s: received %s, expected %s", successTestName, foundIP, ip)
	}

	// Delete the address
	resp, err = genForm("DELETE", addrURL, nil)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")

	// Check that the address is still resolvable.
	x, err := zone.LookupName(successTestName)
	t.Logf("Got %s", x)
	wt.AssertNoErr(t, err)

	// Delete the address record mentioning the other container
	resp, err = genForm("DELETE", otherURL, nil)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")

	// Delete the second IP
	resp, err = genForm("DELETE", addrURL2, nil)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")

	// Check that the address is gone
	x, err = zone.LookupName(successTestName)
	t.Logf("Got %s", x)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "fully-removed address")

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
