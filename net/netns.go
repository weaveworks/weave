package net

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
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

var WeaveUtilCmd = "weaveutil"

// A safe version of WithNetNS* which creates a process executing
// "weaveutil <cmd> [args]" in the given namespace, using runc's nsexec mechanism.
func WithNetNS(nsPath string, cmd string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer

	parentPipe, childPipe, err := newPipe()
	if err != nil {
		return nil, err
	}
	args = append([]string{cmd}, args...)
	c := exec.Command(WeaveUtilCmd, args...)
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.ExtraFiles = []*os.File{childPipe}
	c.Env = []string{"_LIBCONTAINER_INITPIPE=3"}

	fmt.Printf("Starting %+v\n", c)

	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("%s failed to start: %v", WeaveUtilCmd, err)
	}

	// Send info down the pipe for nsexec to do its thing
	r := nl.NewNetlinkRequest(int(InitMsg), 0)
	r.AddData(&Bytemsg{
		Type:  NsPathsAttr,
		Value: []byte(nsPath),
	})
	fmt.Printf("Sending data %+v\n", r)
	if _, err := io.Copy(parentPipe, bytes.NewReader(r.Serialize())); err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(parentPipe)
	var pid *pid
	if err := decoder.Decode(&pid); err != nil {
		return nil, fmt.Errorf("Error decoding nsexec message: %s", err)
	}
	fmt.Printf("Child pid %d\n", pid)

	// The nsexec process exits immediately, but we need to wait on the child
	p, err := os.FindProcess(pid.Pid)
	if err != nil {
		return nil, fmt.Errorf("Could not find child process %d: %s", pid.Pid, err)
	}
	var state *os.ProcessState
	if state, err = p.Wait(); err == nil && !state.Success() {
		err = &exec.ExitError{ProcessState: state}
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %s", string(stderr.Bytes()), err)
	}
	fmt.Printf("Child result %+v %q\n", err, string(stdout.Bytes()))

	// Calling Wait() on the command tidies up, closes descriptors, etc
	if err := c.Wait(); err != nil {
		return nil, fmt.Errorf("%s: %s", string(stderr.Bytes()), err)
	}

	return stdout.Bytes(), nil
}

func newPipe() (parent *os.File, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

type pid struct {
	Pid int `json:"Pid"`
}

func WithNetNSByPid(pid int, cmd string, args ...string) ([]byte, error) {
	return WithNetNS(NSPathByPid(pid), cmd, args...)
}

func NSPathByPid(pid int) string {
	return NSPathByPidWithRoot("/", pid)
}

func NSPathByPidWithRoot(root string, pid int) string {
	return filepath.Join(root, fmt.Sprintf("/proc/%d/ns/net", pid))
}
