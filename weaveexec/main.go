package main

import (
	"flag"
	"net/http"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
)

func main() {
	var target, listen string
	var debug bool

	flag.StringVar(&target, "H", "unix:///var/run/docker.sock", "docker daemon URL to proxy")
	flag.StringVar(&listen, "L", ":12375", "address on which to listen")
	flag.BoolVar(&debug, "debug", false, "log debugging information")
	flag.Parse()

	if debug {
		InitDefaultLogging(true)
	}

	p, err := proxy.NewProxy(target)
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
