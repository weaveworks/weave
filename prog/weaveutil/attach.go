package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/vishvananda/netns"

	"github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	weavenet "github.com/weaveworks/weave/net"
)

func attach(args []string) error {
	if len(args) < 4 {
		cmdUsage("attach-container", "[--no-multicast-route] <container-id> <bridge-name> <mtu> <cidr>...")
	}

	client := api.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	keepTXOn := false
	isAWSVPC := false
	// In a case of an error, we skip applying necessary steps for AWSVPC, because
	// "attach" should work without the weave router running.
	if t, err := client.LocalRangeTracker(); err != nil {
		fmt.Fprintf(os.Stderr, "unable to determine tracker: %s; skipping AWSVPC initialization\n", err)
	} else if t == "awsvpc" {
		isAWSVPC = true
		keepTXOn = true
	}

	withMulticastRoute := true
	if args[0] == "--no-multicast-route" {
		withMulticastRoute = false
		args = args[1:]
	}
	if isAWSVPC {
		withMulticastRoute = false
	}

	pid, nsContainer, err := containerPidAndNs(args[0])
	if err != nil {
		return err
	}
	if nsHost, err := netns.GetFromPid(1); err != nil {
		return fmt.Errorf("unable to open host namespace: %s", err)
	} else if nsHost.Equal(nsContainer) {
		return fmt.Errorf("Container is running in the host network namespace, and therefore cannot be\nconnected to weave. Perhaps the container was started with --net=host.")
	}
	mtu, err := strconv.Atoi(args[2])
	if err != nil && args[3] != "" {
		return fmt.Errorf("unable to parse mtu %q: %s", args[2], err)
	}
	cidrs, err := parseCIDRs(args[3:])
	if err != nil {
		return err
	}

	if isAWSVPC {
		// Currently, we allow only IP addresses from the default subnet
		if defaultSubnet, err := client.DefaultSubnet(); err != nil {
			fmt.Fprintf(os.Stderr, "unable to retrieve default subnet: %s; skipping AWSVPC checks\n", err)
		} else {
			for _, cidr := range cidrs {
				if !sameSubnet(cidr, defaultSubnet) {
					format := "AWSVPC constraints violation: %s does not belong to the default subnet %s"
					return fmt.Errorf(format, cidr, defaultSubnet)
				}
			}
		}
	}

	err = weavenet.AttachContainer(nsContainer, fmt.Sprint(pid), weavenet.VethName, args[1], mtu, withMulticastRoute, cidrs, keepTXOn)
	// If we detected an error but the container has died, tell the user that instead.
	if err != nil && !processExists(pid) {
		err = fmt.Errorf("Container %s died", args[0])
	}
	return err
}

// sameSubnet checks whether ip belongs to network's subnet
func sameSubnet(ip *net.IPNet, network *net.IPNet) bool {
	if network.Contains(ip.IP) {
		i1, i2 := ip.Mask.Size()
		n1, n2 := network.Mask.Size()
		return i1 == n1 && i2 == n2
	}
	return false
}

func containerPidAndNs(containerID string) (int, netns.NsHandle, error) {
	c, err := docker.NewVersionedClientFromEnv("1.18")
	if err != nil {
		return 0, 0, fmt.Errorf("unable to connect to docker: %s", err)
	}
	container, err := c.InspectContainer(containerID)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to inspect container %s: %s", containerID, err)
	}
	if container.State.Pid == 0 {
		return 0, 0, fmt.Errorf("container %s not running", containerID)
	}
	ns, err := netns.GetFromPid(container.State.Pid)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to open namespace for container %s: %s", containerID, err)
	}
	return container.State.Pid, ns, nil
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
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
		cmdUsage("detach-container", "<container-id> <cidr>...")
	}

	_, ns, err := containerPidAndNs(args[0])
	if err != nil {
		return err
	}
	cidrs, err := parseCIDRs(args[1:])
	if err != nil {
		return err
	}
	return weavenet.DetachContainer(ns, args[0], weavenet.VethName, cidrs)
}
