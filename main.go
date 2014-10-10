package main

import (
	"flag"
	"github.com/zettio/weavedns/server"
	"log"
)

func main() {
	var (
		ifaceName string
		dnsPort   int
		httpPort  int
		wait      int
	)

	flag.StringVar(&ifaceName, "iface", "", "name of interface to use for multicast")
	flag.IntVar(&wait, "wait", 0, "number of seconds to wait for interface to be created and come up (defaults to 0)")
	flag.IntVar(&dnsPort, "dnsport", 53, "port to listen to dns requests (defaults to 53)")
	flag.IntVar(&httpPort, "httpport", 6785, "port to listen to HTTP requests (defaults to 6785)")
	flag.Parse()

	err := weavedns.StartServer(ifaceName, dnsPort, httpPort, wait)
	if err != nil {
		log.Fatalf("Failed to start server: %s", err)
	}
}
