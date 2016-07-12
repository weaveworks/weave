// various fastdp operations
package main

import (
	"fmt"
	"os"
	"strconv"

	wnet "github.com/weaveworks/weave/common/net"
	"github.com/weaveworks/weave/common/odp"
)

func createDatapath(args []string) error {
	if len(args) != 2 {
		cmdUsage("create-datapath", "<datapath> <mtu>")
	}

	dpname := args[0]
	mtu, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("unable to parse mtu %q: %s", args[1], err)
	}

	odpSupported, validMTU, err := odp.CreateDatapath(dpname, mtu)
	if !odpSupported {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		// When the kernel lacks ODP support, exit with a special
		// status to distinguish it for the weave script.
		os.Exit(17)
	}
	if err != nil {
		return err
	}

	// Fallback to 1500 MTU if the user is exposed to the issue
	if mtu > 1500 && !validMTU {
		fmt.Fprintf(os.Stderr, "WARNING: Unable to set fastdp MTU to %d, possibly due to 4.3/4.4 kernel version. Setting MTU to 1500 instead.\n", mtu)
		mtu = 1500
	}

	if err := wnet.SetMTU(dpname, mtu); err != nil {
		return err
	}

	return nil
}

func deleteDatapath(args []string) error {
	if len(args) != 1 {
		cmdUsage("delete-datapath", "<datapath>")
	}
	return odp.DeleteDatapath(args[0])
}

// Checks whether a datapath can be created by actually creating and destroying it
func checkDatapath(args []string) error {
	if len(args) != 1 {
		cmdUsage("check-datapath", "<datapath>")
	}

	dpname := args[0]

	odpSupported, _, err := odp.CreateDatapath(dpname, -1)
	if err != nil {
		return err
	}
	if !odpSupported {
		return fmt.Errorf("kernel does not have ODP support")
	}

	return odp.DeleteDatapath(args[0])
}

func addDatapathInterface(args []string) error {
	if len(args) != 2 {
		cmdUsage("add-datapath-interface", "<datapath> <interface>")
	}
	return odp.AddDatapathInterface(args[0], args[1])
}
