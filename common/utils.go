package common

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
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
func FindNetDevs(processID int, match func(string) bool) ([]NetDev, error) {
	var netDevs []NetDev

	ns, err := netns.GetFromPath(fmt.Sprintf("/proc/%d/ns/net", processID))
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
func GetWeaveNetDevs(processID int) ([]NetDev, error) {
	return FindNetDevs(processID, func(name string) bool {
		return strings.HasPrefix(name, "ethwe")
	})
}

// Get the weave bridge interface
func GetBridgeNetDev(bridgeName string) ([]NetDev, error) {
	return FindNetDevs(1, func(name string) bool {
		return name == bridgeName
	})
}
