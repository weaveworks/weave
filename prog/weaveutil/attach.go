package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/vishvananda/netns"

	weavenet "github.com/rajch/weave/net"
	"github.com/rajch/weave/proxy"
)

func attach(args []string) error {
	if len(args) < 3 {
		cmdUsage("attach-container", "[--no-multicast-route] [--keep-tx-on] [--hairpin-mode=true|false] <container-id> <bridge-name> <cidr>...")
	}

	keepTXOn := false
	withMulticastRoute := true
	hairpinMode := true
	for i := 0; i < len(args); {
		switch args[i] {
		case "--no-multicast-route":
			withMulticastRoute = false
			args = append(args[:i], args[i+1:]...)
		case "--keep-tx-on":
			keepTXOn = true
			args = append(args[:i], args[i+1:]...)
		case "--hairpin-mode=false":
			hairpinMode = false
			args = append(args[:i], args[i+1:]...)
		default:
			i++
		}
	}

	pid, err := containerPid(args[0])
	if err != nil {
		return err
	}
	nsContainer, err := netns.GetFromPid(pid)
	if err != nil {
		return fmt.Errorf("unable to open namespace for container %s: %s", args[0], err)
	}

	if nsHost, err := netns.GetFromPid(1); err != nil {
		return fmt.Errorf("unable to open host namespace: %s", err)
	} else if nsHost.Equal(nsContainer) {
		return fmt.Errorf("Container is running in the host network namespace, and therefore cannot be\nconnected to weave - perhaps the container was started with --net=host")
	}
	cidrs, err := parseCIDRs(args[2:])
	if err != nil {
		return err
	}

	err = weavenet.AttachContainer(weavenet.NSPathByPid(pid), fmt.Sprint(pid), weavenet.VethName, args[1], 0, withMulticastRoute, cidrs, keepTXOn, hairpinMode)
	// If we detected an error but the container has died, tell the user that instead.
	if err != nil && !processExists(pid) {
		err = fmt.Errorf("Container %s died", args[0])
	}
	return err
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

	pid, err := containerPid(args[0])
	if err != nil {
		return err
	}
	cidrs, err := parseCIDRs(args[1:])
	if err != nil {
		return err
	}
	return weavenet.DetachContainer(weavenet.NSPathByPid(pid), args[0], weavenet.VethName, cidrs)
}

func rewriteEtcHosts(args []string) error {
	if len(args) < 3 {
		cmdUsage("rewrite-etc-hosts", "<container-id> <image> <cidr> [name:addr...]")
	}
	containerID := args[0]
	image := args[1]
	cidrs := args[2]
	extraHosts := args[3:]

	container, err := inspectContainer(containerID)
	if err != nil {
		return err
	}

	hostsPath := container.HostsPath
	fqdn := container.Config.Hostname + "." + container.Config.Domainname

	var ips []*net.IPNet
	for _, cidr := range strings.Fields(cidrs) {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}
		ips = append(ips, ipnet)
	}

	docker := os.Getenv("DOCKER_HOST")
	if docker == "" {
		docker = "unix:///var/run/docker.sock"
	}
	p, err := proxy.StubProxy(proxy.Config{DockerHost: docker, Image: image})
	if err != nil {
		return err
	}
	return p.RewriteEtcHosts(hostsPath, fqdn, ips, extraHosts)
}
