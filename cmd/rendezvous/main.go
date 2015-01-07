package main

import (
	"flag"
	"fmt"
	. "github.com/zettio/weave/common"
	. "github.com/zettio/weave/rendezvous"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

var version = "(unreleased version)"

const (
	defaultHttpPort = 6786                    // default HTTP API port
	defaultWeaveUrl = "http://127.0.0.1:6784" // default Weave's HTTP API URL
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	var (
		justVersion bool
		domains     []string
		debug       bool
		ifaces      IfaceNamesList
		nifaces     IfaceNamesList
		weaveUrl    string
		httpPort    int
	)

	ifaces = NewIfaceNamesList()
	nifaces = NewIfaceNamesList()

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.Var(&ifaces, "iface", "comma-separated list of interfaces to announce in rendezvous services (default:guess)")
	flag.Var(&nifaces, "niface", "comma-separated list of interfaces to ignore when guessing external interfaces")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	flag.StringVar(&weaveUrl, "weave", defaultWeaveUrl, "weave API URL")
	flag.IntVar(&httpPort, "port", defaultHttpPort, "default HTTP port")
	flag.Parse()
	domains = flag.Args()

	if justVersion {
		io.WriteString(os.Stdout, fmt.Sprintf("weave rendezvous service %s\n", version))
		os.Exit(0)
	}

	InitDefaultLogging(debug)

	endpoints, err := EndpointsListFromIfaceNamesList(ifaces)
	if err != nil {
		log.Fatalf("Could not get rendezvous announced enpoints: %s", err)
	}

	parsedWeaveUrl, err := url.Parse(weaveUrl)
	if err != nil {
		log.Fatalf("Could not parse weave URL \"%s\": %s", weaveUrl, err)
	}

	manager := NewRendezvousManager(endpoints, parsedWeaveUrl)
	for _, domain := range domains {
		manager.Connect(domain)
	}

	go handleHttp(manager, httpPort)
	handleSignals(manager)
}

// HTTP servers for REST requests
func handleHttp(rm *RendezvousManager, httpPort int) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln("weave rendezvous service", version))
		io.WriteString(w, rm.Status())
	})

	http.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		Debug.Printf("JOIN request from %s", r.RemoteAddr)
		domain := r.FormValue("domain")
		if err := rm.Connect(domain); err != nil {
			http.Error(w, fmt.Sprintf("weaverendezvous: error when connecting to domain \"%s\": %s",
				domain, err), http.StatusBadRequest)
		}
	})
	http.HandleFunc("/leave", func(w http.ResponseWriter, r *http.Request) {
		Debug.Printf("LEAVE request from %s", r.RemoteAddr)
		domain := r.FormValue("domain")
		if err := rm.Leave(domain); err != nil {
			http.Error(w, fmt.Sprintf("weaverendezvous: error when leaving domain \"%s\": %s",
				domain, err), http.StatusBadRequest)
		}
	})

	address := fmt.Sprintf(":%d", httpPort)
	Debug.Printf("Starting HTTP service at %s...", address)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		Error.Fatalf("Unable to create HTTP listener: %s", err)
	}
}

// Signals handler
func handleSignals(rm *RendezvousManager) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT)
	buf := make([]byte, 1<<20)
	for {
		sig := <-sigs
		switch sig {
		case syscall.SIGQUIT:
			runtime.Stack(buf, true)
			Info.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf)
			rm.Stop()
		}
	}
}
