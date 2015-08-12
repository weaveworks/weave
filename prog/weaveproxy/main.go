package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/pkg/mflag"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/mflagext"
	"github.com/weaveworks/weave/proxy"
)

var (
	version = "(unreleased version)"
)

func main() {
	var (
		justVersion bool
		logLevel    = "info"
		c           = proxy.Config{ListenAddrs: []string{}}
	)

	c.Version = version

	mflag.BoolVar(&justVersion, []string{"#version", "-version"}, false, "print version and exit")
	mflag.StringVar(&logLevel, []string{"-log-level"}, "info", "logging level (debug, info, warning, error)")
	mflagext.ListVar(&c.ListenAddrs, []string{"H"}, nil, "addresses on which to listen")
	mflag.StringVar(&c.HostnameMatch, []string{"-hostname-match"}, "(.*)", "Regexp pattern to apply on container names (e.g. '^aws-[0-9]+-(.*)$')")
	mflag.StringVar(&c.HostnameReplacement, []string{"-hostname-replacement"}, "$1", "Expression to generate hostnames based on matches from --hostname-match (e.g. 'my-app-$1')")
	mflag.BoolVar(&c.NoDefaultIPAM, []string{"#-no-default-ipam", "-no-default-ipalloc"}, false, "do not automatically allocate addresses for containers without a WEAVE_CIDR")
	mflag.BoolVar(&c.NoRewriteHosts, []string{"-no-rewrite-hosts"}, false, "do not automatically rewrite /etc/hosts. Use if you need the docker IP to remain in /etc/hosts")
	mflag.StringVar(&c.TLSConfig.CACert, []string{"#tlscacert", "-tlscacert"}, "", "Trust certs signed only by this CA")
	mflag.StringVar(&c.TLSConfig.Cert, []string{"#tlscert", "-tlscert"}, "", "Path to TLS certificate file")
	mflag.BoolVar(&c.TLSConfig.Enabled, []string{"#tls", "-tls"}, false, "Use TLS; implied by --tls-verify")
	mflag.StringVar(&c.TLSConfig.Key, []string{"#tlskey", "-tlskey"}, "", "Path to TLS key file")
	mflag.BoolVar(&c.TLSConfig.Verify, []string{"#tlsverify", "-tlsverify"}, false, "Use TLS and verify the remote")
	mflag.BoolVar(&c.WithDNS, []string{"-with-dns", "w"}, false, "instruct created containers to always use weaveDNS as their nameserver")
	mflag.BoolVar(&c.WithoutDNS, []string{"-without-dns"}, false, "instruct created containers to never use weaveDNS as their nameserver")
	mflag.Parse()

	if justVersion {
		fmt.Printf("weave proxy %s\n", version)
		os.Exit(0)
	}

	if c.WithDNS && c.WithoutDNS {
		Log.Fatalf("Cannot use both '--with-dns' and '--without-dns' flags")
	}

	SetLogLevel(logLevel)

	Log.Infoln("weave proxy", version)
	Log.Infoln("Command line arguments:", strings.Join(os.Args[1:], " "))

	p, err := proxy.NewProxy(c)
	if err != nil {
		Log.Fatalf("Could not start proxy: %s", err)
	}

	go p.ListenAndServe()
	SignalHandlerLoop()
}
