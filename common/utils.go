package common

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/j-keck/arping"
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
// Note that the predicate is called while the goroutine is inside the process' netns
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

	err = weavenet.WithNetNSUnsafe(ns, func() error {
		return forEachLink(func(link netlink.Link) error {
			if match(link) {
				netDev, err := linkToNetDev(link)
				if err != nil {
					return err
				}
				netDevs = append(netDevs, netDev)
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

func linkToNetDev(link netlink.Link) (NetDev, error) {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return NetDev{}, err
	}

	netDev := NetDev{Name: link.Attrs().Name, MAC: link.Attrs().HardwareAddr}
	for _, addr := range addrs {
		netDev.CIDRs = append(netDev.CIDRs, addr.IPNet)
	}
	return netDev, nil
}

// ConnectedToBridgePredicate returns a function which is used to query whether
// a given link is a veth interface which one end is connected to a bridge.
// The returned function should be called from a container network namespace which
// the bridge does NOT belong to.
func ConnectedToBridgePredicate(bridgeName string) (func(link netlink.Link) bool, error) {
	indexes := make(map[int]struct{})

	// Scan devices in root namespace to find those attached to weave bridge
	err := weavenet.WithNetNSLinkByPidUnsafe(1, bridgeName,
		func(br netlink.Link) error {
			return forEachLink(func(link netlink.Link) error {
				if link.Attrs().MasterIndex == br.Attrs().Index {
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
		})
	if err != nil {
		return nil, err
	}

	return func(link netlink.Link) bool {
		_, isveth := link.(*netlink.Veth)
		_, found := indexes[link.Attrs().Index]
		return isveth && found
	}, nil
}

func GetNetDevsWithPredicate(processID int, predicate func(link netlink.Link) bool) ([]NetDev, error) {
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

	return FindNetDevs(processID, predicate)
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(processID int) ([]NetDev, error) {
	p, err := ConnectedToBridgePredicate("weave")
	if err != nil {
		return nil, err
	}

	return GetNetDevsWithPredicate(processID, p)
}

// Get the weave bridge interface.
// NB: Should be called from the root network namespace.
func GetBridgeNetDev(bridgeName string) (NetDev, error) {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return NetDev{}, err
	}

	return linkToNetDev(link)
}

// Do post-attach configuration of all veths we have created
func ConfigureARPforVeths(processID int, prefix string) error {
	_, err := FindNetDevs(processID, func(link netlink.Link) bool {
		ifName := link.Attrs().Name
		if strings.HasPrefix(ifName, prefix) {
			weavenet.ConfigureARPCache(ifName)
			if addrs, err := netlink.AddrList(link, netlink.FAMILY_V4); err == nil {
				for _, addr := range addrs {
					arping.GratuitousArpOverIfaceByName(addr.IPNet.IP, ifName)
				}
			}
		}
		return false
	})
	return err
}
