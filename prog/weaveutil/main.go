/* weaveutil: collection of operations required by weave script */
package main

import (
	"fmt"
	"os"
)

var commands map[string]func([]string) error

func init() {
	commands = map[string]func([]string) error{
		"help":                   help,
		"netcheck":               netcheck,
		"docker-tls-args":        dockerTLSArgs,
		"create-datapath":        createDatapath,
		"delete-datapath":        deleteDatapath,
		"check-datapath":         checkDatapath,
		"add-datapath-interface": addDatapathInterface,
		"create-plugin-network":  createPluginNetwork,
		"remove-plugin-network":  removePluginNetwork,
		"container-addrs":        containerAddrs,
		"attach-container":       attach,
		"detach-container":       detach,
		"check-iface":            checkIface,
		"del-iface":              delIface,
		"setup-iface":            setupIface,
		"configure-arp":          configureARP,
		"list-netdevs":           listNetDevs,
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd, found := commands[os.Args[1]]
	if !found {
		usage()
		os.Exit(1)
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
}

func cmdUsage(cmd string, usage string) {
	fmt.Fprintf(os.Stderr, "usage: weaveutil %s %s\n", cmd, usage)
	os.Exit(1)
}
