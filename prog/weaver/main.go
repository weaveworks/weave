package main

import (
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
	"github.com/weaveworks/mesh"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
)

var version = "(unreleased version)"

type dnsConfig struct {
	Domain                 string
	ListenAddress          string
	TTL                    int
	ClientTimeout          time.Duration
	EffectiveListenAddress string
}

func main() {
	procs := runtime.NumCPU()
	// packet sniffing can block an OS thread, so we need one thread
	// for that plus at least one more.
	if procs < 2 {
		procs = 2
	}
	runtime.GOMAXPROCS(procs)

	var (
		justVersion        bool
		config             mesh.Config
		networkConfig      weave.NetworkConfig
		protocolMinVersion int
		ifaceName          string
		routerName         string
		nickName           string
		password           string
		pktdebug           bool
		logLevel           string
		prof               string
		bufSzMB            int
		noDiscovery        bool
		httpAddr           string
		iprangeCIDR        string
		ipsubnetCIDR       string
		peerCount          int
		dockerAPI          string
		peers              []string
		noDNS              bool
		dnsConfig          dnsConfig
		datapathName       string
		trustedSubnetStr   string

		defaultDockerHost = "unix:///var/run/docker.sock"
	)

	if val := os.Getenv("DOCKER_HOST"); val != "" {
		defaultDockerHost = val
	}

	mflag.BoolVar(&justVersion, []string{"#version", "-version"}, false, "print version and exit")
	mflag.StringVar(&config.Host, []string{"-host"}, "", "router host")
	mflag.IntVar(&config.Port, []string{"#port", "-port"}, mesh.Port, "router port")
	mflag.IntVar(&protocolMinVersion, []string{"-min-protocol-version"}, mesh.ProtocolMinVersion, "minimum weave protocol version")
	mflag.StringVar(&ifaceName, []string{"#iface", "-iface"}, "", "name of interface to capture/inject from (disabled if blank)")
	mflag.StringVar(&routerName, []string{"#name", "-name"}, "", "name of router (defaults to MAC of interface)")
	mflag.StringVar(&nickName, []string{"#nickname", "-nickname"}, "", "nickname of peer (defaults to hostname)")
	mflag.StringVar(&password, []string{"#password", "-password"}, "", "network password")
	mflag.StringVar(&logLevel, []string{"-log-level"}, "info", "logging level (debug, info, warning, error)")
	mflag.BoolVar(&pktdebug, []string{"#pktdebug", "#-pktdebug", "-pkt-debug"}, false, "enable per-packet debug logging")
	mflag.StringVar(&prof, []string{"#profile", "-profile"}, "", "enable profiling and write profiles to given path")
	mflag.IntVar(&config.ConnLimit, []string{"#connlimit", "#-connlimit", "-conn-limit"}, 30, "connection limit (0 for unlimited)")
	mflag.BoolVar(&noDiscovery, []string{"#nodiscovery", "#-nodiscovery", "-no-discovery"}, false, "disable peer discovery")
	mflag.IntVar(&bufSzMB, []string{"#bufsz", "-bufsz"}, 8, "capture buffer size in MB")
	mflag.StringVar(&httpAddr, []string{"#httpaddr", "#-httpaddr", "-http-addr"}, "", "address to bind HTTP interface to (disabled if blank, absolute path indicates unix domain socket)")
	mflag.StringVar(&iprangeCIDR, []string{"#iprange", "#-iprange", "-ipalloc-range"}, "", "IP address range reserved for automatic allocation, in CIDR notation")
	mflag.StringVar(&ipsubnetCIDR, []string{"#ipsubnet", "#-ipsubnet", "-ipalloc-default-subnet"}, "", "subnet to allocate within by default, in CIDR notation")
	mflag.IntVar(&peerCount, []string{"#initpeercount", "#-initpeercount", "-init-peer-count"}, 0, "number of peers in network (for IP address allocation)")
	mflag.StringVar(&dockerAPI, []string{"#api", "#-api", "-docker-api"}, defaultDockerHost, "Docker API endpoint")
	mflag.BoolVar(&noDNS, []string{"-no-dns"}, false, "disable DNS server")
	mflag.StringVar(&dnsConfig.Domain, []string{"-dns-domain"}, nameserver.DefaultDomain, "local domain to server requests for")
	mflag.StringVar(&dnsConfig.ListenAddress, []string{"-dns-listen-address"}, nameserver.DefaultListenAddress, "address to listen on for DNS requests")
	mflag.IntVar(&dnsConfig.TTL, []string{"-dns-ttl"}, nameserver.DefaultTTL, "TTL for DNS request from our domain")
	mflag.DurationVar(&dnsConfig.ClientTimeout, []string{"-dns-fallback-timeout"}, nameserver.DefaultClientTimeout, "timeout for fallback DNS requests")
	mflag.StringVar(&dnsConfig.EffectiveListenAddress, []string{"-dns-effective-listen-address"}, "", "address DNS will actually be listening, after Docker port mapping")
	mflag.StringVar(&datapathName, []string{"-datapath"}, "", "ODP datapath name")

	mflag.StringVar(&trustedSubnetStr, []string{"-trusted-subnets"}, "", "Command separated list of trusted subnets in CIDR notation")

	// crude way of detecting that we probably have been started in a
	// container, with `weave launch` --> suppress misleading paths in
	// mflags error messages.
	if os.Args[0] == "/home/weave/weaver" { // matches the Dockerfile ENTRYPOINT
		os.Args[0] = "weave"
		mflag.CommandLine.Init("weave", mflag.ExitOnError)
	}

	mflag.Parse()

	peers = mflag.Args()

	SetLogLevel(logLevel)

	if justVersion {
		fmt.Printf("weave router %s\n", version)
		os.Exit(0)
	}

	Log.Println("Command line options:", options())
	Log.Println("Command line peers:", peers)

	if prof != "" {
		p := *profile.CPUProfile
		p.ProfilePath = prof
		p.NoShutdownHook = true
		defer profile.Start(&p).Stop()
	}

	if protocolMinVersion < mesh.ProtocolMinVersion || protocolMinVersion > mesh.ProtocolMaxVersion {
		Log.Fatalf("--min-protocol-version must be in range [%d,%d]", mesh.ProtocolMinVersion, mesh.ProtocolMaxVersion)
	}
	config.ProtocolMinVersion = byte(protocolMinVersion)

	if pktdebug {
		networkConfig.PacketLogging = packetLogging{}
	} else {
		networkConfig.PacketLogging = nopPacketLogging{}
	}

	overlay, bridge := createOverlay(datapathName, ifaceName, config.Host, config.Port, bufSzMB)
	networkConfig.Bridge = bridge

	name := peerName(routerName, bridge.Interface())

	if nickName == "" {
		var err error
		nickName, err = os.Hostname()
		checkFatal(err)
	}

	config.Password = determinePassword(password)
	config.TrustedSubnets = parseTrustedSubnets(trustedSubnetStr)
	config.PeerDiscovery = !noDiscovery

	router := weave.NewNetworkRouter(config, networkConfig, name, nickName, overlay)
	Log.Println("Our name is", router.Ourself)

	var dockerCli *docker.Client
	if dockerAPI != "" {
		dc, err := docker.NewClient(dockerAPI)
		if err != nil {
			Log.Fatal("Unable to start docker client: ", err)
		} else {
			Log.Info(dc.Info())
		}
		dockerCli = dc
	}
	observeContainers := func(o docker.ContainerObserver) {
		if dockerCli != nil {
			if err := dockerCli.AddObserver(o); err != nil {
				Log.Fatal("Unable to start watcher", err)
			}
		}
	}
	isKnownPeer := func(name mesh.PeerName) bool {
		return router.Peers.Fetch(name) != nil
	}

	var (
		allocator     *ipam.Allocator
		defaultSubnet address.CIDR
	)
	if iprangeCIDR != "" {
		allocator, defaultSubnet = createAllocator(router.Router, iprangeCIDR, ipsubnetCIDR, determineQuorum(peerCount, peers), isKnownPeer)
		observeContainers(allocator)
	} else if peerCount > 0 {
		Log.Fatal("--init-peer-count flag specified without --ipalloc-range")
	}

	var (
		ns        *nameserver.Nameserver
		dnsserver *nameserver.DNSServer
	)
	if !noDNS {
		ns, dnsserver = createDNSServer(dnsConfig, router.Router, isKnownPeer)
		observeContainers(ns)
		ns.Start()
		defer ns.Stop()
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
			ns.HandleHTTP(muxRouter, dockerCli)
		}
		router.HandleHTTP(muxRouter)
		HandleHTTP(muxRouter, version, router, allocator, defaultSubnet, ns, dnsserver)
		http.Handle("/", muxRouter)
		Log.Println("Listening for HTTP control messages on", httpAddr)
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

type packetLogging struct{}

func (packetLogging) LogPacket(msg string, key weave.PacketKey) {
	Log.Println(msg, key.SrcMAC, "->", key.DstMAC)
}

func (packetLogging) LogForwardPacket(msg string, key weave.ForwardPacketKey) {
	Log.Println(msg, key.SrcPeer, key.SrcMAC, "->", key.DstPeer, key.DstMAC)
}

type nopPacketLogging struct{}

func (nopPacketLogging) LogPacket(string, weave.PacketKey) {
}

func (nopPacketLogging) LogForwardPacket(string, weave.ForwardPacketKey) {
}

func createOverlay(datapathName string, ifaceName string, host string, port int, bufSzMB int) (weave.NetworkOverlay, weave.Bridge) {
	overlay := weave.NewOverlaySwitch()
	var bridge weave.Bridge
	switch {
	case datapathName != "" && ifaceName != "":
		Log.Fatal("At most one of --datapath and --iface must be specified.")
	case datapathName != "":
		fastdp, err := weave.NewFastDatapath(datapathName, port)
		checkFatal(err)
		bridge = fastdp.Bridge()
		overlay.Add("fastdp", fastdp.Overlay())
	case ifaceName != "":
		iface, err := weavenet.EnsureInterface(ifaceName)
		checkFatal(err)
		bridge, err = weave.NewPcap(iface, bufSzMB*1024*1024) // bufsz flag is in MB
		checkFatal(err)
	default:
		bridge = weave.NullBridge{}
	}
	sleeve := weave.NewSleeveOverlay(host, port)
	overlay.Add("sleeve", sleeve)
	overlay.SetCompatOverlay(sleeve)
	return overlay, bridge
}

func parseAndCheckCIDR(cidrStr string) address.CIDR {
	_, cidr, err := address.ParseCIDR(cidrStr)
	checkFatal(err)

	if cidr.Size() < ipam.MinSubnetSize {
		Log.Fatalf("Allocation range smaller than minimum size %d: %s", ipam.MinSubnetSize, cidrStr)
	}
	return cidr
}

func createAllocator(router *mesh.Router, ipRangeStr string, defaultSubnetStr string, quorum uint, isKnownPeer func(mesh.PeerName) bool) (*ipam.Allocator, address.CIDR) {
	ipRange := parseAndCheckCIDR(ipRangeStr)
	defaultSubnet := ipRange
	if defaultSubnetStr != "" {
		defaultSubnet = parseAndCheckCIDR(defaultSubnetStr)
		if !ipRange.Range().Overlaps(defaultSubnet.Range()) {
			Log.Fatalf("IP address allocation default subnet %s does not overlap with allocation range %s", defaultSubnet, ipRange)
		}
	}
	allocator := ipam.NewAllocator(router.Ourself.Peer.Name, router.Ourself.Peer.UID, router.Ourself.Peer.NickName, ipRange.Range(), quorum, isKnownPeer)

	allocator.SetInterfaces(router.NewGossip("IPallocation", allocator))
	allocator.Start()

	return allocator, defaultSubnet
}

func createDNSServer(config dnsConfig, router *mesh.Router, isKnownPeer func(mesh.PeerName) bool) (*nameserver.Nameserver, *nameserver.DNSServer) {
	ns := nameserver.New(router.Ourself.Peer.Name, config.Domain, isKnownPeer)
	router.Peers.OnGC(func(peer *mesh.Peer) { ns.PeerGone(peer.Name) })
	ns.SetGossip(router.NewGossip("nameserver", ns))
	dnsserver, err := nameserver.NewDNSServer(ns, config.Domain, config.ListenAddress,
		config.EffectiveListenAddress, uint32(config.TTL), config.ClientTimeout)
	if err != nil {
		Log.Fatal("Unable to start dns server: ", err)
	}
	listenAddr := config.ListenAddress
	if config.EffectiveListenAddress != "" {
		listenAddr = config.EffectiveListenAddress
	}
	Log.Println("Listening for DNS queries on", listenAddr)
	return ns, dnsserver
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

func determinePassword(password string) []byte {
	if password == "" {
		password = os.Getenv("WEAVE_PASSWORD")
	}
	if password == "" {
		Log.Println("Communication between peers is unencrypted.")
		return nil
	}
	Log.Println("Communication between peers via untrusted networks is encrypted.")
	return []byte(password)
}

func peerName(routerName string, iface *net.Interface) mesh.PeerName {
	if routerName == "" {
		if iface == nil {
			Log.Fatal("Either an interface must be specified with --datapath or --iface, or a name with --name")
		}
		routerName = iface.HardwareAddr.String()
	}
	name, err := mesh.PeerNameFromUserInput(routerName)
	checkFatal(err)
	return name
}

func parseTrustedSubnets(trustedSubnetStr string) []*net.IPNet {
	trustedSubnets := []*net.IPNet{}
	if trustedSubnetStr == "" {
		return trustedSubnets
	}

	for _, subnetStr := range strings.Split(trustedSubnetStr, ",") {
		_, subnet, err := net.ParseCIDR(subnetStr)
		if err != nil {
			Log.Fatal("Unable to parse trusted subnets: ", err)
		}
		trustedSubnets = append(trustedSubnets, subnet)
	}

	return trustedSubnets
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

func checkFatal(e error) {
	if e != nil {
		Log.Fatal(e)
	}
}
