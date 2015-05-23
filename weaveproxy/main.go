package main

import (
	"fmt"
	"net/http"

	"code.google.com/p/getopt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

var (
	defaultTarget = "unix:///var/run/docker.sock"
	defaultListen = ":12375"
)

func main() {
	var target, listen string
	var withDNS, withIPAM, debug bool

	getopt.BoolVarLong(&debug, "debug", 'd', "log debugging information")
	getopt.StringVar(&target, 'H', fmt.Sprintf("docker daemon URL to proxy (default %s)", defaultTarget))
	getopt.StringVar(&listen, 'L', fmt.Sprintf("address on which to listen (default %s)", defaultListen))
	getopt.BoolVarLong(&withDNS, "with-dns", 'w', "instruct created containers to use weaveDNS as their nameserver")
	getopt.BoolVarLong(&withIPAM, "with-ipam", 'i', "automatically allocate addresses for containers without a WEAVE_CIDR")
	getopt.Parse()

	if target == "" {
		target = defaultTarget
	}

	if listen == "" {
		listen = defaultListen
	}

	if debug {
		InitDefaultLogging(true)
	}

	p, err := proxy.NewProxy(target, withDNS, withIPAM)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}
	s := &http.Server{
		Addr:    listen,
		Handler: p,
	}

	Info.Printf("Listening on %s", listen)
	Info.Printf("Proxying %s", target)

	if err := s.ListenAndServe(); err != nil {
		Error.Fatalf("Could not listen on %s: %s", listen, err)
	}
}
