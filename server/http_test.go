package weavedns

import (
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestHttp(t *testing.T) {
	var (
		success_test_name = "test1.weave."
		test_addr1        = "10.0.2.1/24"
		docker_ip         = "9.8.7.6"
	)

	var zone = new(ZoneDb)
	go ListenHttp(zone)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	resp, err := http.PostForm("http://localhost:6785/add",
		url.Values{"name": {success_test_name}, "ip": {docker_ip}, "weave_cidr": {test_addr1}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Log("Unexpected http response", resp.Status)
		t.Fail()
	}

	ip, err := zone.MatchLocal(success_test_name)
	if err != nil {
		t.Log(zone)
		t.Fatal(err)
	}
	weave_ip, _, _ := net.ParseCIDR(test_addr1)
	if !ip.Equal(weave_ip) {
		t.Log("Unexpected result for", success_test_name, ip)
		t.Fail()
	}

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
