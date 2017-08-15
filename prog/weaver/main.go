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
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/profile"
	"github.com/weaveworks/common/mflag"
	"github.com/weaveworks/common/mflagext"
	"github.com/weaveworks/common/signals"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/db"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/ipam/tracker"
	"github.com/weaveworks/weave/nameserver"
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/plugin"
	weaveproxy "github.com/weaveworks/weave/proxy"
	weave "github.com/weaveworks/weave/router"
)

var version = "unreleased"

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
	ResolvConf             string
}

func (c *ipamConfig) Enabled() bool {
	var (
		hasPeerCount = c.PeerCount > 0
		hasMode      = c.HasMode()
		hasRange     = c.IPRangeCIDR != ""
		hasSubnet    = c.IPSubnetCIDR != ""
	)
	switch {
	case !(hasPeerCount || hasMode || hasRange || hasSubnet):
		return false
	case !hasRange && hasSubnet:
		Log.Fatal("--ipalloc-default-subnet specified without --ipalloc-range.")
	case !hasRange:
		Log.Fatal("--ipalloc-init specified without --ipalloc-range.")
	}
	if hasMode {
		if err := c.parseMode(); err != nil {
			Log.Fatalf("Unable to parse --ipalloc-init: %s", err)
		}
	}
	return true
}

func (c ipamConfig) HasMode() bool {
	return len(c.Mode) > 0
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

func getenvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
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
		bridgeConfig       weavenet.BridgeConfig
		networkConfig      weave.NetworkConfig
		protocolMinVersion int
		resume             bool
		routerName         string
		nickName           string
		password           string
		pktdebug           bool
		logLevel           = "info"
		prof               string
		bufSzMB            int
		noDiscovery        bool
		httpAddr           string
		statusAddr         string
		ipamConfig         ipamConfig
		dockerAPI          string
		peers              []string
		noDNS              bool
		dnsConfig          dnsConfig
		trustedSubnetStr   string
		dbPrefix           string
		hostRoot           string
		procPath           string
		discoveryEndpoint  string
		token              string
		advertiseAddress   string
		pluginConfig       plugin.Config
		defaultDockerHost  = getenvOrDefault("DOCKER_HOST", "unix:///var/run/docker.sock")
	)

	mflag.BoolVar(&justVersion, []string{"-version"}, false, "print version and exit")
	mflag.StringVar(&config.Host, []string{"-host"}, "", "router host")
	mflag.IntVar(&config.Port, []string{"-port"}, mesh.Port, "router port")
	mflag.IntVar(&protocolMinVersion, []string{"-min-protocol-version"}, mesh.ProtocolMinVersion, "minimum weave protocol version")
	mflag.BoolVar(&resume, []string{"-resume"}, false, "resume connections to previous peers")
	mflag.StringVar(&bridgeConfig.WeaveBridgeName, []string{"-weave-bridge"}, "weave", "name of weave bridge")
	mflag.StringVar(&bridgeConfig.DockerBridgeName, []string{"-docker-bridge"}, "", "name of Docker bridge")
	mflag.BoolVar(&bridgeConfig.NPC, []string{"-expect-npc"}, false, "set up iptables rules for npc")
	mflag.StringVar(&routerName, []string{"-name"}, "", "name of router (defaults to MAC of interface)")
	mflag.StringVar(&nickName, []string{"-nickname"}, "", "nickname of peer (defaults to hostname)")
	mflag.StringVar(&password, []string{"-password"}, "", "network password")
	mflag.StringVar(&logLevel, []string{"-log-level"}, "info", "logging level (debug, info, warning, error)")
	mflag.BoolVar(&pktdebug, []string{"-pkt-debug"}, false, "enable per-packet debug logging")
	mflag.StringVar(&prof, []string{"-profile"}, "", "enable profiling and write profiles to given path")
	mflag.IntVar(&config.ConnLimit, []string{"-conn-limit"}, 30, "connection limit (0 for unlimited)")
	mflag.BoolVar(&noDiscovery, []string{"-no-discovery"}, false, "disable peer discovery")
	mflag.IntVar(&bufSzMB, []string{"-bufsz"}, 8, "capture buffer size in MB")
	mflag.IntVar(&bridgeConfig.MTU, []string{"-mtu"}, 0, "MTU size")
	mflag.StringVar(&httpAddr, []string{"-http-addr"}, "", "address to bind HTTP interface to (disabled if blank, absolute path indicates unix domain socket)")
	mflag.StringVar(&statusAddr, []string{"-status-addr"}, "", "address to bind status+metrics interface to (disabled if blank, absolute path indicates unix domain socket)")
	mflag.StringVar(&ipamConfig.Mode, []string{"-ipalloc-init"}, "", "allocator initialisation strategy (consensus, seed or observer)")
	mflag.StringVar(&ipamConfig.IPRangeCIDR, []string{"-ipalloc-range"}, "", "IP address range reserved for automatic allocation, in CIDR notation")
	mflag.StringVar(&ipamConfig.IPSubnetCIDR, []string{"-ipalloc-default-subnet"}, "", "subnet to allocate within by default, in CIDR notation")
	mflag.StringVar(&dockerAPI, []string{"-docker-api"}, defaultDockerHost, "Docker API endpoint")
	mflag.BoolVar(&noDNS, []string{"-no-dns"}, false, "disable DNS server")
	mflag.StringVar(&dnsConfig.Domain, []string{"-dns-domain"}, nameserver.DefaultDomain, "local domain to server requests for")
	mflag.StringVar(&dnsConfig.ListenAddress, []string{"-dns-listen-address"}, nameserver.DefaultListenAddress, "address to listen on for DNS requests")
	mflag.IntVar(&dnsConfig.TTL, []string{"-dns-ttl"}, nameserver.DefaultTTL, "TTL for DNS request from our domain")
	mflag.DurationVar(&dnsConfig.ClientTimeout, []string{"-dns-fallback-timeout"}, nameserver.DefaultClientTimeout, "timeout for fallback DNS requests")
	mflag.StringVar(&dnsConfig.EffectiveListenAddress, []string{"-dns-effective-listen-address"}, "", "address DNS will actually be listening, after Docker port mapping")
	mflag.StringVar(&dnsConfig.ResolvConf, []string{"-resolv-conf"}, "", "path to resolver configuration for fallback DNS lookups")
	mflag.StringVar(&bridgeConfig.DatapathName, []string{"-datapath"}, "", "ODP datapath name")
	mflag.BoolVar(&bridgeConfig.NoFastdp, []string{"-no-fastdp"}, false, "Disable Fast Datapath")
	mflag.StringVar(&trustedSubnetStr, []string{"-trusted-subnets"}, "", "comma-separated list of trusted subnets in CIDR notation")
	mflag.StringVar(&dbPrefix, []string{"-db-prefix"}, "/weavedb/weave", "pathname/prefix of filename to store data")
	mflag.StringVar(&procPath, []string{"-proc-path"}, "/proc", "path to reach host /proc filesystem")
	mflag.BoolVar(&bridgeConfig.AWSVPC, []string{"-awsvpc"}, false, "use AWS VPC for routing")
	mflag.StringVar(&hostRoot, []string{"-host-root"}, "", "path to reach host filesystem")
	mflag.StringVar(&discoveryEndpoint, []string{"-peer-discovery-url"}, "https://cloud.weave.works/api/net", "url for peer discovery")
	mflag.StringVar(&token, []string{"-token"}, "", "token for peer discovery")
	mflag.StringVar(&advertiseAddress, []string{"-advertise-address"}, "", "address to advertise for peer discovery")

	mflag.BoolVar(&pluginConfig.Enable, []string{"-plugin"}, false, "enable Docker plugin (v1)")
	mflag.BoolVar(&pluginConfig.EnableV2, []string{"-plugin-v2"}, false, "enable Docker plugin (v2)")
	mflag.BoolVar(&pluginConfig.EnableV2Multicast, []string{"-plugin-v2-multicast"}, false, "enable multicast for Docker plugin (v2)")
	mflag.StringVar(&pluginConfig.Socket, []string{"-plugin-socket"}, "/run/docker/plugins/weave.sock", "plugin socket on which to listen")
	mflag.StringVar(&pluginConfig.MeshSocket, []string{"-plugin-mesh-socket"}, "/run/docker/plugins/weavemesh.sock", "plugin socket on which to listen in mesh mode")

	proxyConfig := configureProxy(version, defaultDockerHost)
	if bridgeConfig.AWSVPC {
		proxyConfig.NoMulticastRoute = true
		proxyConfig.KeepTXOn = true
	}

	// crude way of detecting that we probably have been started in a
	// container, with `weave launch` --> suppress misleading paths in
	// mflags error messages.
	if os.Args[0] == "/home/weave/weaver" { // matches the Dockerfile ENTRYPOINT
		os.Args[0] = "weave"
		mflag.CommandLine.Init("weave", mflag.ExitOnError)
	}

	mflag.Parse()

	if justVersion {
		fmt.Printf("weave %s\n", version)
		os.Exit(0)
	}

	peers = mflag.Args()
	if resume && len(peers) > 0 {
		Log.Fatalf("You must not specify an initial peer list in conjunction with --resume")
	}

	common.SetLogLevel(logLevel)
	Log.Println("Command line options:", options())
	Log.Infoln("weave ", version)

	if prof != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(prof), profile.NoShutdownHook).Stop()
	}

	if protocolMinVersion < mesh.ProtocolMinVersion || protocolMinVersion > mesh.ProtocolMaxVersion {
		Log.Fatalf("--min-protocol-version must be in range [%d,%d]", mesh.ProtocolMinVersion, mesh.ProtocolMaxVersion)
	}
	config.ProtocolMinVersion = byte(protocolMinVersion)

	var waitReady common.WaitGroup

	var proxy *weaveproxy.Proxy
	var err error
	if proxyConfig.Enabled {
		if noDNS {
			proxyConfig.WithoutDNS = true
		}
		// Start Weave Proxy:
		proxy, err = weaveproxy.NewProxy(*proxyConfig)
		if err != nil {
			Log.Fatalf("Could not start proxy: %s", err)
		}
		defer proxy.Stop()
		listeners := proxy.Listen()
		proxy.AttachExistingContainers()
		go proxy.Serve(listeners, waitReady.Add())
	}

	if pktdebug {
		networkConfig.PacketLogging = packetLogging{}
	} else {
		networkConfig.PacketLogging = nopPacketLogging{}
	}

	if bridgeConfig.DockerBridgeName != "" {
		if setAddr, err := weavenet.EnforceAddrAssignType(bridgeConfig.DockerBridgeName); err != nil {
			Log.Errorf("While checking address assignment type of %s: %s", bridgeConfig.DockerBridgeName, err)
		} else if setAddr {
			Log.Warningf("Setting %s MAC (mitigate https://github.com/docker/docker/issues/14908)", bridgeConfig.DockerBridgeName)
		}
	}

	name := peerName(routerName, bridgeConfig.WeaveBridgeName, dbPrefix, hostRoot)

	bridgeConfig.Mac = name.String()
	bridgeType, err := weavenet.EnsureBridge(procPath, &bridgeConfig, Log)
	checkFatal(err)
	Log.Println("Bridge type is", bridgeType)

	config.Password = determinePassword(password)

	overlay, injectorConsumer := createOverlay(bridgeType, bridgeConfig, config.Host, config.Port, bufSzMB, config.Password != nil)
	networkConfig.InjectorConsumer = injectorConsumer

	if injectorConsumer != nil {
		if err := weavenet.DetectHairpin("vethwe-bridge", Log); err != nil {
			Log.Errorf("Setting may cause connectivity issues : %s", err)
			Log.Infof("Hairpin mode may have been enabled by other software on this machine")
		}
	}

	if nickName == "" {
		var err error
		nickName, err = os.Hostname()
		checkFatal(err)
	}

	config.TrustedSubnets = parseTrustedSubnets(trustedSubnetStr)
	config.PeerDiscovery = !noDiscovery

	if bridgeConfig.AWSVPC && len(config.Password) > 0 {
		Log.Fatalf("--awsvpc mode is not compatible with the --password option")
	}
	if bridgeConfig.AWSVPC && !ipamConfig.Enabled() {
		Log.Fatalf("--awsvpc mode requires IPAM enabled")
	}

	db, err := db.NewBoltDB(dbPrefix)
	checkFatal(err)
	defer db.Close()

	router, err := weave.NewNetworkRouter(config, networkConfig, name, nickName, overlay, db)
	checkFatal(err)
	Log.Println("Our name is", router.Ourself)

	if token != "" {
		var addresses []string
		if advertiseAddress == "" {
			localAddrs, err := weavenet.LocalAddresses()
			checkFatal(err)
			for _, addr := range localAddrs {
				addresses = append(addresses, addr.IP.String())
			}
		} else {
			addresses = strings.Split(advertiseAddress, ",")
		}
		discoveredPeers, count, err := peerDiscoveryUpdate(discoveryEndpoint, token, name.String(), nickName, addresses)
		checkFatal(err)
		if !ipamConfig.HasMode() {
			ipamConfig.PeerCount = len(peers) + count
		}
		peers = append(peers, discoveredPeers...)
	} else if peers, err = router.InitialPeers(resume, peers); err != nil {
		Log.Fatal("Unable to get initial peer set: ", err)
	}

	var dockerCli *docker.Client
	dockerVersion := "none"
	if dockerAPI != "" {
		dc, err := docker.NewClient(dockerAPI)
		if err != nil {
			Log.Fatal("Unable to start docker client: ", err)
		} else {
			Log.Info(dc.Info())
		}
		dockerCli = dc
		dockerVersion = dockerCli.DockerVersion()
	}

	checkForUpdates(dockerVersion, router)

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
		if bridgeConfig.AWSVPC {
			Log.Infoln("Creating AWSVPC LocalRangeTracker")
			t, err = tracker.NewAWSVPCTracker(bridgeConfig.WeaveBridgeName)
			if err != nil {
				Log.Fatalf("Cannot create AWSVPC LocalRangeTracker: %s", err)
			}
			trackerName = "awsvpc"
		}

		preClaims, err := findExistingAddresses(dockerCli, bridgeConfig.WeaveBridgeName)
		checkFatal(err)

		allocator, defaultSubnet = createAllocator(router, ipamConfig, preClaims, db, t, isKnownPeer)
		observeContainers(allocator)

		if dockerCli != nil {
			allContainerIDs, err := dockerCli.RunningContainerIDs()
			checkFatal(err)
			allocator.PruneOwned(allContainerIDs)
		}
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
		if dockerCli != nil {
			populateDNS(ns, dockerCli, name, bridgeConfig.WeaveBridgeName)
		}
		defer dnsserver.Stop()
	}

	router.Start()
	if errors := router.InitiateConnections(peers, false); len(errors) > 0 {
		Log.Fatal(common.ErrorMessages(errors))
	}
	checkFatal(router.CreateRestartSentinel())

	pluginConfig.DNS = !noDNS
	pluginConfig.DefaultSubnet = defaultSubnet.String()
	plugin := plugin.NewPlugin(pluginConfig)

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
		HandleHTTP(muxRouter, version, router, allocator, defaultSubnet, ns, dnsserver, proxy, plugin, &waitReady)
		HandleHTTPPeer(muxRouter, allocator, discoveryEndpoint, token, name.String())
		muxRouter.Methods("GET").Path("/metrics").Handler(metricsHandler(router, allocator, ns, dnsserver))
		if proxy != nil {
			muxRouter.Methods("GET").Path("/proxyaddrs").HandlerFunc(proxy.StatusHTTP)
		}
		http.Handle("/", common.LoggingHTTPHandler(muxRouter))
		Log.Println("Listening for HTTP control messages on", httpAddr)
		go listenAndServeHTTP(httpAddr, nil)
	}

	if statusAddr != "" {
		muxRouter := mux.NewRouter()
		HandleHTTP(muxRouter, version, router, allocator, defaultSubnet, ns, dnsserver, proxy, plugin, &waitReady)
		muxRouter.Methods("GET").Path("/metrics").Handler(metricsHandler(router, allocator, ns, dnsserver))
		statusMux := http.NewServeMux()
		statusMux.Handle("/", muxRouter)
		Log.Println("Listening for metrics requests on", statusAddr)
		go listenAndServeHTTP(statusAddr, statusMux)
	}

	if plugin != nil {
		go plugin.Start(httpAddr, dockerCli, waitReady.Add())
	}

	if bridgeConfig.AWSVPC {
		// Run this on its own goroutine because the allocator can block
		// We remove the default route installed by the kernel,
		// because awsvpc has installed it as well
		go expose(allocator, defaultSubnet, bridgeConfig.WeaveBridgeName, bridgeConfig.AWSVPC, waitReady.Add())
	}

	signals.SignalHandlerLoop(common.Log, router)
}

func expose(alloc *ipam.Allocator, subnet address.CIDR, bridgeName string, removeDefaultRoute bool, ready func()) {
	addr, err := alloc.Allocate("weave:expose", subnet, false, func() bool { return false })
	checkFatal(err)
	cidr := address.MakeCIDR(subnet, addr)
	err = weavenet.AddBridgeAddr(bridgeName, cidr.IPNet(), removeDefaultRoute)
	checkFatal(err)
	Log.Printf("Bridge %q exposed on address %v", bridgeName, cidr)
	ready()
}

func options() map[string]string {
	options := make(map[string]string)
	mflag.Visit(func(f *mflag.Flag) {
		value := f.Value.String()
		name := canonicalName(f)
		if name == "password" || name == "token" {
			value = "<redacted>"
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

func configureProxy(version string, defaultDockerHost string) *weaveproxy.Config {
	proxyConfig := weaveproxy.Config{
		Version:      version,
		Image:        getenvOrDefault("EXEC_IMAGE", "weaveworks/weaveexec"),
		DockerBridge: getenvOrDefault("DOCKER_BRIDGE", "docker0"),
		DockerHost:   defaultDockerHost,
	}
	mflag.BoolVar(&proxyConfig.Enabled, []string{"-proxy"}, false, "instruct Weave Net to start its Docker proxy")
	mflagext.ListVar(&proxyConfig.ListenAddrs, []string{"H"}, nil, "addresses on which to listen for Docker proxy")
	mflag.StringVar(&proxyConfig.HostnameFromLabel, []string{"-hostname-from-label"}, "", "Key of container label from which to obtain the container's hostname")
	mflag.StringVar(&proxyConfig.HostnameMatch, []string{"-hostname-match"}, "(.*)", "Regexp pattern to apply on container names (e.g. '^aws-[0-9]+-(.*)$')")
	mflag.StringVar(&proxyConfig.HostnameReplacement, []string{"-hostname-replacement"}, "$1", "Expression to generate hostnames based on matches from --hostname-match (e.g. 'my-app-$1')")
	mflag.BoolVar(&proxyConfig.RewriteInspect, []string{"-rewrite-inspect"}, false, "Rewrite 'inspect' calls to return the weave network settings (if attached)")
	mflag.BoolVar(&proxyConfig.NoDefaultIPAM, []string{"-no-default-ipalloc"}, false, "proxy: do not automatically allocate addresses for containers without a WEAVE_CIDR")
	mflag.BoolVar(&proxyConfig.NoRewriteHosts, []string{"-no-rewrite-hosts"}, false, "proxy: do not automatically rewrite /etc/hosts. Use if you need the docker IP to remain in /etc/hosts")
	mflag.StringVar(&proxyConfig.TLSConfig.CACert, []string{"-tlscacert"}, "", "Trust certs signed only by this CA")
	mflag.StringVar(&proxyConfig.TLSConfig.Cert, []string{"-tlscert"}, "", "Path to TLS certificate file")
	mflag.BoolVar(&proxyConfig.TLSConfig.Enabled, []string{"-tls"}, false, "Use TLS; implied by --tlsverify")
	mflag.StringVar(&proxyConfig.TLSConfig.Key, []string{"-tlskey"}, "", "Path to TLS key file")
	mflag.BoolVar(&proxyConfig.TLSConfig.Verify, []string{"-tlsverify"}, false, "Use TLS and verify the remote")
	mflag.BoolVar(&proxyConfig.WithoutDNS, []string{"-without-dns"}, false, "proxy: instruct created containers to never use weaveDNS as their nameserver")
	mflag.BoolVar(&proxyConfig.NoMulticastRoute, []string{"-no-multicast-route"}, false, "proxy: do not add a multicast route via the weave interface when attaching containers")
	return &proxyConfig
}

func createOverlay(bridgeType weavenet.Bridge, config weavenet.BridgeConfig, host string, port int, bufSzMB int, enableEncryption bool) (weave.NetworkOverlay, weave.InjectorConsumer) {
	overlay := weave.NewOverlaySwitch()
	var injectorConsumer weave.InjectorConsumer
	var ignoreSleeve bool

	switch {
	case config.AWSVPC:
		vpc := weave.NewAWSVPC()
		overlay.Add("awsvpc", vpc)
		injectorConsumer = weave.NullInjectorConsumer{}
		// Currently, we do not support any overlay with AWSVPC
		ignoreSleeve = true
	case bridgeType == nil:
		injectorConsumer = weave.NullInjectorConsumer{}
	case bridgeType.IsFastdp():
		iface, err := weavenet.EnsureInterface(config.DatapathName)
		checkFatal(err)
		fastdp, err := weave.NewFastDatapath(iface, port, enableEncryption)
		checkFatal(err)
		injectorConsumer = fastdp.InjectorConsumer()
		overlay.Add("fastdp", fastdp.Overlay())
	case !bridgeType.IsFastdp():
		iface, err := weavenet.EnsureInterface(weavenet.PcapIfName)
		checkFatal(err)
		injectorConsumer, err = weave.NewPcap(iface, bufSzMB*1024*1024) // bufsz flag is in MB
		checkFatal(err)
	}

	if !ignoreSleeve {
		sleeve := weave.NewSleeveOverlay(host, port)
		overlay.Add("sleeve", sleeve)
		overlay.SetCompatOverlay(sleeve)
	}

	return overlay, injectorConsumer
}

func createAllocator(router *weave.NetworkRouter, config ipamConfig, preClaims []ipam.PreClaim, db db.DB, track tracker.LocalRangeTracker, isKnownPeer func(mesh.PeerName) bool) (*ipam.Allocator, address.CIDR) {
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
		PreClaims:   preClaims,
		Quorum:      func() uint { return determineQuorum(config.PeerCount, router) },
		Db:          db,
		IsKnownPeer: isKnownPeer,
		Tracker:     track,
	}

	allocator := ipam.NewAllocator(c)

	gossip, err := router.NewGossip("IPallocation", allocator)
	checkFatal(err)
	allocator.SetInterfaces(gossip)
	allocator.Start()
	router.Peers.OnGC(func(peer *mesh.Peer) { allocator.PeerGone(peer.Name) })

	return allocator, defaultSubnet
}

func createDNSServer(config dnsConfig, router *mesh.Router, isKnownPeer func(mesh.PeerName) bool) (*nameserver.Nameserver, *nameserver.DNSServer) {
	ns := nameserver.New(router.Ourself.Peer.Name, config.Domain, isKnownPeer)
	router.Peers.OnGC(func(peer *mesh.Peer) { ns.PeerGone(peer.Name) })
	gossip, err := router.NewGossip("nameserver", ns)
	checkFatal(err)
	ns.SetGossip(gossip)
	upstream := nameserver.NewUpstream(config.ResolvConf, config.EffectiveListenAddress)
	dnsserver, err := nameserver.NewDNSServer(ns, config.Domain, config.ListenAddress,
		upstream, uint32(config.TTL), config.ClientTimeout)
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

func peerName(routerName, bridgeName, dbPrefix, hostRoot string) mesh.PeerName {
	if routerName == "" {
		iface, err := net.InterfaceByName(bridgeName)
		if err == nil {
			routerName = iface.HardwareAddr.String()
		} else {
			routerName, err = weavenet.GetSystemPeerName(dbPrefix, hostRoot)
			checkFatal(err)
		}
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

func listenAndServeHTTP(httpAddr string, handler http.Handler) {
	protocol := "tcp"
	if strings.HasPrefix(httpAddr, "/") {
		os.Remove(httpAddr) // in case it's there from last time
		protocol = "unix"
	}
	l, err := net.Listen(protocol, httpAddr)
	if err != nil {
		Log.Fatal("Unable to create http listener socket: ", err)
	}
	err = http.Serve(l, handler)
	if err != nil {
		Log.Fatal("Unable to create http server", err)
	}
}

func checkFatal(e error) {
	if e != nil {
		Log.Fatal(e)
	}
}
