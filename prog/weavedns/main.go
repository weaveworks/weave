package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	weavedns "github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
)

var version = "(unreleased version)"

func main() {
	var (
		justVersion     bool
		dockerCli       *docker.Client
		ifaceName       string
		apiPath         string
		domain          string
		dnsPort         int
		httpIfaceName   string
		httpPort        int
		wait            int
		ttl             int
		negTTL          int
		timeout         int
		udpbuf          int
		fallback        string
		refreshInterval int
		relevantTime    int
		maxAnswers      int
		cacheLen        int
		cacheDisabled   bool
		watch           bool
		debug           bool
		err             error
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "path to Docker API socket")
	flag.StringVar(&domain, "domain", weavedns.DefaultLocalDomain, "local domain (ie, 'weave.local.')")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up")
	flag.IntVar(&dnsPort, "dnsport", weavedns.DefaultServerPort, "port to listen to DNS requests")
	flag.StringVar(&httpIfaceName, "httpiface", "", "interface on which to listen for HTTP requests (defaults to empty string which listens on all interfaces)")
	flag.IntVar(&httpPort, "httpport", weavedns.DefaultHTTPPort, "port to listen to HTTP requests")
	flag.IntVar(&cacheLen, "cache", weavedns.DefaultCacheLen, "cache length")
	flag.IntVar(&ttl, "ttl", weavedns.DefaultLocalTTL, "TTL (in secs) for responses for local names")
	flag.BoolVar(&watch, "watch", true, "watch the docker socket for container events")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	// advanced options
	flag.IntVar(&negTTL, "neg-ttl", 0, "negative TTL (in secs) for unanswered queries for local names (0=same value as -ttl")
	flag.IntVar(&refreshInterval, "refresh", weavedns.DefaultRefreshInterval, "refresh interval (in secs) for local names (0=disable)")
	flag.IntVar(&maxAnswers, "max-answers", weavedns.DefaultMaxAnswers, "maximum number of answers returned to clients (0=unlimited)")
	flag.IntVar(&relevantTime, "relevant", weavedns.DefaultRelevantTime, "life time for info in the absence of queries (in secs)")
	flag.IntVar(&udpbuf, "udpbuf", weavedns.DefaultUDPBuflen, "UDP buffer length")
	flag.IntVar(&timeout, "timeout", weavedns.DefaultTimeout, "timeout for resolutions (in millisecs)")
	flag.BoolVar(&cacheDisabled, "no-cache", false, "disable the cache")
	flag.StringVar(&fallback, "fallback", "", "force a fallback server (ie, '8.8.8.8:53') (instead of /etc/resolv.conf values)")
	flag.Parse()

	if justVersion {
		fmt.Printf("weave DNS %s\n", version)
		os.Exit(0)
	}

	InitDefaultLogging(debug)
	Info.Printf("[main] WeaveDNS version %s\n", version) // first thing in log: the version

	var iface *net.Interface
	if ifaceName != "" {
		var err error
		Info.Println("[main] Waiting for mDNS interface", ifaceName, "to come up")
		iface, err = weavenet.EnsureInterface(ifaceName, wait)
		if err != nil {
			Log.Fatal(err)
		} else {
			Info.Println("[main] Interface", ifaceName, "is up")
		}
	}

	var httpIP string
	if httpIfaceName == "" {
		httpIP = "0.0.0.0"
	} else {
		Info.Println("[main] Waiting for HTTP interface", httpIfaceName, "to come up")
		httpIface, err := weavenet.EnsureInterface(httpIfaceName, wait)
		if err != nil {
			Log.Fatal(err)
		}
		Info.Println("[main] Interface", httpIfaceName, "is up")

		addrs, err := httpIface.Addrs()
		if err != nil {
			Log.Fatal(err)
		}

		if len(addrs) == 0 {
			Log.Fatal("[main] No addresses on HTTP interface")
		}

		ip, _, err := net.ParseCIDR(addrs[0].String())
		if err != nil {
			Log.Fatal(err)
		}

		httpIP = ip.String()

	}
	httpAddr := net.JoinHostPort(httpIP, strconv.Itoa(httpPort))

	zoneConfig := weavedns.ZoneConfig{
		Domain:          domain,
		Iface:           iface,
		LocalTTL:        ttl,
		RefreshInterval: refreshInterval,
		RelevantTime:    relevantTime,
	}
	zone, err := weavedns.NewZoneDb(zoneConfig)
	if err != nil {
		Log.Fatal("[main] Unable to initialize the Zone database", err)
	}
	if err := zone.Start(); err != nil {
		Log.Fatal("[main] Unable to start the Zone database", err)
	}
	defer zone.Stop()

	dockerCli, err = docker.NewClient(apiPath)
	if err != nil {
		Log.Fatal("[main] Unable to start docker client: ", err)
	}

	if watch {
		err := dockerCli.AddObserver(zone)
		if err != nil {
			Log.Fatal("[main] Unable to start watcher", err)
		}
	}

	srvConfig := weavedns.DNSServerConfig{
		Zone:             zone,
		Port:             dnsPort,
		CacheLen:         cacheLen,
		LocalTTL:         ttl,
		CacheNegLocalTTL: negTTL,
		MaxAnswers:       maxAnswers,
		Timeout:          timeout,
		UDPBufLen:        udpbuf,
		CacheDisabled:    cacheDisabled,
	}

	if len(fallback) > 0 {
		fallbackHost, fallbackPort, err := net.SplitHostPort(fallback)
		if err != nil {
			Log.Fatal("[main] Could not parse fallback host and port", err)
		}
		srvConfig.UpstreamCfg = &dns.ClientConfig{Servers: []string{fallbackHost}, Port: fallbackPort}
		Log.Debugf("[main] DNS fallback at %s:%s", fallbackHost, fallbackPort)
	}

	srv, err := weavedns.NewDNSServer(srvConfig)
	if err != nil {
		Log.Fatal("[main] Failed to initialize the WeaveDNS server", err)
	}

	httpListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		Log.Fatal("[main] Unable to create http listener: ", err)
	}
	Info.Println("[main] HTTP API listening on", httpAddr)

	go SignalHandlerLoop(srv)
	go weavedns.ServeHTTP(httpListener, version, srv, dockerCli)

	err = srv.Start()
	if err != nil {
		Log.Fatal("[main] Failed to start the WeaveDNS server: ", err)
	}
	srv.ActivateAndServe()
}
