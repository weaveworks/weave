// various fastdp operations
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"

	libodp "github.com/weaveworks/go-odp/odp"
	"github.com/weaveworks/weave/common/odp"
	wnet "github.com/weaveworks/weave/net"
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

	odpSupported, err := odp.CreateDatapath(dpname)
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

	if mtu > 1500 {
		// Check whether a host is exposed to https://github.com/weaveworks/weave/issues/1853
		// If yes, fallback to 1500 MTU.
		if !checkMTU(dpname, mtu) {
			fmt.Fprintf(os.Stderr, "WARNING: Unable to set fastdp MTU to %d, possibly due to 4.3/4.4 kernel version. Setting MTU to 1500 instead.\n", mtu)
			mtu = 1500
		}
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

	odpSupported, err := odp.CreateDatapath(dpname)
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

// Helpers

func checkMTU(dpname string, mtu int) bool {
	var (
		vpid   libodp.VportID
		vpname string
	)

	// Create a dummy vxlan vport
	for i := 0; i < 5; i++ {
		portno, err := getUDPPortNo()
		if err != nil {
			return true
		}
		if vpid, vpname, err = odp.CreateVxlanVport(dpname, "vxlantest", portno); err == nil {
			defer func() {
				if err := odp.DeleteVport(dpname, vpid); err != nil {
					fmt.Fprintf(os.Stderr, "unable to remove unused %q vxlan vport: %s\n", vpname, err)
				}
			}()
			break
		} else if errno, ok := err.(syscall.Errno); !(ok && errno == syscall.EADDRINUSE) {
			return true
		}
	}
	// Couldn't create the vport, skip the check
	if vpname == "" {
		return true
	}

	// Setting >1500 MTU on affected host' vport should fail with EINVAL
	if err := wnet.SetMTU(vpname, mtu); err != nil {
		if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINVAL {
			return false
		}
		// NB: If no link interface for the vport is found (which
		// might be a case for SetMTU to fail), the user is probably
		// running the <= 4.2 kernel, which is fine.
	}

	return true
}

// A dummy way to get an ephemeral port for UDP
func getUDPPortNo() (uint16, error) {
	udpconn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return uint16(0), err
	}
	defer udpconn.Close()
	return uint16(udpconn.LocalAddr().(*net.UDPAddr).Port), nil
}
