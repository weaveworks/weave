package main

import (
	"flag"
	"github.com/zettio/weavedns/server"
	"log"
	"os"
)

func main() {
	log.Println(os.Args)

	var (
		ifaceName string
		apiPath   string
		dnsPort   int
		httpPort  int
		wait      int
		watch     bool
	)

	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "Path to Docker API socket")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (defaults to 0)")
	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests (defaults to 53)")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests (defaults to 6785)")
	flag.BoolVar(&watch, "watch", true, "watch the docker socket for container events")
	flag.Parse()

	var zone = new(weavedns.ZoneDb)

	if watch {
		err := weavedns.StartUpdater(apiPath, zone)
		if err != nil {
			log.Fatal("Unable to start watcher", err)
		}
	}

	err := weavedns.StartServer(zone, ifaceName, dnsPort, httpPort, wait)
	if err != nil {
		log.Fatal("Failed to start server", err)
	}
}
