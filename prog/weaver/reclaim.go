package main

import (
	"net"

	"github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam"
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
)

func a(cidr *net.IPNet) address.CIDR {
	prefixLength, _ := cidr.Mask.Size()
	return address.CIDR{Addr: address.FromIP4(cidr.IP), PrefixLen: prefixLength}
}

// Get all the existing Weave IPs at startup, so we can stop IPAM
// giving out any as duplicates
func findExistingAddresses(bridgeName string) (addrs []ipam.PreClaim, err error) {
	// First get the address for the bridge
	bridgeNetDev, err := weavenet.GetBridgeNetDev(bridgeName)
	if err != nil {
		return nil, err
	}
	for _, cidr := range bridgeNetDev.CIDRs {
		addrs = append(addrs, ipam.PreClaim{Ident: "weave:expose", Cidr: a(cidr)})
	}

	// Then find all veths connected to the bridge
	peerIDs, err := weavenet.ConnectedToBridgeVethPeerIds(bridgeName)
	if err != nil {
		return nil, err
	}

	pids, err := common.AllPids("/proc")
	if err != nil {
		return nil, err
	}

	// Now iterate over all processes to see if they have a network namespace with an attached interface
	for _, pid := range pids {
		netDevs, err := weavenet.GetNetDevsByVethPeerIds(pid, peerIDs)
		if err != nil {
			return nil, err
		}
		for _, netDev := range netDevs {
			for _, cidr := range netDev.CIDRs {
				// We don't know the container ID, so use special magic string
				addrs = append(addrs, ipam.PreClaim{Ident: api.NoContainerID, Cidr: a(cidr)})
			}
		}
	}
	return addrs, nil
}
