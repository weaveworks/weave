package main

import (
	"fmt"
	"os"
	"strings"

	"code.google.com/p/getopt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

var (
	version            = "(unreleased version)"
	defaultListenAddrs = []string{"tcp://0.0.0.0:12375", "unix:///var/run/weave.sock"}
)

func main() {
	var (
		debug       bool
		justVersion bool
		c           = proxy.Config{ListenAddrs: defaultListenAddrs}
	)

	c.Version = version
	getopt.BoolVarLong(&debug, "debug", 'd', "log debugging information")
	getopt.BoolVarLong(&justVersion, "version", 0, "print version and exit")
	getopt.ListVar(&c.ListenAddrs, 'H', fmt.Sprintf("address on which to listen (default %s)", defaultListenAddrs))
	getopt.BoolVarLong(&c.NoDefaultIPAM, "no-default-ipam", 0, "do not automatically allocate addresses for containers without a WEAVE_CIDR")
	getopt.StringVarLong(&c.TLSConfig.CACert, "tlscacert", 0, "Trust certs signed only by this CA")
	getopt.StringVarLong(&c.TLSConfig.Cert, "tlscert", 0, "Path to TLS certificate file")
	getopt.BoolVarLong(&c.TLSConfig.Enabled, "tls", 0, "Use TLS; implied by --tlsverify")
	getopt.StringVarLong(&c.TLSConfig.Key, "tlskey", 0, "Path to TLS key file")
	getopt.BoolVarLong(&c.TLSConfig.Verify, "tlsverify", 0, "Use TLS and verify the remote")
	getopt.BoolVarLong(&c.WithDNS, "with-dns", 'w', "instruct created containers to always use weaveDNS as their nameserver")
	getopt.BoolVarLong(&c.WithoutDNS, "without-dns", 0, "instruct created containers to never use weaveDNS as their nameserver")
	getopt.Parse()

	if justVersion {
		fmt.Printf("weave proxy %s\n", version)
		os.Exit(0)
	}

	if c.WithDNS && c.WithoutDNS {
		Error.Fatalf("Cannot use both '--with-dns' and '--without-dns' flags")
	}

	if debug {
		InitDefaultLogging(true)
	}

	Info.Println("weave proxy", version)
	Info.Println("Command line arguments:", strings.Join(os.Args[1:], " "))

	p, err := proxy.NewProxy(c)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}

	p.ListenAndServe()
}
