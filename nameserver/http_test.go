package nameserver

import (
	"fmt"
	wt "github.com/weaveworks/weave/testing"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
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
		dockerIP        = "9.8.7.6"
	)

	zone := NewZoneDb(DefaultLocalDomain)

	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go ListenHTTP("", nil, testDomain, zone, port)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

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

	// Check that the address is gone
	x, err = zone.LookupName(successTestName)
	t.Logf("Got %s", x)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "fully-removed address")

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
