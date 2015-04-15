package ipam

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/weaveworks/weave/common"
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

func doHTTP(method string, url string) (resp *http.Response, err error) {
	req, _ := http.NewRequest(method, url, nil)
	return http.DefaultClient.Do(req)
}

func listenHTTP(port int, alloc *Allocator) {
	router := mux.NewRouter()
	router.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln(alloc))
	})
	alloc.HandleHTTP(router)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}
	if err := srv.ListenAndServe(); err != nil {
		common.Error.Fatal("Unable to create http listener: ", err)
	}
}

func allocURL(port int, containerID string) string {
	return fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID)
}

func TestHttp(t *testing.T) {
	var (
		containerID = "deadbeef"
		container2  = "baddf00d"
		container3  = "b01df00d"
		testAddr1   = "10.0.3.9"
		netSuffix   = "/29"
		testCIDR1   = "10.0.3.8" + netSuffix
	)

	alloc := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", testCIDR1, 1)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go listenHTTP(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	alloc.claimRingForTesting()
	// Ask the http server for a new address
	cidr1 := HTTPPost(t, allocURL(port, containerID))
	wt.AssertEqualString(t, cidr1, testAddr1+netSuffix, "address")

	// Ask the http server for another address and check it's different
	cidr2 := HTTPPost(t, allocURL(port, container2))
	wt.AssertNotEqualString(t, cidr2, testAddr1+netSuffix, "address")

	// Ask for the first container again and we should get the same address again
	cidr1a := HTTPPost(t, allocURL(port, containerID))
	wt.AssertEqualString(t, cidr1a, testAddr1+netSuffix, "address")

	// Now free the first one, and we should get it back when we ask
	doHTTP("DELETE", allocURL(port, containerID))
	cidr3 := HTTPPost(t, allocURL(port, container3))
	wt.AssertEqualString(t, cidr3, testAddr1+netSuffix, "address")

	// Would like to shut down the http server at the end of this test
	// but it's complicated.
	// See https://groups.google.com/forum/#!topic/golang-nuts/vLHWa5sHnCE
}

func TestBadHttp(t *testing.T) {
	var (
		containerID = "deadbeef"
		testCIDR1   = "10.0.0.0/8"
	)

	alloc := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", testCIDR1, 1)
	defer alloc.Stop()
	port := rand.Intn(10000) + 32768
	fmt.Println("BadHttp test on port", port)
	go listenHTTP(port, alloc)

	alloc.claimRingForTesting()
	cidr1 := HTTPPost(t, allocURL(port, containerID))
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
		testCIDR1   = "10.0.3.5/29"
	)

	alloc := makeAllocatorWithMockGossip(t, "08:00:27:01:c3:9a", testCIDR1, 1)
	defer alloc.Stop()
	alloc.claimRingForTesting()
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go listenHTTP(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	// Stop the alloc so nothing actually works
	unpause := alloc.pause()

	// Ask the http server for a new address
	done := make(chan *http.Response)
	req, _ := http.NewRequest("POST", allocURL(port, containerID), nil)
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
