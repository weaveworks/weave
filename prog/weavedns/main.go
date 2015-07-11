package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/docker/docker/pkg/mflag"
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
		logLevel        string
		err             error
	)

	mflag.BoolVar(&justVersion, []string{"#version", "-version"}, false, "print version and exit")
	mflag.StringVar(&ifaceName, []string{"#iface", "-iface"}, "", "name of interface to use for multicast")
	mflag.StringVar(&apiPath, []string{"#api", "-api"}, "unix:///var/run/docker.sock", "path to Docker API socket")
	mflag.StringVar(&domain, []string{"#domain", "-domain"}, weavedns.DefaultLocalDomain, "local domain (ie, 'weave.local.')")
	mflag.IntVar(&wait, []string{"#wait", "-wait"}, -1, "number of seconds to wait for interfaces to come up (0=don't wait, -1=wait forever)")
	mflag.IntVar(&dnsPort, []string{"#dnsport", "#-dnsport", "-dns-port"}, weavedns.DefaultServerPort, "port to listen to DNS requests")
	mflag.StringVar(&httpIfaceName, []string{"#httpiface", "#-httpiface", "-http-iface"}, "", "interface on which to listen for HTTP requests (empty string means listen on all interfaces)")
	mflag.IntVar(&httpPort, []string{"#httpport", "#-httpport", "-http-port"}, weavedns.DefaultHTTPPort, "port to listen to HTTP requests")
	mflag.IntVar(&cacheLen, []string{"#cache", "-cache"}, weavedns.DefaultCacheLen, "cache length")
	mflag.IntVar(&ttl, []string{"#ttl", "-ttl"}, weavedns.DefaultLocalTTL, "TTL (in secs) for responses for local names")
	mflag.BoolVar(&watch, []string{"#watch", "-watch"}, true, "watch the docker socket for container events")
	mflag.StringVar(&logLevel, []string{"-log-level"}, "info", "logging level (debug, info, warning, error)")
	// advanced options
	mflag.IntVar(&negTTL, []string{"#neg-ttl", "-neg-ttl"}, 0, "negative TTL (in secs) for unanswered queries for local names (0=same value as --ttl")
	mflag.IntVar(&refreshInterval, []string{"#refresh", "-refresh"}, weavedns.DefaultRefreshInterval, "refresh interval (in secs) for local names (0=disable)")
	mflag.IntVar(&maxAnswers, []string{"#max-answers", "#-max-answers", "-dns-max-answers"}, weavedns.DefaultMaxAnswers, "maximum number of answers returned to clients (0=unlimited)")
	mflag.IntVar(&relevantTime, []string{"#relevant", "-relevant"}, weavedns.DefaultRelevantTime, "life time for info in the absence of queries (in secs)")
	mflag.IntVar(&udpbuf, []string{"#udpbuf", "#-udpbuf", "-dns-udpbuf"}, weavedns.DefaultUDPBuflen, "UDP buffer length for DNS")
	mflag.IntVar(&timeout, []string{"#timeout", "#-timeout", "-dns-timeout"}, weavedns.DefaultTimeout, "timeout for resolutions (in millisecs)")
	mflag.BoolVar(&cacheDisabled, []string{"#no-cache", "-no-cache"}, false, "disable the cache")
	mflag.StringVar(&fallback, []string{"#fallback", "#-fallback", "-dns-fallback"}, "", "force a fallback server (ie, '8.8.8.8:53') (instead of /etc/resolv.conf values)")
	mflag.Parse()

	if justVersion {
		fmt.Printf("weave DNS %s\n", version)
		os.Exit(0)
	}

	SetLogLevel(logLevel)
	Log.Infof("[main] WeaveDNS version %s", version) // first thing in log: the version

	var iface *net.Interface
	if ifaceName != "" {
		var err error
		Log.Infoln("[main] Waiting for mDNS interface", ifaceName, "to come up")
		iface, err = weavenet.EnsureInterface(ifaceName, wait)
		if err != nil {
			Log.Fatal(err)
		} else {
			Log.Infoln("[main] Interface", ifaceName, "is up")
		}
	}

	var httpIP string
	if httpIfaceName == "" {
		httpIP = "0.0.0.0"
	} else {
		Log.Infoln("[main] Waiting for HTTP interface", httpIfaceName, "to come up")
		httpIface, err := weavenet.EnsureInterface(httpIfaceName, wait)
		if err != nil {
			Log.Fatal(err)
		}
		Log.Infoln("[main] Interface", httpIfaceName, "is up")

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
	Log.Infoln("[main] HTTP API listening on", httpAddr)

	go SignalHandlerLoop(srv)
	go weavedns.ServeHTTP(httpListener, version, srv, dockerCli)

	err = srv.Start()
	if err != nil {
		Log.Fatal("[main] Failed to start the WeaveDNS server: ", err)
	}
	srv.ActivateAndServe()
}
