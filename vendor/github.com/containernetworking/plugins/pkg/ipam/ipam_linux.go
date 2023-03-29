// Copyright 2015 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ipam

import (
	"fmt"
	"net"
	"os"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"

	"github.com/vishvananda/netlink"
)

const (
	// Note: use slash as separator so we can have dots in interface name (VLANs)
	DisableIPv6SysctlTemplate = "net/ipv6/conf/%s/disable_ipv6"
)

// ConfigureIface takes the result of IPAM plugin and
// applies to the ifName interface
func ConfigureIface(ifName string, res *current.Result) error {
	if len(res.Interfaces) == 0 {
		return fmt.Errorf("no interfaces to configure")
	}

	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", ifName, err)
	}

	var v4gw, v6gw net.IP
	var has_enabled_ipv6 bool = false
	for _, ipc := range res.IPs {
		if ipc.Interface == nil {
			continue
		}
		intIdx := *ipc.Interface
		if intIdx < 0 || intIdx >= len(res.Interfaces) || res.Interfaces[intIdx].Name != ifName {
			// IP address is for a different interface
			return fmt.Errorf("failed to add IP addr %v to %q: invalid interface index", ipc, ifName)
		}

		// Make sure sysctl "disable_ipv6" is 0 if we are about to add
		// an IPv6 address to the interface
		if !has_enabled_ipv6 && ipc.Address.IP.To4() == nil {
			// Enabled IPv6 for loopback "lo" and the interface
			// being configured
			for _, iface := range [2]string{"lo", ifName} {
				ipv6SysctlValueName := fmt.Sprintf(DisableIPv6SysctlTemplate, iface)

				// Read current sysctl value
				value, err := sysctl.Sysctl(ipv6SysctlValueName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ipam_linux: failed to read sysctl %q: %v\n", ipv6SysctlValueName, err)
					continue
				}
				if value == "0" {
					continue
				}

				// Write sysctl to enable IPv6
				_, err = sysctl.Sysctl(ipv6SysctlValueName, "0")
				if err != nil {
					return fmt.Errorf("failed to enable IPv6 for interface %q (%s=%s): %v", iface, ipv6SysctlValueName, value, err)
				}
			}
			has_enabled_ipv6 = true
		}

		addr := &netlink.Addr{IPNet: &ipc.Address, Label: ""}
		if err = netlink.AddrAdd(link, addr); err != nil {
			return fmt.Errorf("failed to add IP addr %v to %q: %v", ipc, ifName, err)
		}

		gwIsV4 := ipc.Gateway.To4() != nil
		if gwIsV4 && v4gw == nil {
			v4gw = ipc.Gateway
		} else if !gwIsV4 && v6gw == nil {
			v6gw = ipc.Gateway
		}
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", ifName, err)
	}

	if v6gw != nil {
		ip.SettleAddresses(ifName, 10)
	}

	for _, r := range res.Routes {
		routeIsV4 := r.Dst.IP.To4() != nil
		gw := r.GW
		if gw == nil {
			if routeIsV4 && v4gw != nil {
				gw = v4gw
			} else if !routeIsV4 && v6gw != nil {
				gw = v6gw
			}
		}
		route := netlink.Route{
			Dst:       &r.Dst,
			LinkIndex: link.Attrs().Index,
			Gw:        gw,
		}

		if err = netlink.RouteAddEcmp(&route); err != nil {
			return fmt.Errorf("failed to add route '%v via %v dev %v': %v", r.Dst, gw, ifName, err)
		}
	}

	return nil
}
