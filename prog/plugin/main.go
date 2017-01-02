package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docker/libnetwork/ipamapi"
	go_docker "github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	ipamplugin "github.com/weaveworks/weave/plugin/ipam"
	netplugin "github.com/weaveworks/weave/plugin/net"
	"github.com/weaveworks/weave/plugin/skel"
)

var version = "(unreleased version)"

func main() {
	var (
		justVersion      bool
		address          string
		nameserver       string
		meshAddress      string
		logLevel         string
		meshNetworkName  string
		noMulticastRoute bool
		removeNetwork    bool
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.StringVar(&logLevel, "log-level", "info", "logging level (debug, info, warning, error)")
	flag.StringVar(&address, "socket", "/run/docker/plugins/weave.sock", "socket on which to listen")
	flag.StringVar(&nameserver, "nameserver", "", "nameserver to provide to containers")
	flag.StringVar(&meshAddress, "meshsocket", "/run/docker/plugins/weavemesh.sock", "socket on which to listen in mesh mode")
	flag.StringVar(&meshNetworkName, "mesh-network-name", "weave", "network name to create in mesh mode")
	flag.BoolVar(&noMulticastRoute, "no-multicast-route", false, "do not add a multicast route to network endpoints")
	flag.BoolVar(&removeNetwork, "remove-network", false, "remove mesh network and exit")

	flag.Parse()

	if justVersion {
		fmt.Printf("weave plugin %s\n", version)
		os.Exit(0)
	}

	SetLogLevel(logLevel)

	// API 1.21 is the first version that supports docker network commands
	dockerClient, err := docker.NewVersionedClientFromEnv("1.21")
	if err != nil {
		Log.Fatalf("unable to connect to docker: %s", err)
	}

	if removeNetwork {
		if _, err = dockerClient.Client.NetworkInfo(meshNetworkName); err == nil {
			err = dockerClient.Client.RemoveNetwork(meshNetworkName)
			if err != nil {
				Log.Fatalf("unable to remove network: %s", err)
			}
		}
		os.Exit(0)
	}

	Log.Println("Weave plugin", version, "Command line options:", os.Args[1:])
	Log.Info(dockerClient.Info())

	err = run(dockerClient, address, meshAddress, meshNetworkName, nameserver, noMulticastRoute)
	if err != nil {
		Log.Fatal(err)
	}
}

func run(dockerClient *docker.Client, address, meshAddress, meshNetworkName, nameserver string, noMulticastRoute bool) error {
	endChan := make(chan error, 1)
	if address != "" {
		globalListener, err := listenAndServe(dockerClient, address, nameserver, noMulticastRoute, endChan, "global", false)
		if err != nil {
			return err
		}
		defer os.Remove(address)
		defer globalListener.Close()
	}
	if meshAddress != "" {
		meshListener, err := listenAndServe(dockerClient, meshAddress, nameserver, noMulticastRoute, endChan, "local", true)
		if err != nil {
			return err
		}
		defer os.Remove(meshAddress)
		defer meshListener.Close()
	}

	if meshNetworkName != "" {
		if err := createNetwork(dockerClient, meshNetworkName, meshAddress); err != nil {
			return err
		}
	}

	go listenAndServeStatus("/home/weave/status.sock")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		Log.Debugf("Caught signal %s; shutting down", sig)
		return nil
	case err := <-endChan:
		return err
	}
}

func listenAndServe(dockerClient *docker.Client, address, nameserver string, noMulticastRoute bool, endChan chan<- error, scope string, withIpam bool) (net.Listener, error) {
	d, err := netplugin.New(dockerClient, version, nameserver, scope, noMulticastRoute)
	if err != nil {
		return nil, err
	}

	var i ipamapi.Ipam
	if withIpam {
		if i, err = ipamplugin.NewIpam(dockerClient, version); err != nil {
			return nil, err
		}
	}

	var listener net.Listener

	// remove sockets from last invocation
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	listener, err = net.Listen("unix", address)
	if err != nil {
		return nil, err
	}
	Log.Printf("Listening on %s for %s scope", address, scope)

	go func() {
		endChan <- skel.Listen(listener, d, i)
	}()

	return listener, nil
}

func createNetwork(dockerClient *docker.Client, networkName, address string) error {
	if _, err := dockerClient.Client.NetworkInfo(networkName); err == nil {
		Log.Printf("Docker network '%s' already exists", networkName)
	} else if _, ok := err.(*go_docker.NoSuchNetwork); ok {
		driverName := strings.TrimSuffix(address, ".sock")
		if i := strings.LastIndex(driverName, "/"); i >= 0 {
			driverName = driverName[i+1:]
		}
		options := go_docker.CreateNetworkOptions{
			Name:           networkName,
			CheckDuplicate: true,
			Driver:         driverName,
			IPAM:           go_docker.IPAMOptions{Driver: driverName},
		}
		_, err := dockerClient.Client.CreateNetwork(options)
		if err != nil {
			return fmt.Errorf("Error creating network: %s", err)
		}
	}
	return nil
}

func listenAndServeStatus(socket string) {
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		Log.Fatalf("Error removing existing status socket: %s", err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		Log.Fatalf("ListenAndServeStatus failed: %s", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	if err := (&http.Server{Handler: handler}).Serve(listener); err != nil {
		Log.Fatalf("ListenAndServeStatus failed: %s", err)
	}
}
