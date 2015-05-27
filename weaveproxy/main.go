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
	var (
		debug       bool
		justVersion bool
		listen      string
		target      string
		tlsCACert   string
		tlsCert     string
		tlsEnabled  bool
		tlsKey      string
		tlsVerify   bool
		withDNS     bool
		withIPAM    bool
	)

	getopt.BoolVarLong(&debug, "debug", 'd', "log debugging information")
	getopt.BoolVarLong(&justVersion, "version", 0, "print version and exit")
	getopt.StringVar(&listen, 'L', fmt.Sprintf("address on which to listen (default %s)", defaultListen))
	getopt.StringVar(&target, 'H', fmt.Sprintf("docker daemon URL to proxy (default %s)", defaultTarget))
	getopt.StringVarLong(&tlsCACert, "tlscacert", 0, "Trust certs signed only by this CA")
	getopt.StringVarLong(&tlsCert, "tlscert", 0, "Path to TLS certificate file")
	getopt.BoolVarLong(&tlsEnabled, "tls", 0, "Use TLS; implied by --tlsverify")
	getopt.StringVarLong(&tlsKey, "tlskey", 0, "Path to TLS key file")
	getopt.BoolVarLong(&tlsVerify, "tlsverify", 0, "Use TLS and verify the remote")
	getopt.BoolVarLong(&withDNS, "with-dns", 'w', "instruct created containers to use weaveDNS as their nameserver")
	getopt.BoolVarLong(&withIPAM, "with-ipam", 'i', "automatically allocate addresses for containers without a WEAVE_CIDR")
	getopt.Parse()

	if justVersion {
		fmt.Printf("weave proxy %s\n", version)
		os.Exit(0)
	}

	if debug {
		InitDefaultLogging(true)
	}

	if target == "" {
		target = defaultTarget
	}

	if listen == "" {
		listen = defaultListen
	}

	p, err := proxy.NewProxy(
		version,
		target,
		listen,
		withDNS,
		withIPAM,
		&proxy.TLSConfig{
			Enabled: tlsEnabled,
			Verify:  tlsVerify,
			Cert:    tlsCert,
			Key:     tlsKey,
			CACert:  tlsCACert,
		},
	)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}

	if err := p.ListenAndServe(); err != nil {
		Error.Fatalf("Could not listen on %s: %s", listen, err)
	}
}
