package main

import (
	"flag"
	"fmt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/updater"
	weavedns "github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

var version = "(unreleased version)"

func main() {
	var (
		justVersion bool
		ifaceName   string
		apiPath     string
		domain      string
		dnsPort     int
		httpPort    int
		wait        int
		timeout     int
		udpbuf      int
		cacheLen    int
		watch       bool
		debug       bool
		err         error
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "path to Docker API socket")
	flag.StringVar(&domain, "domain", weavedns.DefaultLocalDomain, "local domain (ie, 'weave.local.')")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up")
	flag.IntVar(&dnsPort, "dnsport", weavedns.DefaultServerPort, "port to listen to DNS requests")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests")
	flag.IntVar(&timeout, "timeout", weavedns.DefaultTimeout, "timeout for resolutions")
	flag.IntVar(&udpbuf, "udpbuf", weavedns.DefaultUDPBuflen, "UDP buffer length")
	flag.IntVar(&cacheLen, "cache", weavedns.DefaultCacheLen, "cache length")
	flag.BoolVar(&watch, "watch", true, "watch the docker socket for container events")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	flag.Parse()

	if justVersion {
		fmt.Printf("weave DNS %s\n", version)
		os.Exit(0)
	}

	InitDefaultLogging(debug)
	Info.Printf("WeaveDNS version %s\n", version) // first thing in log: the version

	var zone = weavedns.NewZoneDb(domain)

	if watch {
		err := updater.Start(apiPath, zone)
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

	srvConfig := weavedns.DNSServerConfig{
		Port:        dnsPort,
		CacheLen:    cacheLen,
		LocalDomain: domain,
		Timeout:     timeout,
		UDPBufLen:   udpbuf,
	}

	srv, err := weavedns.NewDNSServer(srvConfig, zone, iface)
	if err != nil {
		Error.Fatal("Failed to initialize the WeaveDNS server", err)
	}
	Info.Println("Upstream", srv.Upstream)

	Debug.Printf("Starting the signals handler")
	go handleSignals(srv)

	go weavedns.ListenHTTP(version, srv, domain, zone, httpPort)
	err = srv.Start()
	if err != nil {
		Error.Fatal("Failed to start the WeaveDNS server", err)
	}
}

func handleSignals(s *weavedns.DNSServer) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT)
	buf := make([]byte, 1<<20)
	for {
		sig := <-sigs
		switch sig {
		case syscall.SIGINT:
			Info.Printf("=== received SIGINT ===\n*** exiting\n")
			s.Stop()
			os.Exit(0)
		case syscall.SIGQUIT:
			stacklen := runtime.Stack(buf, true)
			Info.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf[:stacklen])
		}
	}
}
