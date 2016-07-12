package net

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

func SetMTU(ifaceName string, mtu int) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("Unable to find interface %s: %s", ifaceName, err)
	}
	return netlink.LinkSetMTU(link, mtu)
}
