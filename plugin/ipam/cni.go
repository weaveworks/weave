package ipamplugin

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/appc/cni/pkg/skel"
	"github.com/appc/cni/pkg/types"
)

func (i *Ipam) CmdAdd(args *skel.CmdArgs) error {
	// extract the things we care about
	conf, err := loadIPAMConf(args.StdinData)
	if err != nil {
		return err
	}
	if conf == nil {
		conf = &ipamConf{}
	}
	containerID := args.ContainerID
	var ipnet *net.IPNet

	if conf.Subnet == "" {
		ipnet, err = i.weave.AllocateIP(containerID)
	} else {
		var subnet *net.IPNet
		subnet, err = types.ParseCIDR(conf.Subnet)
		if err != nil {
			return fmt.Errorf("subnet given in config, but not parseable: %s", err)
		}
		ipnet, err = i.weave.AllocateIPInSubnet(containerID, subnet)
	}

	if err != nil {
		return err
	}
	result := types.Result{
		IP4: &types.IPConfig{
			IP:      *ipnet,
			Gateway: conf.Gateway,
			Routes:  conf.Routes,
		},
	}
	return result.Print()
}

func (i *Ipam) CmdDel(args *skel.CmdArgs) error {
	return i.weave.ReleaseIPsFor(args.ContainerID)
}

type ipamConf struct {
	Subnet  string        `json:"subnet,omitempty"`
	Gateway net.IP        `json:"gateway,omitempty"`
	Routes  []types.Route `json:"routes"`
}

type netConf struct {
	IPAM *ipamConf `json:"ipam"`
}

func loadIPAMConf(stdinData []byte) (*ipamConf, error) {
	var conf netConf
	return conf.IPAM, json.Unmarshal(stdinData, &conf)
}
