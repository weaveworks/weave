package main

import (
	"code.google.com/p/gopacket/layers"
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/davecheney/profile"
	"github.com/gorilla/mux"
	weavenet "github.com/zettio/weave/net"
	weave "github.com/zettio/weave/router"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
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
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&ifaceName, "iface", "", "name of interface to read from")
	flag.StringVar(&routerName, "name", "", "name of router (defaults to MAC)")
	flag.StringVar(&nickName, "nickname", "", "nickname of peer (defaults to hostname)")
	flag.StringVar(&password, "password", "", "network password")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (defaults to 0, i.e. don't wait)")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&prof, "profile", "", "enable profiling and write profiles to given path")
	flag.IntVar(&connLimit, "connlimit", 10, "connection limit (defaults to 10, set to 0 for unlimited)")
	flag.IntVar(&bufSz, "bufsz", 8, "capture buffer size in MB (defaults to 8MB)")
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

	if ifaceName == "" {
		fmt.Println("Missing required parameter 'iface'")
		os.Exit(1)
	}
	iface, err := weavenet.EnsureInterface(ifaceName, wait)
	if err != nil {
		log.Fatal(err)
	}

	if routerName == "" {
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

	var logFrame func(string, []byte, *layers.Ethernet)
	if debug {
		logFrame = func(prefix string, frame []byte, eth *layers.Ethernet) {
			h := fmt.Sprintf("%x", sha256.Sum256(frame))
			if eth == nil {
				log.Println(prefix, len(frame), "bytes (", h, ")")
			} else {
				log.Println(prefix, len(frame), "bytes (", h, "):", eth.SrcMAC, "->", eth.DstMAC)
			}
		}
	} else {
		logFrame = func(prefix string, frame []byte, eth *layers.Ethernet) {}
	}

	if prof != "" {
		p := *profile.CPUProfile
		p.ProfilePath = prof
		defer profile.Start(&p).Stop()
	}

	router := weave.NewRouter(iface, ourName, nickName, pwSlice, connLimit, bufSz*1024*1024, logFrame)
	log.Println("Our name is", router.Ourself.Name, "("+router.Ourself.NickName+")")
	router.Start()
	for _, peer := range peers {
		if addr, err := net.ResolveTCPAddr("tcp4", weave.NormalisePeerAddr(peer)); err == nil {
			router.ConnectionMaker.InitiateConnection(addr.String())
		} else {
			log.Fatal(err)
		}
	}
	go handleHTTP(router)
	handleSignals(router)
}

func handleHTTP(router *weave.Router) {
	encryption := "off"
	if router.UsingPassword() {
		encryption = "on"
	}

	muxRouter := mux.NewRouter()

	muxRouter.Methods("GET").Path("/status").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln("weave router", version))
		io.WriteString(w, fmt.Sprintln("Encryption", encryption))
		io.WriteString(w, router.Status())
	})

	muxRouter.Methods("GET").Path("/status-json").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json, _ := router.GenerateStatusJSON(version, encryption)
		w.Write(json)
	})

	muxRouter.Methods("POST").Path("/connect").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := r.FormValue("peer")
		if addr, err := net.ResolveTCPAddr("tcp4", weave.NormalisePeerAddr(peer)); err == nil {
			router.ConnectionMaker.InitiateConnection(addr.String())
		} else {
			http.Error(w, fmt.Sprint("invalid peer address: ", err), http.StatusBadRequest)
		}
	})

	http.Handle("/", muxRouter)

	address := fmt.Sprintf(":%d", weave.HTTPPort)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal("Unable to create http listener: ", err)
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
