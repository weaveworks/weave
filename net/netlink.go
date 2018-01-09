package net

import (
	"syscall"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

func LinkAddIfNotExist(link netlink.Link) error {
	err := netlink.LinkAdd(link)
	if err != nil && err == syscall.EEXIST {
		return nil
	}
	return errors.Wrapf(err, "creating link %q", link)
}
