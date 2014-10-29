package main

import (
	"flag"
	weavedns "github.com/zettio/weave/nameserver"
	"io/ioutil"
	"net"
	"os"
)

func main() {
	var (
		ifaceName string
		apiPath   string
		dnsPort   int
		httpPort  int
		wait      int
		watch     bool
		debug     bool
	)

	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "Path to Docker API socket")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up")
	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests")
	flag.BoolVar(&watch, "watch", true, "watch the docker socket for container events")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	flag.Parse()

	debugOut := ioutil.Discard
	if debug {
		debugOut = os.Stderr
	}
	weavedns.InitLogging(debugOut, os.Stdout, os.Stdout, os.Stderr)

	var zone = new(weavedns.ZoneDb)

	if watch {
		err := weavedns.StartUpdater(apiPath, zone)
		if err != nil {
			weavedns.Error.Fatal("Unable to start watcher", err)
		}
	}

	var iface *net.Interface = nil
	if ifaceName != "" {
		var err error
		iface, err = weavedns.EnsureInterface(ifaceName, wait)
		if err != nil {
			weavedns.Error.Fatal(err)
		}
	}

	err := weavedns.StartServer(zone, iface, dnsPort, httpPort, wait)
	if err != nil {
		weavedns.Error.Fatal("Failed to start server", err)
	}
}
