package ipam

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	wt "github.com/weaveworks/weave/testing"
)

func HTTPPost(t *testing.T, url string) string {
	resp, err := http.Post(url, "", nil)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	return string(body)
}

func HTTPGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	wt.AssertNoErr(t, err)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	return string(body)
}

func doHTTP(method string, url string) (resp *http.Response, err error) {
	req, _ := http.NewRequest(method, url, nil)
	return http.DefaultClient.Do(req)
}

func listenHTTP(alloc *Allocator, subnet address.CIDR) int {
	router := mux.NewRouter()
	router.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln(alloc))
	})
	alloc.HandleHTTP(router, subnet)

	httpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		common.Error.Fatal("Unable to create http listener: ", err)
	}

	go func() {
		srv := &http.Server{Handler: router}
		if err := srv.Serve(httpListener); err != nil {
			common.Error.Fatal("Unable to serve http: ", err)
		}
	}()
	return httpListener.Addr().(*net.TCPAddr).Port
}

func identURL(port int, containerID string) string {
	return fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID)
}

func allocURL(port int, cidr string, containerID string) string {
	return fmt.Sprintf("http://localhost:%d/ip/%s/%s", port, containerID, cidr)
}

func TestHttp(t *testing.T) {
	var (
		containerID = "deadbeef"
		container2  = "baddf00d"
		container3  = "b01df00d"
		universe    = "10.0.0.0/8"
		testAddr1   = "10.0.3.9/29"
		testCIDR1   = "10.0.3.8/29"
		testCIDR2   = "10.2.0.0/16"
		testAddr2   = "10.2.0.1/16"
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", universe, 1)
	port := listenHTTP(alloc, subnet)

	alloc.claimRingForTesting()
	cidr1a := HTTPPost(t, allocURL(port, testCIDR1, containerID))
	wt.AssertEqualString(t, cidr1a, testAddr1, "address")

	cidr2a := HTTPPost(t, allocURL(port, testCIDR2, containerID))
	wt.AssertEqualString(t, cidr2a, testAddr2, "address")

	check := HTTPGet(t, allocURL(port, testCIDR1, containerID))
	wt.AssertEqualString(t, check, cidr1a, "address")
	check = HTTPGet(t, allocURL(port, testCIDR2, containerID))
	wt.AssertEqualString(t, check, cidr2a, "address")

	// Ask the http server for a pair of addresses for another container and check they're different
	cidr1b := HTTPPost(t, allocURL(port, testCIDR1, container2))
	wt.AssertFalse(t, cidr1b == testAddr1, "address")
	cidr2b := HTTPPost(t, allocURL(port, testCIDR2, container2))
	wt.AssertFalse(t, cidr2b == testAddr2, "address")

	// Now free the first container, and we should get it back when we ask
	doHTTP("DELETE", identURL(port, containerID))

	cidr1c := HTTPPost(t, allocURL(port, testCIDR1, container3))
	wt.AssertEqualString(t, cidr1c, testAddr1, "address")
	cidr2c := HTTPPost(t, allocURL(port, testCIDR2, container3))
	wt.AssertEqualString(t, cidr2c, testAddr2, "address")

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}

func TestBadHttp(t *testing.T) {
	var (
		containerID = "deadbeef"
		testCIDR1   = "10.0.0.0/8"
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", testCIDR1, 1)
	defer alloc.Stop()
	port := listenHTTP(alloc, subnet)

	alloc.claimRingForTesting()
	cidr1 := HTTPPost(t, allocURL(port, testCIDR1, containerID))
	parts := strings.Split(cidr1, "/")
	testAddr1 := parts[0]
	// Verb that's not handled
	resp, err := doHTTP("HEAD", fmt.Sprintf("http://localhost:%d/ip/%s/%s", port, containerID, testAddr1))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusNotFound, "http response")
	// Mis-spelled URL
	resp, err = doHTTP("POST", fmt.Sprintf("http://localhost:%d/xip/%s/", port, containerID))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusNotFound, "http response")
	// Malformed URL
	resp, err = doHTTP("POST", fmt.Sprintf("http://localhost:%d/ip/%s/foo/bar/baz", port, containerID))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusNotFound, "http response")
}

func TestHTTPCancel(t *testing.T) {
	wt.RunWithTimeout(t, 2*time.Second, func() {
		impTestHTTPCancel(t)
	})
}

func impTestHTTPCancel(t *testing.T) {
	common.InitDefaultLogging(true)
	var (
		containerID = "deadbeef"
		testCIDR1   = "10.0.3.0/29"
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", testCIDR1, 1)
	defer alloc.Stop()
	alloc.claimRingForTesting()
	port := listenHTTP(alloc, subnet)

	// Stop the alloc so nothing actually works
	unpause := alloc.pause()

	// Ask the http server for a new address
	done := make(chan *http.Response)
	req, _ := http.NewRequest("POST", allocURL(port, testCIDR1, containerID), nil)
	go func() {
		res, _ := http.DefaultClient.Do(req)
		done <- res
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("Cancelling allocate")
	http.DefaultTransport.(*http.Transport).CancelRequest(req)
	unpause()
	res := <-done
	if res != nil {
		wt.Fatalf(t, "Error: Allocate returned non-nil")
	}
}
