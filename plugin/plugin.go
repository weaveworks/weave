package plugin

import (
	"net"
	"os"
	"path"
	"strings"

	"github.com/docker/libnetwork/ipamapi"
	weaveapi "github.com/rajch/weave/api"
	"github.com/rajch/weave/common"
	"github.com/rajch/weave/common/docker"
	weavenet "github.com/rajch/weave/net"
	ipamplugin "github.com/rajch/weave/plugin/ipam"
	netplugin "github.com/rajch/weave/plugin/net"
	"github.com/rajch/weave/plugin/skel"
)

const (
	pluginV2Name    = "net-plugin"
	defaultNetwork  = "weave"
	MulticastOption = netplugin.MulticastOption
)

var Log = common.Log

type Config struct {
	Socket            string
	MeshSocket        string
	Enable            bool
	EnableV2          bool
	EnableV2Multicast bool
	DNS               bool
	DefaultSubnet     string
	ProcPath          string // path to reach host /proc filesystem
}

type Plugin struct {
	Config
}

func NewPlugin(config Config) *Plugin {
	if !config.Enable && !config.EnableV2 {
		return nil
	}
	plugin := &Plugin{Config: config}
	return plugin
}

func (plugin *Plugin) Start(weaveAPIAddr string, dockerClient *docker.Client, ready func()) {
	weave := weaveapi.NewClient(weaveAPIAddr, Log)

	Log.Info("Waiting for Weave API Server...")
	weave.WaitAPIServer(30)
	Log.Info("Finished waiting for Weave API Server")

	if err := plugin.run(dockerClient, weave, ready); err != nil {
		Log.Fatal(err)
	}
}

func (plugin *Plugin) run(dockerClient *docker.Client, weave *weaveapi.Client, ready func()) error {
	endChan := make(chan error, 1)

	if plugin.Socket != "" {
		globalListener, err := listenAndServe(dockerClient, weave, plugin.Socket, endChan, "global", false, plugin.DNS, plugin.EnableV2, plugin.EnableV2Multicast, plugin.ProcPath)
		if err != nil {
			return err
		}
		defer os.Remove(plugin.Socket)
		defer globalListener.Close()
	}
	if plugin.MeshSocket != "" {
		meshListener, err := listenAndServe(dockerClient, weave, plugin.MeshSocket, endChan, "local", true, plugin.DNS, plugin.EnableV2, plugin.EnableV2Multicast, plugin.ProcPath)
		if err != nil {
			return err
		}
		defer os.Remove(plugin.MeshSocket)
		defer meshListener.Close()
		if !plugin.EnableV2 {
			Log.Printf("Creating default %q network", defaultNetwork)
			options := map[string]interface{}{MulticastOption: "true"}
			dockerClient.EnsureNetwork(defaultNetwork, pluginNameFromAddress(plugin.MeshSocket), plugin.DefaultSubnet, options)
		}
	}
	ready()

	return <-endChan
}

func listenAndServe(dockerClient *docker.Client, weave *weaveapi.Client, address string, endChan chan<- error, scope string, withIpam, dns bool, isPluginV2, forceMulticast bool, procPath string) (net.Listener, error) {
	var name string
	if isPluginV2 {
		name = pluginV2Name
	} else {
		name = pluginNameFromAddress(address)
	}

	d, err := netplugin.New(dockerClient, weave, name, scope, dns, isPluginV2, forceMulticast, procPath)
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
