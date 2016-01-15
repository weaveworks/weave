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
		"add-datapath-interface": addDatapathInterface,
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
