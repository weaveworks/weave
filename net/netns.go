// +build go1.10

package net

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var ErrLinkNotFound = errors.New("Link not found")

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
			if err.Error() == errors.New("Link not found").Error() {
				return ErrLinkNotFound
			}
			return err
		}
		return work(link)
	})
}

func WithNetNSByPath(path string, work func() error) error {
	ns, err := netns.GetFromPath(path)
	if err != nil {
		return err
	}
	return WithNetNS(ns, work)
}

func NSPathByPid(pid int) string {
	return NSPathByPidWithRoot("/", pid)
}

func NSPathByPidWithRoot(root string, pid int) string {
	return filepath.Join(root, fmt.Sprintf("/proc/%d/ns/net", pid))
}
