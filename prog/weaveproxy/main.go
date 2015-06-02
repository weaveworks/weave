package main

import (
	"fmt"
	"os"

	"code.google.com/p/getopt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

var (
	version       = "(unreleased version)"
	defaultTarget = "unix:///var/run/docker.sock"
	defaultListen = ":12375"
)

func main() {
	var target, listen string
	var withDNS, withIPAM, debug, justVersion bool

	getopt.BoolVarLong(&debug, "debug", 'd', "log debugging information")
	getopt.BoolVarLong(&justVersion, "version", 0, "print version and exit")
	getopt.StringVar(&target, 'H', fmt.Sprintf("docker daemon URL to proxy (default %s)", defaultTarget))
	getopt.StringVar(&listen, 'L', fmt.Sprintf("address on which to listen (default %s)", defaultListen))
	getopt.BoolVarLong(&withDNS, "with-dns", 'w', "instruct created containers to use weaveDNS as their nameserver")
	getopt.BoolVarLong(&withIPAM, "with-ipam", 'i', "automatically allocate addresses for containers without a WEAVE_CIDR")
	getopt.Parse()

	if justVersion {
		fmt.Printf("weave proxy %s\n", version)
		os.Exit(0)
	}

	if target == "" {
		target = defaultTarget
	}

	if listen == "" {
		listen = defaultListen
	}

	if debug {
		InitDefaultLogging(true)
	}

	p, err := proxy.NewProxy(version, target, listen, withDNS, withIPAM)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}

	Info.Printf("Listening on %s", listen)
	Info.Printf("Proxying %s", target)

	if err := p.ListenAndServe(); err != nil {
		Error.Fatalf("Could not listen on %s: %s", listen, err)
	}
}
