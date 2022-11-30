package ipamplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
)

func (i *Ipam) CmdAdd(args *skel.CmdArgs) error {
	var conf types.NetConf
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}
	result, err := i.Allocate(args)
	if err != nil {
		return err
	}
	return types.PrintResult(result, conf.CNIVersion)
}

func (i *Ipam) Allocate(args *skel.CmdArgs) (types.Result, error) {
	// extract the things we care about
	conf, err := loadIPAMConf(args.StdinData)
	if err != nil {
		return nil, err
	}
	if conf == nil {
		conf = &ipamConf{}
	}
	containerID := args.ContainerID
	if containerID == "" {
		return nil, fmt.Errorf("Weave CNI Allocate: blank container name")
	}

	var ipconfigs []*current.IPConfig

	if len(conf.IPs) > 0 {
		// configuration includes desired IPs

		var ips []net.IP
		for _, ip := range conf.IPs {
			ip4 := net.ParseIP(ip).To4()
			ip16 := net.ParseIP(ip).To16()

			if ip4 == nil && ip16 == nil {
				return nil, errors.New("provided value is not an IP")
			}

			if ip4 == nil && ip16 != nil {
				return nil, errors.New("allocation of ipv6 addresses is not implemented")
			}

			ips = append(ips, ip4)
		}

		for j := range ips {
			ipnet := &net.IPNet{
				IP:   ips[j],
				Mask: ips[j].DefaultMask(),
			}

			err := i.weave.ClaimIP(containerID, ipnet, false)
			if err != nil {
				return nil, err
			}

			ipconfigs = append(ipconfigs, &current.IPConfig{
				Version: "4",
				Address: *ipnet,
				Gateway: conf.Gateway,
			})
		}
	} else if conf.Subnet == "" {
		// configuration doesn't include Subnet or IPs, so ask the allocator for an IP
		ipnet, err := i.weave.AllocateIP(containerID, false)
		if err != nil {
			return nil, err
		}

		ipconfigs = append(ipconfigs, &current.IPConfig{
			Version: "4",
			Address: *ipnet,
			Gateway: conf.Gateway,
		})
	} else {
		// configuration includes desired Subnet

		subnet, err := types.ParseCIDR(conf.Subnet)
		if err != nil {
			return nil, fmt.Errorf("subnet given in config, but not parseable: %s", err)
		}
		ipnet, err := i.weave.AllocateIPInSubnet(containerID, subnet, false)
		if err != nil {
			return nil, err
		}

		ipconfigs = append(ipconfigs, &current.IPConfig{
			Version: "4",
			Address: *ipnet,
			Gateway: conf.Gateway,
		})
	}

	return &current.Result{
		IPs:    ipconfigs,
		Routes: conf.Routes,
	}, nil
}

func (i *Ipam) CmdDel(args *skel.CmdArgs) error {
	return i.Release(args)
}

func (i *Ipam) Release(args *skel.CmdArgs) error {
	return i.weave.ReleaseIPsFor(args.ContainerID)
}

type ipamConf struct {
	Subnet  string         `json:"subnet,omitempty"`
	Gateway net.IP         `json:"gateway,omitempty"`
	Routes  []*types.Route `json:"routes"`
	IPs     []string       `json:"ips,omitempty"`
}

type netConf struct {
	IPAM *ipamConf `json:"ipam"`
}

func loadIPAMConf(stdinData []byte) (*ipamConf, error) {
	var conf netConf
	return conf.IPAM, json.Unmarshal(stdinData, &conf)
}
