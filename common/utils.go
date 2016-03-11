package common

import (
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

	return err
}

type NetDev struct {
	MAC   net.HardwareAddr
	CIDRs []*net.IPNet
}

// Search the network namespace of a process for interfaces matching a predicate
func FindNetDevs(processID int, match func(link netlink.Link) bool) ([]NetDev, error) {
	var netDevs []NetDev

	ns, err := netns.GetFromPid(processID)
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	err = WithNetNS(ns, func() error {
		return forEachLink(func(link netlink.Link) error {
			if match(link) {
				netDev, err := linkToNetDev(link)
				if err != nil {
					return err
				}
				netDevs = append(netDevs, *netDev)
			}
			return nil
		})
	})

	return netDevs, err
}

func forEachLink(f func(netlink.Link) error) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := f(link); err != nil {
			return err
		}
	}
	return nil
}

func linkToNetDev(link netlink.Link) (*NetDev, error) {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}

	netDev := &NetDev{MAC: link.Attrs().HardwareAddr}
	for _, addr := range addrs {
		netDev.CIDRs = append(netDev.CIDRs, addr.IPNet)
	}
	return netDev, nil
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(processID int) ([]NetDev, error) {
	return FindNetDevs(processID, func(link netlink.Link) bool {
		return strings.HasPrefix(link.Attrs().Name, "ethwe")
	})
}

// Get the weave bridge interface
func GetBridgeNetDev(bridgeName string) ([]NetDev, error) {
	return FindNetDevs(1, func(link netlink.Link) bool {
		return link.Attrs().Name == bridgeName
	})
}
