package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	weavedns "github.com/zettio/weave/nameserver"
	weavenet "github.com/zettio/weave/net"
	"io"
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
		localDomain string
		fallback    string
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
	flag.StringVar(&localDomain, "localDomain", weavedns.DEFAULT_LOCAL_DOMAIN, "local domain (ie, 'weave.local.')")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up")
	flag.IntVar(&dnsPort, "dnsport", weavedns.DEFAULT_SERVER_PORT, "port to listen to DNS requests")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests")
	flag.StringVar(&fallback, "fallback", "", "force a fallback (ie, '8.8.8.8:53') instead of /etc/resolv.conf values")
	flag.IntVar(&timeout, "timeout", weavedns.DEFAULT_TIMEOUT, "timeout for resolutions")
	flag.IntVar(&udpbuf, "udpbuf", weavedns.DEFAULT_UDP_BUFLEN, "UDP buffer length")
	flag.IntVar(&cacheLen, "cache", weavedns.DEFAULT_CACHE_LEN, "cache length")
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

	srvConfig := weavedns.DNSServerConfig{
		Port:                dnsPort,
		CacheLen:            cacheLen,
		LocalDomain:         localDomain,
		Timeout:             timeout,
		UdpBufLen:           udpbuf,
	}

	if len(fallback) > 0 {
		fallbackHost, fallbackPort, err := net.SplitHostPort(fallback)
		if err != nil {
			Error.Fatal("Fould not parse fallback host and port", err)
		}
		srvConfig.UpstreamCfg = &dns.ClientConfig{Servers: []string{fallbackHost}, Port: fallbackPort}
		Debug.Printf("DNS fallback at %s:%s", fallbackHost, fallbackPort)
	}

	go weavedns.ListenHttp(localDomain, zone, httpPort)
	srv, err := weavedns.NewDNSServer(srvConfig, zone, iface)
	if err != nil {
		Error.Fatal("Failed to initialize the WeaveDNS server", err)
	}

	Debug.Printf("Starting the signals handler")
	go handleSignals(srv)

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
