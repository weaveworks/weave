/* weaveutil: collection of operations required by weave script */
package main

import (
	"fmt"
	"os"
	"strings"
)

var commands map[string]func([]string) error

func init() {
	commands = map[string]func([]string) error{
		"help":                     help,
		"netcheck":                 netcheck,
		"docker-tls-args":          dockerTLSArgs,
		"detect-bridge-type":       detectBridgeType,
		"create-datapath":          createDatapath,
		"delete-datapath":          deleteDatapath,
		"check-datapath":           checkDatapath,
		"add-datapath-interface":   addDatapathInterface,
		"remove-plugin-network":    removePluginNetwork,
		"container-addrs":          containerAddrs,
		"process-addrs":            processAddrs,
		"container-id":             containerID,
		"container-state":          containerState,
		"container-fqdn":           containerFQDN,
		"run-container":            runContainer,
		"list-containers":          listContainers,
		"stop-container":           stopContainer,
		"kill-container":           killContainer,
		"remove-container":         removeContainer,
		"create-volume-container":  createVolumeContainer,
		"attach-container":         attach,
		"detach-container":         detach,
		"cni-net":                  cniNet,
		"cni-ipam":                 cniIPAM,
		"bridge-ip":                bridgeIP,
		"unique-id":                uniqueID,
		"swarm-manager-peers":      swarmManagerPeers,
		"is-docker-plugin-enabled": isDockerPluginEnabled,
		"rewrite-etc-hosts":        rewriteEtcHosts,
		"get-db-flag":              getDBFlag,
		"set-db-flag":              setDBFlag,
	}
}

func main() {
	// If no args passed, act as CNI plugin based on executable name
	switch {
	case len(os.Args) == 1 && strings.HasSuffix(os.Args[0], "weave-ipam"):
		cniIPAM(os.Args)
		os.Exit(0)
	case len(os.Args) == 1 && strings.HasSuffix(os.Args[0], "weave-net"):
		cniNet(os.Args)
		os.Exit(0)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd, found := commands[os.Args[1]]
	if !found {
		fmt.Fprintf(os.Stderr, "%q cmd is not found\n", os.Args[1])
		usage()
		os.Exit(1)
	}
	if err := cmd(os.Args[2:]); err != nil {
		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func help(args []string) error {
	if len(args) > 0 {
		cmdUsage("help", "")
	}
	usage()
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: weaveutil <command> <arg>...")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "where <command> is one of:")
	fmt.Fprintln(os.Stderr)
	for cmd := range commands {
		fmt.Fprintln(os.Stderr, cmd)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Set an environment variable called DOCKER_API_VERSION to control the API version used.\nDefault is: %v\n", defaulDockerAPIVersion)
}

func cmdUsage(cmd string, usage string) {
	fmt.Fprintf(os.Stderr, "usage: weaveutil %s %s\n", cmd, usage)
	os.Exit(1)
}
