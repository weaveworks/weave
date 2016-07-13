/* weaveutil: collection of operations required by weave script */
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/vishvananda/netns"

	weavenet "github.com/weaveworks/weave/net"
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
	if len(os.Args) < 2 || (len(os.Args) > 1 && os.Args[1] == "--netns-fd" && len(os.Args) < 4) {
		usage()
		os.Exit(1)
	}

	var ns netns.NsHandle
	withNetNS := false
	if os.Args[1] == "--netns-fd" {
		withNetNS = true
		fd, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot parse --netns-fd %s: %s\n", os.Args[2], err)
			os.Exit(1)
		}
		ns = netns.NsHandle(fd)
		os.Args = os.Args[3:]
	} else {
		os.Args = os.Args[1:]
	}

	cmd, found := commands[os.Args[0]]
	if !found {
		usage()
		os.Exit(1)
	}

	var err error
	work := func() error { return cmd(os.Args[1:]) }
	if withNetNS {
		err = weavenet.WithNetNSUnsafe(ns, work)
	} else {
		err = work()
	}
	if err != nil {
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
	fmt.Fprintln(os.Stderr, "usage: weaveutil [--netns-fd <fd>] <command> <arg>...")
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
