package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/docker/pkg/mflag"
	"github.com/gorilla/mux"
	"github.com/pkg/profile"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/go-checkpoint"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/db"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/ipam/tracker"
	"github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
)

var version = "(unreleased version)"
var checker *checkpoint.Checker
var newVersion atomic.Value

var Log = common.Log

type ipamConfig struct {
	IPRangeCIDR   string
	IPSubnetCIDR  string
	PeerCount     int
	Mode          string
	Observer      bool
	SeedPeerNames []mesh.PeerName
}

type dnsConfig struct {
	Domain                 string
	ListenAddress          string
	TTL                    int
	ClientTimeout          time.Duration
	EffectiveListenAddress string
}

const (
	updateCheckPeriod = 6 * time.Hour
)

func checkForUpdates() {
	newVersion.Store("")

	handleResponse := func(r *checkpoint.CheckResponse, err error) {
		if err != nil {
			Log.Printf("Error checking version: %v", err)
			return
		}
		if r.Outdated {
			newVersion.Store(r.CurrentVersion)
			Log.Printf("Weave version %s is available; please update at %s",
				r.CurrentVersion, r.CurrentDownloadURL)
		}
	}

	// Start background version checking
	params := checkpoint.CheckParams{
		Product:       "weave-net",
		Version:       version,
		SignatureFile: "",
	}
	checker = checkpoint.CheckInterval(&params, updateCheckPeriod, handleResponse)
}

func (c *ipamConfig) Enabled() bool {
	var (
		hasPeerCount = c.PeerCount > 0
		hasMode      = len(c.Mode) > 0
		hasRange     = c.IPRangeCIDR != ""
		hasSubnet    = c.IPSubnetCIDR != ""
	)
	switch {
	case !(hasPeerCount || hasMode || hasRange || hasSubnet):
		return false
	case !hasRange && hasSubnet:
		Log.Fatal("--ipalloc-default-subnet specified without --ipalloc-range.")
	case !hasRange:
		Log.Fatal("--ipalloc-init or --init-peer-count specified without --ipalloc-range.")
	case hasMode && hasPeerCount:
		Log.Fatal("At most one of --ipalloc-init or --init-peer-count may be specified.")
	}
	if hasMode {
		if err := c.parseMode(); err != nil {
			Log.Fatalf("Unable to parse --ipalloc-init: %s", err)
		}
	}
	return true
}

func (c *ipamConfig) parseMode() error {
	modeAndParam := strings.SplitN(c.Mode, "=", 2)

	switch modeAndParam[0] {
	case "consensus":
		if len(modeAndParam) == 2 {
			peerCount, err := strconv.Atoi(modeAndParam[1])
			if err != nil {
				return fmt.Errorf("bad consensus parameter: %s", err)
			}
			c.PeerCount = peerCount
		}
	case "seed":
		if len(modeAndParam) != 2 {
			return fmt.Errorf("seed mode requires peer name list")
		}
		seedPeerNames, err := parsePeerNames(modeAndParam[1])
		if err != nil {
			return fmt.Errorf("bad seed parameter: %s", err)
		}
		c.SeedPeerNames = seedPeerNames
	case "observer":
		if len(modeAndParam) != 1 {
			return fmt.Errorf("observer mode takes no parameter")
		}
		c.Observer = true
	default:
		return fmt.Errorf("unknown mode: %s", modeAndParam[0])
	}

	return nil
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
		resume             bool
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
		ipamConfig         ipamConfig
		dockerAPI          string
		peers              []string
		noDNS              bool
		dnsConfig          dnsConfig
		datapathName       string
		trustedSubnetStr   string
		dbPrefix           string
		isAWSVPC           bool

		defaultDockerHost = "unix:///var/run/docker.sock"
	)

	if val := os.Getenv("DOCKER_HOST"); val != "" {
		defaultDockerHost = val
	}

	mflag.BoolVar(&justVersion, []string{"#version", "-version"}, false, "print version and exit")
	mflag.StringVar(&config.Host, []string{"-host"}, "", "router host")
	mflag.IntVar(&config.Port, []string{"#port", "-port"}, mesh.Port, "router port")
	mflag.IntVar(&protocolMinVersion, []string{"-min-protocol-version"}, mesh.ProtocolMinVersion, "minimum weave protocol version")
	mflag.BoolVar(&resume, []string{"-resume"}, false, "resume connections to previous peers")
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
	mflag.StringVar(&ipamConfig.Mode, []string{"-ipalloc-init"}, "", "allocator initialisation strategy (consensus, seed or observer)")
	mflag.StringVar(&ipamConfig.IPRangeCIDR, []string{"#iprange", "#-iprange", "-ipalloc-range"}, "", "IP address range reserved for automatic allocation, in CIDR notation")
	mflag.StringVar(&ipamConfig.IPSubnetCIDR, []string{"#ipsubnet", "#-ipsubnet", "-ipalloc-default-subnet"}, "", "subnet to allocate within by default, in CIDR notation")
	mflag.IntVar(&ipamConfig.PeerCount, []string{"#initpeercount", "#-initpeercount", "-init-peer-count"}, 0, "number of peers in network (for IP address allocation)")
	mflag.StringVar(&dockerAPI, []string{"#api", "#-api", "-docker-api"}, defaultDockerHost, "Docker API endpoint")
	mflag.BoolVar(&noDNS, []string{"-no-dns"}, false, "disable DNS server")
	mflag.StringVar(&dnsConfig.Domain, []string{"-dns-domain"}, nameserver.DefaultDomain, "local domain to server requests for")
	mflag.StringVar(&dnsConfig.ListenAddress, []string{"-dns-listen-address"}, nameserver.DefaultListenAddress, "address to listen on for DNS requests")
	mflag.IntVar(&dnsConfig.TTL, []string{"-dns-ttl"}, nameserver.DefaultTTL, "TTL for DNS request from our domain")
	mflag.DurationVar(&dnsConfig.ClientTimeout, []string{"-dns-fallback-timeout"}, nameserver.DefaultClientTimeout, "timeout for fallback DNS requests")
	mflag.StringVar(&dnsConfig.EffectiveListenAddress, []string{"-dns-effective-listen-address"}, "", "address DNS will actually be listening, after Docker port mapping")
	mflag.StringVar(&datapathName, []string{"-datapath"}, "", "ODP datapath name")
	mflag.StringVar(&trustedSubnetStr, []string{"-trusted-subnets"}, "", "comma-separated list of trusted subnets in CIDR notation")
	mflag.StringVar(&dbPrefix, []string{"-db-prefix"}, "/weavedb/weave", "pathname/prefix of filename to store data")
	mflag.BoolVar(&isAWSVPC, []string{"#awsvpc", "-awsvpc"}, false, "use AWS VPC for routing")

	// crude way of detecting that we probably have been started in a
	// container, with `weave launch` --> suppress misleading paths in
	// mflags error messages.
	if os.Args[0] == "/home/weave/weaver" { // matches the Dockerfile ENTRYPOINT
		os.Args[0] = "weave"
		mflag.CommandLine.Init("weave", mflag.ExitOnError)
	}

	mflag.Parse()

	peers = mflag.Args()
	if resume && len(peers) > 0 {
		Log.Fatalf("You must not specify an initial peer list in conjuction with --resume")
	}

	common.SetLogLevel(logLevel)

	if justVersion {
		fmt.Printf("weave router %s\n", version)
		os.Exit(0)
	}

	Log.Println("Command line options:", options())

	checkForUpdates()

	if prof != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(prof), profile.NoShutdownHook).Stop()
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

	overlay, bridge := createOverlay(datapathName, ifaceName, isAWSVPC, config.Host, config.Port, bufSzMB)
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

	db, err := db.NewBoltDB(dbPrefix + "data.db")
	checkFatal(err)
	defer db.Close()

	router := weave.NewNetworkRouter(config, networkConfig, name, nickName, overlay, db)
	Log.Println("Our name is", router.Ourself)

	if peers, err = router.InitialPeers(resume, peers); err != nil {
		Log.Fatal("Unable to get initial peer set: ", err)
	}

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
		trackerName   string
	)
	if ipamConfig.Enabled() {
		var t tracker.LocalRangeTracker
		if isAWSVPC {
			Log.Infoln("Creating AWSVPC LocalRangeTracker")
			t, err = tracker.NewAWSVPCTracker()
			if err != nil {
				Log.Fatalf("Cannot create AWSVPC LocalRangeTracker: %s", err)
			}
			trackerName = "awsvpc"
		}
		allocator, defaultSubnet = createAllocator(router, ipamConfig, db, t, isKnownPeer)
		observeContainers(allocator)
		ids, err := dockerCli.AllContainerIDs()
		checkFatal(err)
		allocator.PruneOwned(ids)
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
	if errors := router.InitiateConnections(peers, false); len(errors) > 0 {
		Log.Fatal(common.ErrorMessages(errors))
	}

	// The weave script always waits for a status call to succeed,
	// so there is no point in doing "weave launch --http-addr ''".
	// This is here to support stand-alone use of weaver.
	if httpAddr != "" {
		muxRouter := mux.NewRouter()
		if allocator != nil {
			allocator.HandleHTTP(muxRouter, defaultSubnet, trackerName, dockerCli)
		}
		if ns != nil {
			ns.HandleHTTP(muxRouter, dockerCli)
		}
		router.HandleHTTP(muxRouter)
		HandleHTTP(muxRouter, version, router, allocator, defaultSubnet, ns, dnsserver)
		http.Handle("/", common.LoggingHTTPHandler(muxRouter))
		Log.Println("Listening for HTTP control messages on", httpAddr)
		go listenAndServeHTTP(httpAddr)
	}

	common.SignalHandlerLoop(router)
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

func createOverlay(datapathName string, ifaceName string, isAWSVPC bool, host string, port int, bufSzMB int) (weave.NetworkOverlay, weave.Bridge) {
	overlay := weave.NewOverlaySwitch()
	var bridge weave.Bridge
	var ignoreSleeve bool

	switch {
	case isAWSVPC:
		vpc := weave.NewAWSVPC()
		overlay.Add("awsvpc", vpc)
		bridge = weave.NullBridge{}
		// Currently, we do not support any overlay with AWSVPC
		ignoreSleeve = true
	case datapathName != "" && ifaceName != "":
		Log.Fatal("At most one of --datapath and --iface must be specified.")
	case datapathName != "":
		iface, err := weavenet.EnsureInterface(datapathName)
		checkFatal(err)
		fastdp, err := weave.NewFastDatapath(iface, port)
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

	if !ignoreSleeve {
		sleeve := weave.NewSleeveOverlay(host, port)
		overlay.Add("sleeve", sleeve)
		overlay.SetCompatOverlay(sleeve)
	}

	return overlay, bridge
}

func createAllocator(router *weave.NetworkRouter, config ipamConfig, db db.DB, track tracker.LocalRangeTracker, isKnownPeer func(mesh.PeerName) bool) (*ipam.Allocator, address.CIDR) {
	ipRange, err := ipam.ParseCIDRSubnet(config.IPRangeCIDR)
	checkFatal(err)
	defaultSubnet := ipRange
	if config.IPSubnetCIDR != "" {
		defaultSubnet, err = ipam.ParseCIDRSubnet(config.IPSubnetCIDR)
		checkFatal(err)
		if !ipRange.Range().Overlaps(defaultSubnet.Range()) {
			Log.Fatalf("IP address allocation default subnet %s does not overlap with allocation range %s", defaultSubnet, ipRange)
		}
	}

	c := ipam.Config{
		OurName:     router.Ourself.Peer.Name,
		OurUID:      router.Ourself.Peer.UID,
		OurNickname: router.Ourself.Peer.NickName,
		Seed:        config.SeedPeerNames,
		Universe:    ipRange,
		IsObserver:  config.Observer,
		Quorum:      func() uint { return determineQuorum(config.PeerCount, router) },
		Db:          db,
		IsKnownPeer: isKnownPeer,
		Tracker:     track,
	}

	allocator := ipam.NewAllocator(c)

	allocator.SetInterfaces(router.NewGossip("IPallocation", allocator))
	allocator.Start()
	router.Peers.OnGC(func(peer *mesh.Peer) { allocator.PeerGone(peer.Name) })

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

// Pick a quorum size based on the number of peer addresses.
func determineQuorum(initPeerCountFlag int, router *weave.NetworkRouter) uint {
	if initPeerCountFlag > 0 {
		return uint(initPeerCountFlag/2 + 1)
	}

	peers := router.ConnectionMaker.Targets(true)

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

func parsePeerNames(s string) ([]mesh.PeerName, error) {
	peerNames := []mesh.PeerName{}
	if s == "" {
		return peerNames, nil
	}

	for _, peerNameStr := range strings.Split(s, ",") {
		peerName, err := mesh.PeerNameFromUserInput(peerNameStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing peer names: %s", err)
		}
		peerNames = append(peerNames, peerName)
	}

	return peerNames, nil
}

func listenAndServeHTTP(httpAddr string) {
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
