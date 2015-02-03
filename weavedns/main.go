package main

import (
	"flag"
	"fmt"
	. "github.com/zettio/weave/common"
	weavedns "github.com/zettio/weave/nameserver"
	weavenet "github.com/zettio/weave/net"
	"io"
	"net"
	"os"
)

var version = "(unreleased version)"

func main() {
	var (
		justVersion bool
		ifaceName   string
		apiPath     string
		dnsPort     int
		httpPort    int
		wait        int
		watch       bool
		debug       bool
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "Path to Docker API socket")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up")
	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests")
	flag.BoolVar(&watch, "watch", true, "watch the docker socket for container events")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	flag.Parse()

	if justVersion {
		io.WriteString(os.Stdout, fmt.Sprintf("weave DNS %s\n", version))
		os.Exit(0)
	}

	InitDefaultLogging(debug)

	var zone = new(weavedns.ZoneDb)

	if watch {
		err := weavedns.StartUpdater(apiPath, zone)
		if err != nil {
			Error.Fatal("Unable to start watcher", err)
		}
	}

	var iface *net.Interface
	if ifaceName != "" {
		var err error
		Info.Println("Waiting for interface", ifaceName, "to come up")
		iface, err = weavenet.EnsureInterface(ifaceName, wait)
		if err != nil {
			Error.Fatal(err)
		} else {
			Info.Println("Interface", ifaceName, "is up")
		}
	}

	go weavedns.ListenHttp(weavedns.LOCAL_DOMAIN, zone, httpPort)
	err := weavedns.StartServer(zone, iface, dnsPort, wait)
	if err != nil {
		Error.Fatal("Failed to start server", err)
	}
}
