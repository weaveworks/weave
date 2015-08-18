package net

import (
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink/nl"
)

// Wait for an interface to come up.
func EnsureInterface(ifaceName string) (*net.Interface, error) {
	s, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_LINK)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	iface, err := ensureInterface(s, ifaceName)
	if err != nil {
		return nil, err
	}
	return iface, err
}

func ensureInterface(s *nl.NetlinkSocket, ifaceName string) (*net.Interface, error) {
	if iface, err := findInterface(ifaceName); err == nil {
		return iface, nil
	}
	waitForIfUp(s, ifaceName)
	iface, err := findInterface(ifaceName)
	return iface, err
}

// Wait for an interface to come up and have a route added to the multicast subnet.
// This matches the behaviour in 'weave attach', which is the only context in which
// we expect this to be called.  If you change one, change the other to match.
func EnsureInterfaceAndMcastRoute(ifaceName string) (*net.Interface, error) {
	s, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_LINK, syscall.RTNLGRP_IPV4_ROUTE)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	iface, err := ensureInterface(s, ifaceName)
	if err != nil {
		return nil, err
	}
	dest := net.IPv4(224, 0, 0, 0)
	if CheckRouteExists(ifaceName, dest) {
		return iface, err
	}
	waitForRoute(s, ifaceName, dest)
	return iface, err
}

func findInterface(ifaceName string) (iface *net.Interface, err error) {
	if iface, err = net.InterfaceByName(ifaceName); err != nil {
		return iface, fmt.Errorf("Unable to find interface %s", ifaceName)
	}
	if 0 == (net.FlagUp & iface.Flags) {
		return iface, fmt.Errorf("Interface %s is not up", ifaceName)
	}
	return
}

func waitForIfUp(s *nl.NetlinkSocket, ifaceName string) error {
	for {
		msgs, err := s.Receive()
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.Header.Type {
			case syscall.RTM_NEWLINK: // receive this type for link 'up'
				ifmsg := nl.DeserializeIfInfomsg(m.Data)
				attrs, err := syscall.ParseNetlinkRouteAttr(&m)
				if err != nil {
					return err
				}
				name := ""
				for _, attr := range attrs {
					if attr.Attr.Type == syscall.IFA_LABEL {
						name = string(attr.Value[:len(attr.Value)-1])
					}
				}
				if ifaceName == name && ifmsg.Flags&syscall.IFF_UP != 0 {
					return nil
				}
			}
		}
	}
}
