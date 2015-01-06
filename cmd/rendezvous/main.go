package main

import (
	"flag"
	"fmt"
	. "github.com/zettio/weave/common"
	. "github.com/zettio/weave/rendezvous"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

var version = "(unreleased version)"

const (
	defaultHttpPort = 6786
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	var (
		justVersion bool
		domains     []string
		debug       bool
		ifaces      IfaceNamesList
		httpPort    int
	)

	ifaces = NewIfaceNamesList()

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.Var(&ifaces, "iface", "comma-separated list of interfaces to announce in rendezvous services (default:guess)")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
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

	manager := NewRendezvousManager(endpoints)
	for _, domain := range domains {
		manager.Connect(domain)
	}

	go handleHttp(manager, httpPort)
	handleSignals(manager)
}

// HTTP servers for REST requests
func handleHttp(rm *RendezvousManager, httpPort int) {
	// we currently support only one command: "connect <domain>"
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		Debug.Printf("Connect request from %s", r.RemoteAddr)
		domain := r.FormValue("domain")
		if err := rm.Connect(domain); err == nil {
			http.Error(w, fmt.Sprint("error when connecting to domain %s: %s", domain, err), http.StatusBadRequest)
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
