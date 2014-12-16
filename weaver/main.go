package main

import (
	"code.google.com/p/gopacket/layers"
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/davecheney/profile"
	lg "github.com/zettio/weave/logging"
	weavenet "github.com/zettio/weave/net"
	weave "github.com/zettio/weave/router"
	"github.com/zettio/weave/sortinghat"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {

	log.SetPrefix(weave.Protocol + " ")
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println(os.Args)

	procs := runtime.NumCPU()
	// packet sniffing can block an OS thread, so we need one thread
	// for that plus at least one more.
	if procs < 2 {
		procs = 2
	}
	runtime.GOMAXPROCS(procs)

	var (
		ifaceName  string
		routerName string
		password   string
		wait       int
		debug      bool
		pktdebug   bool
		prof       string
		peers      []string
		connLimit  int
		bufSz      int
	)

	flag.StringVar(&ifaceName, "iface", "", "name of interface to read from")
	flag.StringVar(&routerName, "name", "", "name of router (defaults to MAC)")
	flag.StringVar(&password, "password", "", "network password")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (defaults to 0, i.e. don't wait)")
	flag.BoolVar(&pktdebug, "pktdebug", false, "enable per-packet debug logging")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&prof, "profile", "", "enable profiling and write profiles to given path")
	flag.IntVar(&connLimit, "connlimit", 10, "connection limit (defaults to 10, set to 0 for unlimited)")
	flag.IntVar(&bufSz, "bufsz", 8, "capture buffer size in MB (defaults to 8MB)")
	flag.Parse()
	peers = flag.Args()

	lg.InitDefault(debug)

	if ifaceName == "" {
		fmt.Println("Missing required parameter 'iface'")
		os.Exit(1)
	}
	iface, err := weavenet.EnsureInterface(ifaceName, wait)
	if err != nil {
		log.Fatal(err)
	}

	if connLimit < 0 {
		connLimit = 0
	}

	if routerName == "" {
		routerName = iface.HardwareAddr.String()
	}

	ourName, err := weave.PeerNameFromUserInput(routerName)
	if err != nil {
		log.Fatal(err)
	}

	var logFrame func(string, []byte, *layers.Ethernet)
	if pktdebug {
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

	router := weave.NewRouter(iface, ourName, []byte(password), connLimit, bufSz*1024*1024, logFrame)
	router.Start()
	for _, peer := range peers {
		if addr, err := net.ResolveTCPAddr("tcp4", weave.NormalisePeerAddr(peer)); err == nil {
			router.ConnectionMaker.InitiateConnection(addr.String())
		} else {
			log.Fatal(err)
		}
	}
	allocator := sortinghat.NewAllocator(ourName, router.Ourself, net.ParseIP("10.0.1.1"), 253)
	router.GossipDelegate = allocator
	allocator.HandleHttp()
	go handleHttp(router, allocator)
	handleSignals(router)
}

func handleHttp(router *weave.Router, alloc *sortinghat.Allocator) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, router.Status())
		io.WriteString(w, fmt.Sprintln(alloc))
	})
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		peer := r.FormValue("peer")
		if addr, err := net.ResolveTCPAddr("tcp4", weave.NormalisePeerAddr(peer)); err == nil {
			router.ConnectionMaker.InitiateConnection(addr.String())
		} else {
			http.Error(w, fmt.Sprint("invalid peer address: ", err), http.StatusBadRequest)
		}
	})
	address := fmt.Sprintf(":%d", weave.HttpPort)
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
			runtime.Stack(buf, true)
			log.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf)
		case syscall.SIGUSR1:
			log.Printf("=== received SIGUSR1 ===\n*** status...\n%s\n*** end\n", router.Status())
		}
	}
}
