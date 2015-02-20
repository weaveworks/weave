package ipam

import (
	"fmt"
	wt "github.com/zettio/weave/testing"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"testing"
	"time"
)

func HttpGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	return string(body)
}

func TestHttp(t *testing.T) {
	var (
		containerID = "deadbeef"
		container2  = "baddf00d"
		container3  = "b01df00d"
		testAddr1   = "10.0.3.5"
		testCIDR1   = "10.0.3.5/29"
	)
	const (
		ourUID  = 123456
		peerUID = 654321
	)

	alloc := testAllocator(t, "08:00:27:01:c3:9a", ourUID, testCIDR1).addSpace(testAddr1, 4)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go ListenHttp(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	// Ask the http server for a new address
	cidr1 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	wt.AssertEqualString(t, cidr1, testCIDR1, "address")

	// Ask the http server for another address and check it's different
	cidr2 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container2))
	wt.AssertNotEqualString(t, cidr2, testCIDR1, "address")

	// Ask for the first container again and we should get the same address again
	cidr1a := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	wt.AssertEqualString(t, cidr1a, testCIDR1, "address")

	// Now free the first one, and we should get it back when we ask
	addr1, _, err := net.ParseCIDR(cidr1)
	wt.AssertNoErr(t, err)
	wt.AssertNoErr(t, alloc.Free(addr1))
	cidr3 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container3))
	wt.AssertEqualString(t, cidr3, testCIDR1, "address")

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
