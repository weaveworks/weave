package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	. "github.com/weaveworks/weave/common"
)

var version = "(unreleased version)"

const (
	defaultHeartbeat = "30s"
	defaultTTL       = "60s"
)

func main() {
	var (
		justVersion   bool
		routerURL     string
		httpIfaceName string
		httpPort      int
		localAddr     string
		wait          int
		hb            string
		ttl           string
		logLevel      string
		verbose       bool
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&routerURL, "weave", defaultWeaveURL, "weave API URL")
	flag.StringVar(&localAddr, "local", "", "local address announced to other peers")
	flag.IntVar(&wait, "wait", -1, "number of seconds to wait for interfaces to come up (0=don't wait, -1=wait forever)")
	flag.StringVar(&hb, "hb", defaultHeartbeat, "heartbeat (with units)")
	flag.StringVar(&ttl, "ttl", defaultTTL, "TTL (with units)")
	flag.StringVar(&httpIfaceName, "http-iface", "", "interface on which to listen for HTTP requests (defaults to empty string which listens on all interfaces)")
	flag.IntVar(&httpPort, "http-port", defaultHTTPPort, "port for the Discovery HTTP API")
	flag.StringVar(&logLevel, "log-level", "info", "logging level (debug, info, warning, error)")
	flag.BoolVar(&verbose, "v", false, "enable verbose mode (debug logging level)")

	flag.Parse()

	if justVersion {
		fmt.Printf("weave Discovery %s\n", version)
		os.Exit(0)
	}
	if verbose {
		logLevel = "debug"
	}
	hbDur, err := time.ParseDuration(hb)
	if err != nil {
		Log.Fatalf("[main] Could not parse heartbeat '%s': %s", hb, err)
	}
	ttlDur, err := time.ParseDuration(ttl)
	if err != nil {
		Log.Fatalf("[main] Could not parse TTL '%s': %s", ttl, err)
	}

	SetLogLevel(logLevel)
	Log.Infof("[main] WeaveDiscovery version %s", version) // first thing in log: the version

	if len(localAddr) == 0 {
		Log.Fatalf("[main] Local address not provided")
	}
	Log.Infof("[main] Will announce local address '%s'", localAddr)

	weaveCli, err := NewWeaveClient(routerURL)
	if err != nil {
		Log.Fatalf("[main] Could not initialize the weave router client: %s", err)
	}
	manager := NewDiscoveryManager(localAddr, weaveCli)
	httpServer := NewDiscoveryHTTP(DiscoveryHTTPConfig{
		Manager:   manager,
		Iface:     httpIfaceName,
		Port:      httpPort,
		Wait:      wait,
		Heartbeat: hbDur,
		TTL:       ttlDur,
	})

	manager.Start()
	httpServer.Start()

	// join any additional endpoint provided in command line, using the heartbeat and TTL arguments
	numEps := len(flag.Args())
	if numEps > 0 {
		Log.Debugf("[main] %d endpoints provided in arguments", numEps)
		for _, ep := range flag.Args() {
			if err = manager.Join(ep, hbDur, ttlDur); err != nil {
				Log.Fatalf("[main] Could not join '%s': %s", ep, err)
			}
		}
	} else {
		Log.Debugf("[main] No endpoints provided in arguments")
	}

	SignalHandlerLoop(httpServer, manager)
}
