package sortinghat

import (
	"fmt"
	"github.com/zettio/weave/router"
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
		testAddr1   = "10.0.3.4"
	)
	const (
		ourUID  = 123456
		peerUID = 654321
	)

	ourName, _ := router.PeerNameFromString("08:00:27:01:c3:9a")
	alloc := NewAllocator(ourName, ourUID, nil, net.ParseIP(testAddr1), 3)
	alloc.manageSpace(net.ParseIP(testAddr1), 3)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go ListenHttp(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	// Ask the http server for a new address
	addr1 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	if addr1 != testAddr1 {
		t.Fatalf("Expected address %s but got %s", testAddr1, addr1)
	}

	// Ask the http server for another address and check it's different
	addr2 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container2))
	if addr2 == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr2)
	}

	// Now free the first one, and we should get it back when we ask
	alloc.Free(net.ParseIP(addr1))
	addr3 := HttpGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container2))
	if addr3 != testAddr1 {
		t.Fatalf("Expected address %s but got %s", testAddr1, addr1)
	}

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}
