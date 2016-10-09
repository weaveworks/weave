package main

import (
	"strings"

	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	weavenet "github.com/weaveworks/weave/net"
)

func configureARP(args []string) error {
	if len(args) != 1 {
		cmdUsage("configure-arp", "<iface-name-prefix>")
	}
	prefix := args[0]

	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		ifName := link.Attrs().Name
		if strings.HasPrefix(ifName, prefix) {
			weavenet.ConfigureARPCache(ifName)
			if addrs, err := netlink.AddrList(link, netlink.FAMILY_V4); err == nil {
				for _, addr := range addrs {
					arping.GratuitousArpOverIfaceByName(addr.IPNet.IP, ifName)
				}
			}
		}
	}

	return nil
}
