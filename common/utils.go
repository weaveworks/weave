package common

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	weavenet "github.com/weaveworks/weave/net"
)

// Assert test is true, panic otherwise
func Assert(test bool) {
	if !test {
		panic("Assertion failure")
	}
}

// AssertWithMsg panics with the given msg if test is false
func AssertWithMsg(test bool, msg string) {
	if !test {
		panic(msg)
	}
}

func ErrorMessages(errors []error) string {
	var result []string
	for _, err := range errors {
		result = append(result, err.Error())
	}
	return strings.Join(result, "\n")
}

type NetDev struct {
	Name  string
	MAC   net.HardwareAddr
	CIDRs []*net.IPNet
}

// Search the network namespace of a process for interfaces matching a predicate
func FindNetDevs(processID int, match func(link netlink.Link) bool) ([]NetDev, error) {
	var netDevs []NetDev

	ns, err := netns.GetFromPid(processID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer ns.Close()

	err = weavenet.WithNetNS(ns, func() error {
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

	netDev := &NetDev{Name: link.Attrs().Name, MAC: link.Attrs().HardwareAddr}
	for _, addr := range addrs {
		netDev.CIDRs = append(netDev.CIDRs, addr.IPNet)
	}
	return netDev, nil
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(processID int) ([]NetDev, error) {
	// Bail out if this container is running in the root namespace
	nsToplevel, err := netns.GetFromPid(1)
	if err != nil {
		return nil, fmt.Errorf("unable to open root namespace: %s", err)
	}
	nsContainr, err := netns.GetFromPid(processID)
	if err != nil {
		return nil, fmt.Errorf("unable to open process %d namespace: %s", processID, err)
	}
	if nsToplevel.Equal(nsContainr) {
		return nil, nil
	}

	weaveBridge, err := netlink.LinkByName("weave")
	if err != nil {
		return nil, fmt.Errorf("Cannot find weave bridge: %s", err)
	}
	// Scan devices in root namespace to find those attached to weave bridge
	indexes := make(map[int]struct{})
	err = forEachLink(func(link netlink.Link) error {
		if link.Attrs().MasterIndex == weaveBridge.Attrs().Index {
			peerIndex := link.Attrs().ParentIndex
			if peerIndex == 0 {
				// perhaps running on an older kernel where ParentIndex doesn't work.
				// as fall-back, assume the indexes are consecutive
				peerIndex = link.Attrs().Index - 1
			}
			indexes[peerIndex] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return FindNetDevs(processID, func(link netlink.Link) bool {
		_, isveth := link.(*netlink.Veth)
		_, found := indexes[link.Attrs().Index]
		return isveth && found
	})
}

// Get the weave bridge interface
func GetBridgeNetDev(bridgeName string) ([]NetDev, error) {
	return FindNetDevs(1, func(link netlink.Link) bool {
		return link.Attrs().Name == bridgeName
	})
}
