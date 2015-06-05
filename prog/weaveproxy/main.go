package main

import (
	"fmt"
	"os"

	"code.google.com/p/getopt"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

var (
	version           = "(unreleased version)"
	defaultDockerAddr = "unix:///var/run/docker.sock"
	defaultListenAddr = ":12375"
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
	getopt.StringVar(&c.ListenAddr, 'L', fmt.Sprintf("address on which to listen (default %s)", defaultListenAddr))
	getopt.StringVar(&c.DockerAddr, 'H', fmt.Sprintf("docker daemon URL to proxy (default %s)", defaultDockerAddr))
	getopt.StringVarLong(&c.TLSConfig.CACert, "tlscacert", 0, "Trust certs signed only by this CA")
	getopt.StringVarLong(&c.TLSConfig.Cert, "tlscert", 0, "Path to TLS certificate file")
	getopt.BoolVarLong(&c.TLSConfig.Enabled, "tls", 0, "Use TLS; implied by --tlsverify")
	getopt.StringVarLong(&c.TLSConfig.Key, "tlskey", 0, "Path to TLS key file")
	getopt.BoolVarLong(&c.TLSConfig.Verify, "tlsverify", 0, "Use TLS and verify the remote")
	getopt.BoolVarLong(&c.WithDNS, "with-dns", 'w', "instruct created containers to use weaveDNS as their nameserver")
	getopt.BoolVarLong(&c.WithIPAM, "with-ipam", 'i', "automatically allocate addresses for containers without a WEAVE_CIDR")
	getopt.Parse()

	if justVersion {
		fmt.Printf("weave proxy %s\n", version)
		os.Exit(0)
	}

	if debug {
		InitDefaultLogging(true)
	}

	p, err := proxy.NewProxy(c)
	if err != nil {
		Error.Fatalf("Could not start proxy: %s", err)
	}

	if err := p.ListenAndServe(); err != nil {
		Error.Fatalf("Could not listen on %s: %s", p.ListenAddr, err)
	}
}
