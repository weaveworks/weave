package common

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"io/ioutil"
	"net"
	"runtime"
	"strings"
)

// Assert test is true, panic otherwise
func Assert(test bool) {
	if !test {
		panic("Assertion failure")
	}
}

func ErrorMessages(errors []error) string {
	var result []string
	for _, err := range errors {
		result = append(result, err.Error())
	}
	return strings.Join(result, "\n")
}

func WithNetNS(ns netns.NsHandle, work func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	oldNs, err := netns.Get()
	if err == nil {
		defer oldNs.Close()

		err = netns.Set(ns)
		if err == nil {
			defer netns.Set(oldNs)

			err = work()
		}
	}

	return nil
}

type NetDev struct {
	MAC   net.HardwareAddr
	CIDRs []*net.IPNet
}

// Search the network namespace of a process for interfaces matching a predicate
func FindNetDevs(procPath string, processID int, match func(string) bool) ([]NetDev, error) {
	var netDevs []NetDev

	ns, err := netns.GetFromPath(fmt.Sprintf("%s/%d/ns/net", procPath, processID))
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	err = WithNetNS(ns, func() error {
		links, err := netlink.LinkList()
		if err != nil {
			return err
		}
		for _, link := range links {
			if match(link.Attrs().Name) {
				addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
				if err != nil {
					return err
				}

				netDev := NetDev{MAC: link.Attrs().HardwareAddr}
				for _, addr := range addrs {
					netDev.CIDRs = append(netDev.CIDRs, addr.IPNet)
				}
				netDevs = append(netDevs, netDev)
			}
		}
		return nil
	})

	return netDevs, err
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(procPath string, processID int) ([]NetDev, error) {
	return FindNetDevs(procPath, processID, func(name string) bool {
		return strings.HasPrefix(name, "ethwe")
	})
}

// Get the weave bridge interface
func GetBridgeNetDev(procPath, bridgeName string) ([]NetDev, error) {
	return FindNetDevs(procPath, 1, func(name string) bool {
		return name == bridgeName
	})
}

func EnforceDockerBridgeAddrAssignType(bridgeName string) error {
	addrAssignType, err := ioutil.ReadFile(fmt.Sprintf("/sys/class/net/%s/addr_assign_type"))
	if err != nil {
		return err
	}

	// From include/uapi/linux/netdevice.h
	// #define NET_ADDR_PERM       0   /* address is permanent (default) */
	// #define NET_ADDR_RANDOM     1   /* address is generated randomly */
	// #define NET_ADDR_STOLEN     2   /* address is stolen from other device */
	// #define NET_ADDR_SET        3   /* address is set using dev_set_mac_address() */
	if string(addrAssignType) != "3" {
		link, err := netlink.LinkByName(bridgeName)
		if err != nil {
			return err
		}

		mac, err := RandomMAC()
		if err != nil {
			return err
		}

		if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
			return err
		}
	}

	return nil
}
