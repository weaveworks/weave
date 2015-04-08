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

	"github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
)

func HTTPGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusOK, "http response")
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	return string(body)
}

func getHTTP(method string, url string) (resp *http.Response, err error) {
	req, _ := http.NewRequest(method, url, nil)
	return http.DefaultClient.Do(req)
}

func listenHTTP(port int, alloc *Allocator) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln(alloc))
	})
	alloc.HandleHTTP(mux)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != nil {
		common.Error.Fatal("Unable to create http listener: ", err)
	}
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

	alloc := testAllocator(t, "08:00:27:01:c3:9a", testCIDR1)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go listenHTTP(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	ExpectBroadcastMessage(alloc, nil) // on leader election, broadcasts its state
	// Ask the http server for a new address
	cidr1 := HTTPGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	wt.AssertEqualString(t, cidr1, testAddr1+netSuffix, "address")

	// Ask the http server for another address and check it's different
	cidr2 := HTTPGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container2))
	wt.AssertNotEqualString(t, cidr2, testAddr1+netSuffix, "address")

	// Ask for the first container again and we should get the same address again
	cidr1a := HTTPGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	wt.AssertEqualString(t, cidr1a, testAddr1+netSuffix, "address")

	// Now free the first one, and we should get it back when we ask
	getHTTP("DELETE", fmt.Sprintf("http://localhost:%d/ip/%s/%s", port, containerID, testAddr1))
	cidr3 := HTTPGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, container3))
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

	alloc := testAllocator(t, "08:00:27:01:c3:9a", testCIDR1)
	defer alloc.Stop()
	port := rand.Intn(10000) + 32768
	fmt.Println("BadHttp test on port", port)
	go listenHTTP(port, alloc)

	ExpectBroadcastMessage(alloc, nil) // on leader election, broadcasts its state
	cidr1 := HTTPGet(t, fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID))
	parts := strings.Split(cidr1, "/")
	testAddr1 := parts[0]
	// Verb that's not handled
	resp, err := getHTTP("POST", fmt.Sprintf("http://localhost:%d/ip/%s/%s", port, containerID, testAddr1))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusBadRequest, "http response")
	// Mis-spelled URL
	resp, err = getHTTP("GET", fmt.Sprintf("http://localhost:%d/xip/%s/", port, containerID))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusNotFound, "http response")
	// Malformed URL
	resp, err = getHTTP("GET", fmt.Sprintf("http://localhost:%d/ip/%s/foo/bar/baz", port, containerID))
	wt.AssertNoErr(t, err)
	wt.AssertStatus(t, resp.StatusCode, http.StatusBadRequest, "http response")
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

	alloc := testAllocator(t, "08:00:27:01:c3:9a", testCIDR1)
	port := rand.Intn(10000) + 32768
	fmt.Println("Http test on port", port)
	go listenHTTP(port, alloc)

	time.Sleep(100 * time.Millisecond) // Allow for http server to get going

	// Stop the alloc so nothing actually works
	alloc.Stop()

	// Ask the http server for a new address
	done := make(chan *http.Response)
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/ip/%s", port, containerID), nil)
	go func() {
		res, _ := http.DefaultClient.Do(req)
		done <- res
	}()

	time.Sleep(1000 * time.Millisecond)
	fmt.Println("Cancelling get")
	http.DefaultTransport.(*http.Transport).CancelRequest(req)
	res := <-done
	if res != nil {
		wt.Fatalf(t, "Error: Get returned non-nil")
	}
}
