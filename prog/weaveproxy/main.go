package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pborman/getopt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

var (
	version           = "(unreleased version)"
	defaultDockerAddr = "unix:///var/run/docker.sock"
	defaultListenAddr = "tcp://0.0.0.0:12375"
)

func main() {
	var (
		debug       bool
		justVersion bool
		c           = proxy.Config{
			DockerAddr: defaultDockerAddr,
			ListenAddr: defaultListenAddr,
		}
	)

	c.Version = version
	getopt.BoolVarLong(&debug, "debug", 'd', "log debugging information")
	getopt.BoolVarLong(&justVersion, "version", 0, "print version and exit")
	getopt.StringVar(&c.ListenAddr, 'H', fmt.Sprintf("address on which to listen (default %s)", defaultListenAddr))
	getopt.StringVar(&c.DockerAddr, 'D', fmt.Sprintf("docker daemon URL to proxy (default %s)", defaultDockerAddr))
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

	protoAddrParts := strings.SplitN(c.ListenAddr, "://", 2)
	if len(protoAddrParts) == 2 {
		if protoAddrParts[0] != "tcp" {
			Error.Fatalf("Invalid protocol format: %q", protoAddrParts[0])
		}
		c.ListenAddr = protoAddrParts[1]
	} else {
		c.ListenAddr = protoAddrParts[0]
	}

	p, err := proxy.NewProxy(c)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}

	if err := p.ListenAndServe(); err != nil {
		Error.Fatalf("Could not listen on %s: %s", p.ListenAddr, err)
	}
}
