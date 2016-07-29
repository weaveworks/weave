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

	"github.com/docker/docker/pkg/mflag"
	"github.com/gorilla/mux"
	"github.com/pkg/profile"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/db"
	"github.com/weaveworks/weave/nameserver"
	weave "github.com/weaveworks/weave/router"
)

var version = "(unreleased version)"

var Log = common.Log

type dnsConfig struct {
	Domain                 string
	ListenAddress          string
	TTL                    int
	ClientTimeout          time.Duration
	EffectiveListenAddress string
	ResolvConf             string
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
		logLevel           string
		prof               string
		bufSzMB            int
		noDiscovery        bool
		httpAddr           string
		peers              []string
		dnsConfig          dnsConfig
		dbPrefix           string
	)

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
	mflag.StringVar(&prof, []string{"#profile", "-profile"}, "", "enable profiling and write profiles to given path")
	mflag.IntVar(&config.ConnLimit, []string{"#connlimit", "#-connlimit", "-conn-limit"}, 30, "connection limit (0 for unlimited)")
	mflag.BoolVar(&noDiscovery, []string{"#nodiscovery", "#-nodiscovery", "-no-discovery"}, false, "disable peer discovery")
	mflag.IntVar(&bufSzMB, []string{"#bufsz", "-bufsz"}, 8, "capture buffer size in MB")
	mflag.StringVar(&httpAddr, []string{"#httpaddr", "#-httpaddr", "-http-addr"}, "", "address to bind HTTP interface to (disabled if blank, absolute path indicates unix domain socket)")
	mflag.StringVar(&dnsConfig.Domain, []string{"-dns-domain"}, nameserver.DefaultDomain, "local domain to server requests for")
	mflag.StringVar(&dnsConfig.ListenAddress, []string{"-dns-listen-address"}, nameserver.DefaultListenAddress, "address to listen on for DNS requests")
	mflag.IntVar(&dnsConfig.TTL, []string{"-dns-ttl"}, nameserver.DefaultTTL, "TTL for DNS request from our domain")
	mflag.DurationVar(&dnsConfig.ClientTimeout, []string{"-dns-fallback-timeout"}, nameserver.DefaultClientTimeout, "timeout for fallback DNS requests")
	mflag.StringVar(&dnsConfig.EffectiveListenAddress, []string{"-dns-effective-listen-address"}, "", "address DNS will actually be listening, after Docker port mapping")
	mflag.StringVar(&dnsConfig.ResolvConf, []string{"-resolv-conf"}, "", "path to resolver configuration for fallback DNS lookups")
	mflag.StringVar(&dbPrefix, []string{"-db-prefix"}, "/weavedb/weave", "pathname/prefix of filename to store data")

	mflag.Parse()

	peers = mflag.Args()

	Log.Println("Peers:", peers)
	if resume && len(peers) > 0 {
		Log.Fatalf("You must not specify an initial peer list in conjunction with --resume")
	}

	common.SetLogLevel(logLevel)

	if justVersion {
		fmt.Printf("weave router %s\n", version)
		os.Exit(0)
	}

	Log.Println("Command line options:", options())

	if prof != "" {
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(prof), profile.NoShutdownHook).Stop()
	}

	if protocolMinVersion < mesh.ProtocolMinVersion || protocolMinVersion > mesh.ProtocolMaxVersion {
		Log.Fatalf("--min-protocol-version must be in range [%d,%d]", mesh.ProtocolMinVersion, mesh.ProtocolMaxVersion)
	}
	config.ProtocolMinVersion = byte(protocolMinVersion)

	name := peerName(routerName, nil)

	if nickName == "" {
		var err error
		nickName, err = os.Hostname()
		checkFatal(err)
	}

	config.Password = determinePassword(password)
	config.PeerDiscovery = !noDiscovery

	db, err := db.NewBoltDB(dbPrefix + "data.db")
	checkFatal(err)
	defer db.Close()

	router := weave.NewNetworkRouter(config, networkConfig, name, nickName, nil, db)
	Log.Println("Our name is", router.Ourself)

	if peers, err = router.InitialPeers(resume, peers); err != nil {
		Log.Fatal("Unable to get initial peer set: ", err)
	}

	isKnownPeer := func(name mesh.PeerName) bool {
		return router.Peers.Fetch(name) != nil
	}

	_peers := router.ConnectionMaker.Targets(true)
	Log.Println("router.ConnectionMaker.Targets(true) =>", _peers)

	var (
		ns        *nameserver.Nameserver
		dnsserver *nameserver.DNSServer
	)

	ns, dnsserver = createDNSServer(dnsConfig, router.Router, isKnownPeer)
	//observeContainers(ns)
	ns.Start()
	defer ns.Stop()
	dnsserver.ActivateAndServe()
	defer dnsserver.Stop()

	router.Start()
	if errors := router.InitiateConnections(peers, false); len(errors) > 0 {
		Log.Fatal(common.ErrorMessages(errors))
	}

	// The weave script always waits for a status call to succeed,
	// so there is no point in doing "weave launch --http-addr ''".
	// This is here to support stand-alone use of weaver.
	if httpAddr != "" {
		muxRouter := mux.NewRouter()
		if ns != nil {
			ns.HandleHTTP(muxRouter, nil)
		}
		router.HandleHTTP(muxRouter)
		HandleHTTP(muxRouter, version, router, ns, dnsserver)
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

func createDNSServer(config dnsConfig, router *mesh.Router, isKnownPeer func(mesh.PeerName) bool) (*nameserver.Nameserver, *nameserver.DNSServer) {
	ns := nameserver.New(router.Ourself.Peer.Name, config.Domain, isKnownPeer)
	router.Peers.OnGC(func(peer *mesh.Peer) { ns.PeerGone(peer.Name) })
	ns.SetGossip(router.NewGossip("nameserver", ns))
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
