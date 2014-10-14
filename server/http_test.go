package weavedns

import (
	"fmt"
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
		container_id      = "deadbeef"
		success_test_name = "test1.weave."
		test_addr1        = "10.0.2.1/24"
		docker_ip         = "9.8.7.6"
	)

	var zone = new(ZoneDb)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go ListenHttp(zone, port)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	// Ask the http server to add our test address into the database
	addr_parts := strings.Split(test_addr1, "/")
	resp, err := genForm("PUT", fmt.Sprintf("http://localhost:%d/name/%s/%s", port, container_id, addr_parts[0]),
		url.Values{"fqdn": {success_test_name}, "local_ip": {docker_ip}, "routing_prefix": {addr_parts[1]}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal("Unexpected http response", resp.Status)
	}

	// Check that the address is now there.
	ip, err := zone.MatchLocal(success_test_name)
	if err != nil {
		t.Fatal(err)
	}
	weave_ip, _, _ := net.ParseCIDR(test_addr1)
	if !ip.Equal(weave_ip) {
		t.Fatal("Unexpected result for", success_test_name, ip)
	}

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
