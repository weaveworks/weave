package main

import (
	"code.google.com/p/gopacket/layers"
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/davecheney/profile"
	"github.com/gorilla/mux"
	weavenet "github.com/weaveworks/weave/net"
	weave "github.com/weaveworks/weave/router"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

var version = "(unreleased version)"

func main() {

	log.SetPrefix(weave.Protocol + " ")
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	procs := runtime.NumCPU()
	// packet sniffing can block an OS thread, so we need one thread
	// for that plus at least one more.
	if procs < 2 {
		procs = 2
	}
	runtime.GOMAXPROCS(procs)

	var (
		justVersion bool
		port        int
		ifaceName   string
		routerName  string
		nickName    string
		password    string
		wait        int
		debug       bool
		prof        string
		peers       []string
		connLimit   int
		bufSz       int
		httpAddr    string
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.IntVar(&port, "port", weave.Port, "router port")
	flag.StringVar(&ifaceName, "iface", "", "name of interface to capture/inject from (disabled if blank)")
	flag.StringVar(&routerName, "name", "", "name of router (defaults to MAC of interface)")
	flag.StringVar(&nickName, "nickname", "", "nickname of peer (defaults to hostname)")
	flag.StringVar(&password, "password", "", "network password")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (0 = don't wait)")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&prof, "profile", "", "enable profiling and write profiles to given path")
	flag.IntVar(&connLimit, "connlimit", 30, "connection limit (0 for unlimited)")
	flag.IntVar(&bufSz, "bufsz", 8, "capture buffer size in MB")
	flag.StringVar(&httpAddr, "httpaddr", fmt.Sprintf(":%d", weave.HTTPPort), "address to bind HTTP interface to (disabled if blank, absolute path indicates unix domain socket)")
	flag.Parse()
	peers = flag.Args()

	if justVersion {
		fmt.Printf("weave router %s\n", version)
		os.Exit(0)
	}

	options := make(map[string]string)
	flag.Visit(func(f *flag.Flag) {
		value := f.Value.String()
		if f.Name == "password" {
			value = "<elided>"
		}
		options[f.Name] = value
	})
	log.Println("Command line options:", options)
	log.Println("Command line peers:", peers)

	var err error

	var iface *net.Interface
	if ifaceName != "" {
		iface, err = weavenet.EnsureInterface(ifaceName, wait)
		if err != nil {
			log.Fatal(err)
		}
	}

	if routerName == "" {
		if iface == nil {
			log.Fatal("Either an interface must be specified with -iface or a name with -name")
		}
		routerName = iface.HardwareAddr.String()
	}

	if nickName == "" {
		nickName, err = os.Hostname()
		if err != nil {
			log.Fatal(err)
		}
	}

	ourName, err := weave.PeerNameFromUserInput(routerName)
	if err != nil {
		log.Fatal(err)
	}

	if password == "" {
		password = os.Getenv("WEAVE_PASSWORD")
	}

	var pwSlice []byte
	if password == "" {
		log.Println("Communication between peers is unencrypted.")
	} else {
		pwSlice = []byte(password)
		log.Println("Communication between peers is encrypted.")
	}

	if prof != "" {
		p := *profile.CPUProfile
		p.ProfilePath = prof
		defer profile.Start(&p).Stop()
	}

	router := weave.NewRouter(
		weave.RouterConfig{
			Port:      port,
			Iface:     iface,
			Name:      ourName,
			NickName:  nickName,
			Password:  pwSlice,
			ConnLimit: connLimit,
			BufSz:     bufSz * 1024 * 1024,
			LogFrame:  logFrameFunc(debug)})

	log.Println("Our name is", router.Ourself.FullName())
	router.Start()
	for _, peer := range peers {
		router.ConnectionMaker.InitiateConnection(peer)
	}
	if httpAddr != "" {
		go handleHTTP(router, httpAddr)
	}
	handleSignals(router)
}

func logFrameFunc(debug bool) weave.LogFrameFunc {
	if !debug {
		return func(prefix string, frame []byte, eth *layers.Ethernet) {}
	}
	return func(prefix string, frame []byte, eth *layers.Ethernet) {
		h := fmt.Sprintf("%x", sha256.Sum256(frame))
		if eth == nil {
			log.Println(prefix, len(frame), "bytes (", h, ")")
		} else {
			log.Println(prefix, len(frame), "bytes (", h, "):", eth.SrcMAC, "->", eth.DstMAC)
		}
	}
}

func handleHTTP(router *weave.Router, httpAddr string) {
	encryption := "off"
	if router.UsingPassword() {
		encryption = "on"
	}

	muxRouter := mux.NewRouter()

	muxRouter.Methods("GET").Path("/status").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "weave router", version)
		fmt.Fprintln(w, "Encryption", encryption)
		fmt.Fprintln(w, router.Status())
	})

	muxRouter.Methods("GET").Path("/status-json").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json, _ := router.GenerateStatusJSON(version, encryption)
		w.Write(json)
	})

	muxRouter.Methods("POST").Path("/connect").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if peer := r.FormValue("peer"); peer != "" {
			router.ConnectionMaker.InitiateConnection(peer)
		} else {
			http.Error(w, "missing 'peer' param", http.StatusBadRequest)
		}
	})

	muxRouter.Methods("POST").Path("/forget").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if peer := r.FormValue("peer"); peer != "" {
			router.ConnectionMaker.ForgetConnection(peer)
		} else {
			http.Error(w, "missing 'peer' param", http.StatusBadRequest)
		}
	})

	http.Handle("/", muxRouter)

	protocol := "tcp"
	if strings.HasPrefix(httpAddr, "/") {
		os.Remove(httpAddr) // in case it's there from last time
		protocol = "unix"
	}
	l, err := net.Listen(protocol, httpAddr)
	if err != nil {
		log.Fatal("Unable to create http listener socket: ", err)
	}

	err = http.Serve(l, nil)
	if err != nil {
		log.Fatal("Unable to create http server", err)
	}
}

func handleSignals(router *weave.Router) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT, syscall.SIGUSR1)
	buf := make([]byte, 1<<20)
	for {
		sig := <-sigs
		switch sig {
		case syscall.SIGQUIT:
			stacklen := runtime.Stack(buf, true)
			log.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf[:stacklen])
		case syscall.SIGUSR1:
			log.Printf("=== received SIGUSR1 ===\n*** status...\n%s\n*** end\n", router.Status())
		}
	}
}
