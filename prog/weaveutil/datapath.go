// various fastdp operations
package main

import (
	"fmt"
	"os"

	"github.com/weaveworks/weave/common/odp"
)

func createDatapath(args []string) error {
	if len(args) != 1 {
		cmdUsage("create-datapath", "<datapath>")
	}
	odpSupported, err := odp.CreateDatapath(args[0])
	if !odpSupported {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		// When the kernel lacks ODP support, exit with a special
		// status to distinguish it for the weave script.
		os.Exit(17)
	}
	return err
}

func deleteDatapath(args []string) error {
	if len(args) != 1 {
		cmdUsage("delete-datapath", "<datapath>")
	}
	return odp.DeleteDatapath(args[0])
}

func addDatapathInterface(args []string) error {
	if len(args) != 2 {
		cmdUsage("add-datapath-interface", "<datapath> <interface>")
	}
	return odp.AddDatapathInterface(args[0], args[1])
}
