package net

import (
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// NB: The following function is unsafe, because it changes a network namespace
//     of an OS thread which executes it. During the execution, the Go runtime
//     might clone a new thread which is going to run in the given ns and might
//     schedule other go-routines which suppose to run in the host network ns.
//     Also, the work function cannot create any go-routine, because it might
//     be run by other threads running in any non-given network namespace.
//     Please see https://github.com/weaveworks/weave/issues/2388#issuecomment-228365069
//     for more details.
//
//     Before using, make sure that you understand the implications!
func WithNetNSUnsafe(ns netns.NsHandle, work func() error) error {
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

func WithNetNSLinkUnsafe(ns netns.NsHandle, ifName string, work func(link netlink.Link) error) error {
	return WithNetNSUnsafe(ns, func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}
		return work(link)
	})
}
