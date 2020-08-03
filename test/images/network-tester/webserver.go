// Adapted from k8s.io/kubernetes/test/images/network-tester/webserver.go

// A tiny web server for checking networking connectivity.
//
// Will dial out to, and expect to hear from, every other server
// Options to look for other servers:
// 1. via command-line args, following options
//
// Will serve a webserver on given -port.
//
// Visit /read to see the current state, or /quit to shut down.
//
// Visit /status to see pass/running/fail determination. (literally, it will
// return one of those words.)
//
// Visit /client_ip to see the source IP addr of the request.
//
// /write is used by other network test servers to register connectivity.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	port          = flag.Int("port", 8080, "Port number to serve at.")
	peerCount     = flag.Int("peers", 8, "Must find at least this many peers for the test to pass.")
	dnsName       = flag.String("dns-name", "", "DNS name to find other network test pods via.")
	delayShutdown = flag.Int("delay-shutdown", 0, "Number of seconds to delay shutdown when receiving SIGTERM.")
	waitIface     = flag.String("iface", "", "name of interface to wait for")
)

// State tracks the internal state of our little http server.
// It's returned verbatim over the /read endpoint.
type State struct {
	// Hostname is set once and never changed-- it's always safe to read.
	Hostname string

	// The below fields require that lock is held before reading or writing.
	Sent                 map[string]int
	Received             map[string]int
	StillContactingPeers bool

	lock sync.Mutex
}

func (s *State) doneContactingPeers() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.StillContactingPeers = false
}

// serveStatus returns "pass", "running", or "fail".
func (s *State) serveStatus(w http.ResponseWriter, r *http.Request) {
	s.Logf("Checking status for %q with %d sent and %d received and %d peers", *dnsName, len(s.Sent), len(s.Received), *peerCount)
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.Sent) >= *peerCount && len(s.Received) >= *peerCount {
		fmt.Fprintf(w, "pass")
		return
	}
	if s.StillContactingPeers {
		fmt.Fprintf(w, "running")
		return
	}
	// Logf can't be called while holding the lock, so defer using a goroutine
	go s.Logf("Declaring failure for %q with %d sent and %d received and %d peers", *dnsName, len(s.Sent), len(s.Received), *peerCount)
	fmt.Fprintf(w, "fail")
}

// serveClientIP returns the client source IP addr.
func (s *State) serveClientIP(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		go s.Logf("Warning: unable to parse remote addr: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	fmt.Fprintln(w, host)
}

// serveRead writes our json encoded state
func (s *State) serveRead(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()
	w.WriteHeader(http.StatusOK)
	b, err := json.MarshalIndent(s, "", "\t")
	s.appendErr(err)
	_, err = w.Write(b)
	s.appendErr(err)
}

// WritePost is the format that (json encoded) requests to the /write handler should take.
type WritePost struct {
	Source string
	Dest   string
}

// WriteResp is returned by /write
type WriteResp struct {
	Hostname string
}

// serveWrite records the contact in our state.
func (s *State) serveWrite(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	s.lock.Lock()
	defer s.lock.Unlock()
	w.WriteHeader(http.StatusOK)
	var wp WritePost
	s.appendErr(json.NewDecoder(r.Body).Decode(&wp))
	if wp.Source == "" {
		s.appendErr(fmt.Errorf("%v: Got request with no source", s.Hostname))
	} else {
		if s.Received == nil {
			s.Received = map[string]int{}
		}
		s.Received[wp.Source]++
	}
	s.appendErr(json.NewEncoder(w).Encode(&WriteResp{Hostname: s.Hostname}))
}

// appendErr logs an error, if err is not nil.
func (s *State) appendErr(err error) {
	if err != nil {
		log.Printf("error: %s", err)
	}
}

// Logf writes to the log.
func (s *State) Logf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// s must not be locked
func (s *State) appendSuccessfulSend(toHostname string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.Sent == nil {
		s.Sent = map[string]int{}
	}
	s.Sent[toHostname]++
}

var (
	// Our one and only state object
	state State
)

func main() {
	flag.Parse()

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Error getting hostname: %v", err)
	}

	if *waitIface != "" {
		_, err = EnsureInterface(*waitIface, 30)
		if err != nil {
			log.Fatalf("Error getting interface: %v", err)
		}
	}

	if *delayShutdown > 0 {
		termCh := make(chan os.Signal)
		signal.Notify(termCh, syscall.SIGTERM)
		go func() {
			<-termCh
			log.Printf("Sleeping %d seconds before exit ...", *delayShutdown)
			time.Sleep(time.Duration(*delayShutdown) * time.Second)
			os.Exit(0)
		}()
	}

	state := State{
		Hostname:             hostname,
		StillContactingPeers: true,
	}

	go contactOthers(&state)

	http.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		os.Exit(0)
	})

	http.HandleFunc("/read", state.serveRead)
	http.HandleFunc("/write", state.serveWrite)
	http.HandleFunc("/status", state.serveStatus)
	http.HandleFunc("/client_ip", state.serveClientIP)

	go log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *port), nil))

	select {}
}

// Find all siblings and post to their /write handler.
func contactOthers(state *State) {
	sleepTime := 1 * time.Second
	// In large cluster getting all endpoints is pretty expensive.
	// Thus, we will limit ourselves to send on average at most 10 such
	// requests per second
	if sleepTime < time.Duration(*peerCount/10)*time.Second {
		sleepTime = time.Duration(*peerCount/10) * time.Second
	}
	timeout := 5 * time.Minute
	// Similarly we need to bump timeout so that it is reasonable in large
	// clusters.
	if timeout < time.Duration(*peerCount)*time.Second {
		timeout = time.Duration(*peerCount) * time.Second
	}
	defer state.doneContactingPeers()

	for start := time.Now(); time.Since(start) < timeout; time.Sleep(sleepTime) {
		eps := getWebserverEndpoints()
		if len(eps) >= *peerCount {
			break
		}
		state.Logf("%q has %v endpoints (%v), which is less than %v as expected. Waiting for all endpoints to come up.", *dnsName, len(eps), eps, *peerCount)
	}

	// Do this repeatedly, in case there's some propagation delay with getting
	// newly started siblings into the endpoints list.
	for i := 0; i < 15; i++ {
		eps := getWebserverEndpoints()
		for ep := range eps {
			state.Logf("Attempting to contact %s", ep)
			contactSingle(ep, state)
		}
		time.Sleep(sleepTime)
	}
}

//getWebserverEndpoints returns the webserver endpoints as a set of String, each in the format like "http://{ip}:{port}"
func getWebserverEndpoints() map[string]struct{} {
	eps := map[string]struct{}{}
	if *dnsName != "" {
		addrs, err := net.LookupHost(*dnsName)
		if err != nil {
			state.Logf("Error from DNS lookup of %q: %s", *dnsName, err)
			return nil
		}
		for _, a := range addrs {
			eps[fmt.Sprintf("http://%s:%d", a, *port)] = struct{}{}
		}
	}
	for _, a := range flag.Args() {
		eps[fmt.Sprintf("http://%s:%d", a, *port)] = struct{}{}
	}
	return eps
}

// contactSingle dials the address 'e' and tries to POST to its /write address.
func contactSingle(e string, state *State) {
	body, err := json.Marshal(&WritePost{
		Dest:   e,
		Source: state.Hostname,
	})
	if err != nil {
		log.Fatalf("json marshal error: %v", err)
	}
	resp, err := http.Post(e+"/write", "application/json", bytes.NewReader(body))
	if err != nil {
		state.Logf("Warning: unable to contact the endpoint %q: %v", e, err)
		return
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		state.Logf("Warning: unable to read response from '%v': '%v'", e, err)
		return
	}
	var wr WriteResp
	err = json.Unmarshal(body, &wr)
	if err != nil {
		state.Logf("Warning: unable to unmarshal response (%v) from '%v': '%v'", string(body), e, err)
		return
	}
	state.appendSuccessfulSend(wr.Hostname)
}

func EnsureInterface(ifaceName string, wait int) (iface *net.Interface, err error) {
	if iface, err = findInterface(ifaceName); err == nil || wait == 0 {
		return
	}
	for ; err != nil && wait > 0; wait-- {
		time.Sleep(1 * time.Second)
		iface, err = findInterface(ifaceName)
	}
	return
}

func findInterface(ifaceName string) (iface *net.Interface, err error) {
	if iface, err = net.InterfaceByName(ifaceName); err != nil {
		return iface, fmt.Errorf("Unable to find interface %s", ifaceName)
	}
	if 0 == (net.FlagUp & iface.Flags) {
		return iface, fmt.Errorf("Interface %s is not up", ifaceName)
	}
	return
}
