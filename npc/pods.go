package npc

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"

	"github.com/weaveworks/weave/common"
	weavenet "github.com/weaveworks/weave/net"
)

// Called when we see a pod's IP for the first time
func setupPod(podIP string, allowMcast bool) {
	if allowMcast {
		common.Log.Infof("Allowing multicast for pod with IP %q", podIP)
		return // nothing needs doing
	}
	ip := net.ParseIP(podIP)
	if ip == nil {
		common.Log.Errorf("Unable to parse pod IP %q", podIP)
		return
	}
	pid, err := findPidWithWeaveIP(ip)
	if err != nil {
		common.Log.Errorf("Unable to find pod with IP %q: %s", podIP, err)
		return
	}
	err = blockMcast(pid)
	if err != nil {
		common.Log.Errorf("Unable to find pod with IP %q: %s", podIP, err)
		return
	}
}

func findPidWithWeaveIP(ip net.IP) (int, error) {
	peerIDs, err := weavenet.ConnectedToBridgeVethPeerIds(weavenet.WeaveBridgeName)
	if err != nil {
		return 0, err
	}

	pids, err := common.AllPids("/proc")
	if err != nil {
		return 0, err
	}

	// Really we only want to look at those pids that own a namespace
	// - Kubernetes has all containers in a pod share the same namespace
	// but there seems to be nothing to distinguish the 'owner'.
	// We could reduce the work done by eliminating pids with duplicate namespaces.
	for _, pid := range pids {
		netDevs, err := weavenet.GetNetDevsByVethPeerIds(pid, peerIDs)
		if err != nil {
			return 0, err
		}
		for _, netDev := range netDevs {
			for _, cidr := range netDev.CIDRs {
				if ip.Equal(cidr.IP) {
					return pid, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("No process found with IP %v", ip)
}

func blockMcast(pid int) error {
	return iptablesInNamespace(pid, TableFilter, "INPUT", "-d", "224.0.0.0/4", "-j", "DROP")
}

func iptablesInNamespace(pid int, table, chain string, rulespec ...string) error {
	args := []string{fmt.Sprintf("--net=%d", pid), "iptables", "-I", "-t", table, "-C", chain}
	args = append(args, rulespec...)
	c := exec.Command("nsenter", args...)
	common.Log.Debugf("Running %v", c)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %s", string(stderr.Bytes()), err)
	}

	return nil
}
