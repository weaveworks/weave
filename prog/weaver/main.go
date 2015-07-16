package main

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/davecheney/profile"
	"github.com/docker/docker/pkg/mflag"
	"github.com/gorilla/mux"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
)

var version = "(unreleased version)"

func main() {
	procs := runtime.NumCPU()
	// packet sniffing can block an OS thread, so we need one thread
	// for that plus at least one more.
	if procs < 2 {
		procs = 2
	}
	runtime.GOMAXPROCS(procs)

	var (
		config             weave.Config
		justVersion        bool
		protocolMinVersion int
		ifaceName          string
		routerName         string
		nickName           string
		password           string
		wait               int
		pktdebug           bool
		logLevel           string
		prof               string
		bufSzMB            int
		noDiscovery        bool
		httpAddr           string
		iprangeCIDR        string
		ipsubnetCIDR       string
		peerCount          int
		apiPath            string
		peers              []string
		noDNS              bool
		dnsDomain          string
		dnsListenAddress   string
		dnsTTL             int
		dnsClientTimeout   time.Duration
	)

	mflag.BoolVar(&justVersion, []string{"#version", "-version"}, false, "print version and exit")
	mflag.IntVar(&config.Port, []string{"#port", "-port"}, weave.Port, "router port")
	mflag.IntVar(&protocolMinVersion, []string{"-min-protocol-version"}, weave.ProtocolMinVersion, "minimum weave protocol version")
	mflag.StringVar(&ifaceName, []string{"#iface", "-iface"}, "", "name of interface to capture/inject from (disabled if blank)")
	mflag.StringVar(&routerName, []string{"#name", "-name"}, "", "name of router (defaults to MAC of interface)")
	mflag.StringVar(&nickName, []string{"#nickname", "-nickname"}, "", "nickname of peer (defaults to hostname)")
	mflag.StringVar(&password, []string{"#password", "-password"}, "", "network password")
	mflag.IntVar(&wait, []string{"#wait", "-wait"}, -1, "number of seconds to wait for interface to come up (0=don't wait, -1=wait forever)")
	mflag.StringVar(&logLevel, []string{"-log-level"}, "info", "logging level (debug, info, warning, error)")
	mflag.BoolVar(&pktdebug, []string{"#pktdebug", "#-pktdebug", "-pkt-debug"}, false, "enable per-packet debug logging")
	mflag.StringVar(&prof, []string{"#profile", "-profile"}, "", "enable profiling and write profiles to given path")
	mflag.IntVar(&config.ConnLimit, []string{"#connlimit", "#-connlimit", "-conn-limit"}, 30, "connection limit (0 for unlimited)")
	mflag.BoolVar(&noDiscovery, []string{"#nodiscovery", "#-nodiscovery", "-no-discovery"}, false, "disable peer discovery")
	mflag.IntVar(&bufSzMB, []string{"#bufsz", "-bufsz"}, 8, "capture buffer size in MB")
	mflag.StringVar(&httpAddr, []string{"#httpaddr", "#-httpaddr", "-http-addr"}, fmt.Sprintf(":%d", weave.HTTPPort), "address to bind HTTP interface to (disabled if blank, absolute path indicates unix domain socket)")
	mflag.StringVar(&iprangeCIDR, []string{"#iprange", "#-iprange", "-ipalloc-range"}, "", "IP address range reserved for automatic allocation, in CIDR notation")
	mflag.StringVar(&ipsubnetCIDR, []string{"#ipsubnet", "#-ipsubnet", "-ipalloc-default-subnet"}, "", "subnet to allocate within by default, in CIDR notation")
	mflag.IntVar(&peerCount, []string{"#initpeercount", "#-initpeercount", "-init-peer-count"}, 0, "number of peers in network (for IP address allocation)")
	mflag.StringVar(&apiPath, []string{"#api", "-api"}, "unix:///var/run/docker.sock", "Path to Docker API socket")
	mflag.BoolVar(&noDNS, []string{"-no-dns"}, false, "disable DNS server")
	mflag.StringVar(&dnsDomain, []string{"-dns-domain"}, nameserver.DefaultDomain, "local domain to server requests for")
	mflag.StringVar(&dnsListenAddress, []string{"-dns-listen-address"}, nameserver.DefaultListenAddress, "address to listen on for DNS requests")
	mflag.IntVar(&dnsTTL, []string{"-dns-ttl"}, nameserver.DefaultTTL, "TTL for DNS request from our domain")
	mflag.DurationVar(&dnsClientTimeout, []string{"-dns-fallback-timeout"}, nameserver.DefaultClientTimeout, "timeout for fallback DNS requests")

	mflag.Parse()
	peers = mflag.Args()

	SetLogLevel(logLevel)
	if justVersion {
		fmt.Printf("weave router %s\n", version)
		os.Exit(0)
	}

	Log.Println("Command line options:", options())
	Log.Println("Command line peers:", peers)

	if protocolMinVersion < weave.ProtocolMinVersion || protocolMinVersion > weave.ProtocolMaxVersion {
		Log.Fatalf("--min-protocol-version must be in range [%d,%d]", weave.ProtocolMinVersion, weave.ProtocolMaxVersion)
	}
	config.ProtocolMinVersion = byte(protocolMinVersion)

	var err error

	if ifaceName != "" {
		config.Iface, err = weavenet.EnsureInterface(ifaceName, wait)
		if err != nil {
			Log.Fatal(err)
		}
	}

	if routerName == "" {
		if config.Iface == nil {
			Log.Fatal("Either an interface must be specified with --iface or a name with -name")
		}
		routerName = config.Iface.HardwareAddr.String()
	}
	name, err := weave.PeerNameFromUserInput(routerName)
	if err != nil {
		Log.Fatal(err)
	}

	if nickName == "" {
		nickName, err = os.Hostname()
		if err != nil {
			Log.Fatal(err)
		}
	}

	if password == "" {
		password = os.Getenv("WEAVE_PASSWORD")
	}
	if password == "" {
		Log.Println("Communication between peers is unencrypted.")
	} else {
		config.Password = []byte(password)
		Log.Println("Communication between peers is encrypted.")
	}

	if prof != "" {
		p := *profile.CPUProfile
		p.ProfilePath = prof
		p.NoShutdownHook = true
		defer profile.Start(&p).Stop()
	}

	config.BufSz = bufSzMB * 1024 * 1024
	config.LogFrame = logFrameFunc(pktdebug)
	config.PeerDiscovery = !noDiscovery

	router := weave.NewRouter(config, name, nickName)
	Log.Println("Our name is", router.Ourself)

	dockerCli, err := docker.NewClient(apiPath)
	if err != nil {
		Log.Fatal("Unable to start docker client: ", err)
	}

	var allocator *ipam.Allocator
	var defaultSubnet address.CIDR
	if iprangeCIDR != "" {
		allocator, defaultSubnet = createAllocator(router, iprangeCIDR, ipsubnetCIDR, determineQuorum(peerCount, peers))
		if err = dockerCli.AddObserver(allocator); err != nil {
			Log.Fatal("Unable to start watcher", err)
		}
	} else if peerCount > 0 {
		Log.Fatal("--init-peer-count flag specified without --ipalloc-range")
	}

	var (
		ns        *nameserver.Nameserver
		dnsserver *nameserver.DNSServer
	)
	if !noDNS {
		ns = nameserver.New(router.Ourself.Peer.Name, router.Peers, dockerCli, dnsDomain)
		ns.SetGossip(router.NewGossip("nameserver", ns))
		if err = dockerCli.AddObserver(ns); err != nil {
			Log.Fatal("Unable to start watcher", err)
		}
		ns.Start()
		defer ns.Stop()
		dnsserver, err = nameserver.NewDNSServer(ns, dnsDomain, dnsListenAddress, uint32(dnsTTL), dnsClientTimeout)
		if err != nil {
			Log.Fatal("Unable to start dns server: ", err)
		}
		dnsserver.ActivateAndServe()
		defer dnsserver.Stop()
	}

	router.Start()
	if errors := router.ConnectionMaker.InitiateConnections(peers, false); len(errors) > 0 {
		Log.Fatal(ErrorMessages(errors))
	}

	// The weave script always waits for a status call to succeed,
	// so there is no point in doing "weave launch --http-addr ''".
	// This is here to support stand-alone use of weaver.
	if httpAddr != "" {
		muxRouter := mux.NewRouter()
		if allocator != nil {
			allocator.HandleHTTP(muxRouter, defaultSubnet, dockerCli)
		}
		if ns != nil {
			ns.HandleHTTP(muxRouter)
		}
		router.HandleHTTP(muxRouter)
		HandleHTTP(muxRouter, version, router, allocator, defaultSubnet, ns, dnsserver)
		http.Handle("/", muxRouter)
		go listenAndServeHTTP(httpAddr, muxRouter)
	}

	SignalHandlerLoop(router)
}

func options() map[string]string {
	options := make(map[string]string)
	mflag.Visit(func(f *mflag.Flag) {
		value := f.Value.String()
		name := canonicalName(f)
		if name == "password" {
			value = "<elided>"
		}
		options[name] = value
	})
	return options
}

func canonicalName(f *mflag.Flag) string {
	for _, n := range f.Names {
		if n[0] != '#' {
			return strings.TrimLeft(n, "#-")
		}
	}
	return ""
}

func logFrameFunc(debug bool) weave.LogFrameFunc {
	if !debug {
		return func(prefix string, frame []byte, dec *weave.EthernetDecoder) {}
	}
	return func(prefix string, frame []byte, dec *weave.EthernetDecoder) {
		h := fmt.Sprintf("%x", sha256.Sum256(frame))
		parts := []interface{}{prefix, len(frame), "bytes (", h, ")"}

		if dec != nil {
			parts = append(parts, dec.Eth.SrcMAC, "->", dec.Eth.DstMAC)

			if dec.DF() {
				parts = append(parts, "(DF)")
			}
		}

		Log.Println(parts...)
	}
}

func parseAndCheckCIDR(cidrStr string) address.CIDR {
	_, cidr, err := address.ParseCIDR(cidrStr)
	if err != nil {
		Log.Fatal(err)
	}
	if cidr.Size() < ipam.MinSubnetSize {
		Log.Fatalf("Allocation range smaller than minimum size %d: %s", ipam.MinSubnetSize, cidrStr)
	}
	return cidr
}

func createAllocator(router *weave.Router, ipRangeStr string, defaultSubnetStr string, quorum uint) (*ipam.Allocator, address.CIDR) {
	ipRange := parseAndCheckCIDR(ipRangeStr)
	defaultSubnet := ipRange
	if defaultSubnetStr != "" {
		defaultSubnet = parseAndCheckCIDR(defaultSubnetStr)
		if !ipRange.Range().Overlaps(defaultSubnet.Range()) {
			Log.Fatalf("Default subnet %s out of bounds: %s", defaultSubnet, ipRange)
		}
	}
	allocator := ipam.NewAllocator(router.Ourself.Peer.Name, router.Ourself.Peer.UID, router.Ourself.Peer.NickName, ipRange.Range(), quorum)

	allocator.SetInterfaces(router.NewGossip("IPallocation", allocator))
	allocator.Start()

	return allocator, defaultSubnet
}

// Pick a quorum size heuristically based on the number of peer
// addresses passed.
func determineQuorum(initPeerCountFlag int, peers []string) uint {
	if initPeerCountFlag > 0 {
		return uint(initPeerCountFlag/2 + 1)
	}

	// Guess a suitable quorum size based on the list of peer
	// addresses.  The peer list might or might not contain an
	// address for this peer, so the conservative assumption is
	// that it doesn't.  The list might contain multiple addresses
	// that resolve to the same peer, in which case the quorum
	// might be larger than it needs to be.  But the user can
	// specify it explicitly if that becomes a problem.
	clusterSize := uint(len(peers) + 1)
	quorum := clusterSize/2 + 1
	Log.Println("Assuming quorum size of", quorum)
	return quorum
}

func listenAndServeHTTP(httpAddr string, muxRouter *mux.Router) {
	protocol := "tcp"
	if strings.HasPrefix(httpAddr, "/") {
		os.Remove(httpAddr) // in case it's there from last time
		protocol = "unix"
	}
	l, err := net.Listen(protocol, httpAddr)
	if err != nil {
		Log.Fatal("Unable to create http listener socket: ", err)
	}
	err = http.Serve(l, nil)
	if err != nil {
		Log.Fatal("Unable to create http server", err)
	}
}
