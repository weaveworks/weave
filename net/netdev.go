package net

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var netdevRegExp = regexp.MustCompile(`^([^ ]+?) ([^ ]+?) \[([^]]*)\]$`)

type Dev struct {
	Name  string
	MAC   net.HardwareAddr
	CIDRs []*net.IPNet
}

func (d Dev) String() string {
	return fmt.Sprintf("%s %s %s", d.Name, d.MAC, d.CIDRs)
}

func ParseNetDev(netdev string) (Dev, error) {
	match := netdevRegExp.FindStringSubmatch(netdev)
	if match == nil {
		return Dev{}, fmt.Errorf("invalid netdev: %s", netdev)
	}

	iface := match[1]
	mac, err := net.ParseMAC(match[2])
	if err != nil {
		return Dev{}, fmt.Errorf("cannot parse mac %s: %s", match[2], err)
	}

	var cidrs []*net.IPNet
	for _, cidr := range strings.Split(match[3], " ") {
		if cidr != "" {
			ip, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				return Dev{}, fmt.Errorf("cannot parse cidr %s: %s", cidr, err)
			}
			ipnet.IP = ip
			cidrs = append(cidrs, ipnet)
		}
	}

	return Dev{Name: iface, MAC: mac, CIDRs: cidrs}, nil
}

func LinkToNetDev(link netlink.Link) (Dev, error) {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return Dev{}, err
	}

	netDev := Dev{Name: link.Attrs().Name, MAC: link.Attrs().HardwareAddr}
	for _, addr := range addrs {
		netDev.CIDRs = append(netDev.CIDRs, addr.IPNet)
	}
	return netDev, nil
}

// ConnectedToBridgePeers returns peer indexes of veth links connected to the given
// bridge. The peer index is used to query from a container netns whether the
// container is connected to the bridge.
func ConnectedToBridgePeers(bridgeName string) ([]int, error) {
	var peers []int

	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, err
	}
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	// TODO(mp) check rtnetlink.c maybe there is a better way to list links connected
	// to bridge
	for _, link := range links {
		if _, isveth := link.(*netlink.Veth); isveth && link.Attrs().MasterIndex == br.Attrs().Index {
			peerIndex := link.Attrs().ParentIndex
			if peerIndex == 0 {
				// perhaps running on an older kernel where ParentIndex doesn't work.
				// as fall-back, assume the peers are consecutive
				peerIndex = link.Attrs().Index - 1
			}
			peers = append(peers, peerIndex)
		}
	}

	return peers, nil
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(processID int) ([]Dev, error) {
	// TODO(mp) pass by name
	peers, err := ConnectedToBridgePeers("weave")
	if err != nil {
		return nil, err
	}

	return GetWeaveNetDevsByPeers(processID, peers)
}

func GetWeaveNetDevsByPeers(processID int, peers []int) ([]Dev, error) {
	// Bail out if this container is running in the root namespace
	netnsRoot, err := netns.GetFromPid(1)
	if err != nil {
		return nil, fmt.Errorf("unable to open root namespace: %s", err)
	}
	netnsContainer, err := netns.GetFromPid(processID)
	if err != nil {
		return nil, fmt.Errorf("unable to open process %d namespace: %s", processID, err)
	}
	if netnsRoot.Equal(netnsContainer) {
		return nil, nil
	}

	var netdevs []Dev
	peersStr := make([]string, len(peers))

	for i, peer := range peers {
		peersStr[i] = strconv.Itoa(peer)
	}
	netdevsStr, err := WithNetNSByPid(processID, "list-netdevs", strings.Join(peersStr, ","))
	if err != nil {
		return nil, fmt.Errorf("list-netdevs failed: %s", err)
	}
	for _, netdevStr := range strings.Split(netdevsStr, "\n") {
		if netdevStr != "" {
			netdev, err := ParseNetDev(netdevStr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse netdev %s: %s", netdevStr, err)
			}
			netdevs = append(netdevs, netdev)
		}
	}

	return netdevs, nil
}

// Get the weave bridge interface.
// NB: Should be called from the root network namespace.
func GetBridgeNetDev(bridgeName string) (Dev, error) {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return Dev{}, err
	}

	return LinkToNetDev(link)
}
