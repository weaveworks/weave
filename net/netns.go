package net

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var ErrLinkNotFound = errors.New("Link not found")

// NB: The following function is unsafe, because:
//     - It changes a network namespace (netns) of an OS thread which runs
//       the function. During execution, the Go runtime might clone a new OS thread
//       for scheduling other go-routines, thus they might end up running in
//       a "wrong" netns.
//     - runtime.LockOSThread does not guarantee that a spawned go-routine on
//       the locked thread will be run by it. Thus, the work function is
//       not allowed to spawn any go-routine which is dependent on the given netns.

//     Please see https://github.com/weaveworks/weave/issues/2388#issuecomment-228365069
//     for more details and make sure that you understand the implications before
//     using the function!
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
			if err.Error() == errors.New("Link not found").Error() {
				return ErrLinkNotFound
			}
			return err
		}
		return work(link)
	})
}

func WithNetNSLinkByPidUnsafe(pid int, ifName string, work func(link netlink.Link) error) error {
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return err
	}
	defer ns.Close()

	return WithNetNSLinkUnsafe(ns, ifName, work)
}

// A bit safer version of WithNetNS* which creates a process executing
// "weaveutil --netns-fd <cmd>".
//
// The main problem with this approach is that the weaveutil process cannot
// create any go-routine while executing a given cmd, unless we make sure that
// all OS threads of the Go runtime instance running the cmd is in the same
// netns.
//
// TODO(mp) Fix (indirect) circular dependency (weaveutil -> net -> weaveutil)
func WithNetNS(ns netns.NsHandle, cmd string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer

	utilExecPath, err := findUtilExec()
	if err != nil {
		return "", fmt.Errorf("cannot find %q executable: %s", utilExecName, err)
	}

	args = append([]string{"--netns-fd", strconv.Itoa(int(ns)), cmd}, args...)
	// netns.Get* does not set O_CLOEXEC for netns fd, so the fd can be reused by a child
	c := exec.Command(utilExecPath, args...)
	c.Stdout = &stdout
	c.Stderr = &stderr
	err = c.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %s", string(stderr.Bytes()), err)
	}

	return string(stdout.Bytes()), nil
}

func WithNetNSByPid(pid int, cmd string, args ...string) (string, error) {
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return "", err
	}

	return WithNetNS(ns, cmd, args...)
}

// Helpers

const utilExecName = "weaveutil"

func findUtilExec() (string, error) {
	// First, check whether it runs inside "weaveutil" process
	p, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return "", err
	}
	if path.Base(p) == utilExecName {
		return p, nil
	}

	// If not, lookup $PATH
	p, err = exec.LookPath(utilExecName)
	if err != nil {
		return "", err
	}
	return p, nil
}
