package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/plugin/net"
	"github.com/weaveworks/weave/plugin/skel"
)

var version = "(unreleased version)"

func main() {
	var (
		justVersion bool
		address     string
		nameserver  string
		logLevel    string
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&logLevel, "log-level", "info", "logging level (debug, info, warning, error)")
	flag.StringVar(&address, "socket", "/run/docker/plugins/weave.sock", "socket on which to listen")
	flag.StringVar(&nameserver, "nameserver", "", "nameserver to provide to containers")

	flag.Parse()

	if justVersion {
		fmt.Printf("weave plugin %s\n", version)
		os.Exit(0)
	}

	SetLogLevel(logLevel)

	Log.Println("Weave plugin", version, "Command line options:", os.Args)

	var d skel.Driver
	d, err := plugin.New(version, nameserver)
	if err != nil {
		Log.Fatalf("unable to create driver: %s", err)
	}

	var listener net.Listener

	// remove socket from last invocation
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		Log.Fatal(err)
	}
	listener, err = net.Listen("unix", address)
	if err != nil {
		Log.Fatal(err)
	}
	defer listener.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	endChan := make(chan error, 1)
	go func() {
		endChan <- skel.Listen(listener, d)
	}()

	select {
	case sig := <-sigChan:
		Log.Debugf("Caught signal %s; shutting down", sig)
	case err := <-endChan:
		if err != nil {
			Log.Errorf("Error from listener: %s", err)
			listener.Close()
			os.Exit(1)
		}
	}
}
