package common

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
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

func ErrorMessages(errors []error) string {
	var result []string
	for _, err := range errors {
		result = append(result, err.Error())
	}
	return strings.Join(result, "\n")
}

var netdevRegExp = regexp.MustCompile(`^([^ ]+?) ([^ ]+?) \[([^]]*)\]$`)

type NetDev struct {
	Name  string
	MAC   net.HardwareAddr
	CIDRs []*net.IPNet
}

func (d NetDev) String() string {
	return fmt.Sprintf("%s %s %s", d.Name, d.MAC, d.CIDRs)
}

func ParseNetDev(netdev string) (NetDev, error) {
	match := netdevRegExp.FindStringSubmatch(netdev)
	if match == nil {
		return NetDev{}, fmt.Errorf("invalid netdev: %s", netdev)
	}

	iface := match[1]
	mac, err := net.ParseMAC(match[2])
	if err != nil {
		return NetDev{}, fmt.Errorf("cannot parse mac %s: %s", match[2], err)
	}

	var cidrs []*net.IPNet
	for _, cidr := range strings.Split(match[3], " ") {
		if cidr != "" {
			ip, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				return NetDev{}, fmt.Errorf("cannot parse cidr %s: %s", cidr, err)
			}
			ipnet.IP = ip
			cidrs = append(cidrs, ipnet)
		}
	}

	return NetDev{Name: iface, MAC: mac, CIDRs: cidrs}, nil
}

func LinkToNetDev(link netlink.Link) (NetDev, error) {
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

// ConnectedToBridgeVethPeerIds returns peer indexes of veth links connected to
// the given bridge. The peer index is used to query from a container netns
// whether the container is connected to the bridge.
func ConnectedToBridgeVethPeerIds(bridgeName string) ([]int, error) {
	var ids []int

	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, err
	}
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if _, isveth := link.(*netlink.Veth); isveth && link.Attrs().MasterIndex == br.Attrs().Index {
			peerID := link.Attrs().ParentIndex
			if peerID == 0 {
				// perhaps running on an older kernel where ParentIndex doesn't work.
				// as fall-back, assume the peers are consecutive
				peerID = link.Attrs().Index - 1
			}
			ids = append(ids, peerID)
		}
	}

	return ids, nil
}

// Lookup the weave interface of a container
func GetWeaveNetDevs(processID int) ([]NetDev, error) {
	peerIDs, err := ConnectedToBridgeVethPeerIds("weave")
	if err != nil {
		return nil, err
	}

	return GetNetDevsByVethPeerIds(processID, peerIDs)
}

func GetNetDevsByVethPeerIds(processID int, peerIDs []int) ([]NetDev, error) {
	// Bail out if this container is running in the root namespace
	netnsRoot, err := netns.GetFromPid(1)
	if err != nil {
		return nil, fmt.Errorf("unable to open root namespace: %s", err)
	}
	defer netnsRoot.Close()
	netnsContainer, err := netns.GetFromPid(processID)
	if err != nil {
		// Unable to find a namespace for this process - just return nothing
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to open process %d namespace: %s", processID, err)
	}
	defer netnsContainer.Close()
	if netnsRoot.Equal(netnsContainer) {
		return nil, nil
	}

	var netdevs []NetDev

	peersStr := make([]string, len(peerIDs))
	for i, id := range peerIDs {
		peersStr[i] = strconv.Itoa(id)
	}
	netdevsStr, err := weavenet.WithNetNSByPid(processID, "list-netdevs", strings.Join(peersStr, ","))
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
func GetBridgeNetDev(bridgeName string) (NetDev, error) {
	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return NetDev{}, err
	}

	return LinkToNetDev(link)
}
