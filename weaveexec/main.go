package main

import (
	"flag"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/proxy"
	"net/http"
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
	s := &http.Server{
		Addr:    listen,
		Handler: p,
	}

	Info.Printf("Listening on %s", listen)
	Info.Printf("Proxying %s", target)

	err = s.ListenAndServe()
	if err != nil {
		Error.Fatalf("Could not listen on %s: %s", listen, err)
	}
}
