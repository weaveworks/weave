package plugin

import (
	"net"
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

const (
	pluginV2Name    = "net-plugin"
	defaultNetwork  = "weave"
	MulticastOption = netplugin.MulticastOption
)

var Log = common.Log

func Start(weaveAPIAddr string, dockerClient *docker.Client, address string, meshAddress string, dns bool, isPluginV2, forceMulticast bool, defaultSubnet string, ready func()) {
	weave := weaveapi.NewClient(weaveAPIAddr, Log)

	Log.Info("Waiting for Weave API Server...")
	weave.WaitAPIServer(30)
	Log.Info("Finished waiting for Weave API Server")

	if err := run(dockerClient, weave, address, meshAddress, dns, isPluginV2, forceMulticast, defaultSubnet, ready); err != nil {
		Log.Fatal(err)
	}
}

func run(dockerClient *docker.Client, weave *weaveapi.Client, address, meshAddress string, dns, isPluginV2, forceMulticast bool, defaultSubnet string, ready func()) error {
	endChan := make(chan error, 1)

	if address != "" {
		globalListener, err := listenAndServe(dockerClient, weave, address, endChan, "global", false, dns, isPluginV2, forceMulticast)
		if err != nil {
			return err
		}
		defer os.Remove(address)
		defer globalListener.Close()
	}
	if meshAddress != "" {
		meshListener, err := listenAndServe(dockerClient, weave, meshAddress, endChan, "local", true, dns, isPluginV2, forceMulticast)
		if err != nil {
			return err
		}
		defer os.Remove(meshAddress)
		defer meshListener.Close()
		if !isPluginV2 {
			Log.Printf("Creating default %q network", defaultNetwork)
			options := map[string]interface{}{MulticastOption: "true"}
			dockerClient.EnsureNetwork(defaultNetwork, pluginNameFromAddress(meshAddress), defaultSubnet, options)
		}
	}
	ready()

	return <-endChan
}

func listenAndServe(dockerClient *docker.Client, weave *weaveapi.Client, address string, endChan chan<- error, scope string, withIpam, dns bool, isPluginV2, forceMulticast bool) (net.Listener, error) {
	var name string
	if isPluginV2 {
		name = pluginV2Name
	} else {
		name = pluginNameFromAddress(address)
	}

	d, err := netplugin.New(dockerClient, weave, name, scope, dns, isPluginV2, forceMulticast)
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

// Take a socket address like "/run/docker/plugins/weavemesh.sock" and extract the plugin name "weavemesh"
func pluginNameFromAddress(address string) string {
	return strings.TrimSuffix(path.Base(address), ".sock")
}
