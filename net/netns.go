package net

import (
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

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

func WithNetNSLink(ns netns.NsHandle, ifName string, work func(link netlink.Link) error) error {
	return WithNetNS(ns, func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}
		return work(link)
	})
}

func WithNetNSLinkByPid(pid int, ifName string, work func(link netlink.Link) error) error {
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return err
	}
	defer ns.Close()

	return WithNetNSLink(ns, ifName, work)
}
