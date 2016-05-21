package main

import (
	"net"
	"strconv"

	"github.com/vishvananda/netns"

	weavenet "github.com/weaveworks/weave/net"
)

func attach(args []string) error {
	if len(args) < 4 {
		cmdUsage("attach-container", "[--no-multicast-route] <pid> <bridge-name> <mtu> <cidr>...")
	}

	withMulticastRoute := true
	if args[0] == "--no-multicast-route" {
		withMulticastRoute = false
		args = args[1:]
	}

	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return err
	}
	mtu, err := strconv.Atoi(args[2])
	if err != nil && args[3] != "" {
		return err
	}
	cidrs, err := parseCIDRs(args[3:])
	if err != nil {
		return err
	}
	return weavenet.AttachContainer(ns, args[0], "ethwe", args[1], mtu, withMulticastRoute, cidrs)
}

func parseCIDRs(args []string) (cidrs []*net.IPNet, err error) {
	for _, ipstr := range args {
		ip, ipnet, err := net.ParseCIDR(ipstr)
		if err != nil {
			return nil, err
		}
		ipnet.IP = ip // we want the specific IP plus the mask
		cidrs = append(cidrs, ipnet)
	}
	return
}

func detach(args []string) error {
	if len(args) < 2 {
		cmdUsage("detach-container", "<pid> <cidr>...")
	}

	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return err
	}
	cidrs, err := parseCIDRs(args[1:])
	if err != nil {
		return err
	}
	return weavenet.DetachContainer(ns, args[0], "ethwe", cidrs)
}
