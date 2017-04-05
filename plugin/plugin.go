package plugin

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/docker/libnetwork/ipamapi"
	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	weavenet "github.com/weaveworks/weave/net"
	ipamplugin "github.com/weaveworks/weave/plugin/ipam"
	netplugin "github.com/weaveworks/weave/plugin/net"
	"github.com/weaveworks/weave/plugin/skel"
)

var Log = common.Log

func Start(weaveAPIAddr string, dockerClient *docker.Client, address string, meshAddress string) {
	weave := weaveapi.NewClient(weaveAPIAddr, Log)

	Log.Info("Waiting for Weave API Server...")
	weave.WaitAPIServer(30)
	Log.Info("Finished waiting for Weave API Server")

	if err := run(dockerClient, weave, address, meshAddress); err != nil {
		Log.Fatal(err)
	}
}

func run(dockerClient *docker.Client, weave *weaveapi.Client, address, meshAddress string) error {
	endChan := make(chan error, 1)

	if address != "" {
		globalListener, err := listenAndServe(dockerClient, weave, address, endChan, "global", false)
		if err != nil {
			return err
		}
		defer os.Remove(address)
		defer globalListener.Close()
	}
	if meshAddress != "" {
		meshListener, err := listenAndServe(dockerClient, weave, meshAddress, endChan, "local", true)
		if err != nil {
			return err
		}
		defer os.Remove(meshAddress)
		defer meshListener.Close()
	}

	statusListener, err := weavenet.ListenUnixSocket("/home/weave/plugin-status.sock")
	if err != nil {
		return err
	}
	go serveStatus(statusListener)

	return <-endChan
}

func listenAndServe(dockerClient *docker.Client, weave *weaveapi.Client, address string, endChan chan<- error, scope string, withIpam bool) (net.Listener, error) {
	name := strings.TrimSuffix(path.Base(address), ".sock")
	d, err := netplugin.New(dockerClient, weave, name, scope)
	if err != nil {
		return nil, err
	}

	var i ipamapi.Ipam
	if withIpam {
		i = ipamplugin.NewIpam(weave)
	}

	listener, err := weavenet.ListenUnixSocket(address)
	if err != nil {
		return nil, err
	}
	Log.Printf("Listening on %s for %s scope", address, scope)

	go func() {
		endChan <- skel.Listen(listener, d, i)
	}()

	return listener, nil
}

func serveStatus(listener net.Listener) {
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})}
	if err := server.Serve(listener); err != nil {
		Log.Fatalf("ListenAndServeStatus failed: %s", err)
	}
}
