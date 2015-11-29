package ipamplugin

import (
	"fmt"
	"net"

	"github.com/docker/libnetwork/ipamapi"
	"github.com/weaveworks/weave/api"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
)

const (
	WeaveContainer = "weave"
)

type ipam struct {
	client *docker.Client
	weave  *api.Client
}

func NewIpam(client *docker.Client, version string) (ipamapi.Ipam, error) {
	return &ipam{client: client}, nil
}

func (i *ipam) configureWeaveClient() error {
	if i.weave != nil {
		return nil
	}
	ip, err := i.client.GetContainerIP(WeaveContainer)
	if err != nil {
		Log.Warningf("weave ipam not available: %s", err)
		return fmt.Errorf("weave ipam not available: %s", err)
	}
	i.weave = api.NewClient(ip)
	return nil
}

func (i *ipam) GetDefaultAddressSpaces() (string, string, error) {
	Log.Debugln("GetDefaultAddressSpaces")
	return "weavelocal", "weaveglobal", nil
}

func (i *ipam) RequestPool(addressSpace, pool, subPool string, options map[string]string, v6 bool) (string, *net.IPNet, map[string]string, error) {
	Log.Debugln("RequestPool", addressSpace, pool, subPool, options)
	if err := i.configureWeaveClient(); err == nil {
		cidr, err := i.weave.DefaultSubnet()
		Log.Debugln("RequestPool returning ", cidr, err)
		return "weavepool", cidr, nil, err
	} else {
		return "", nil, nil, err
	}
}

func (i *ipam) ReleasePool(poolID string) error {
	Log.Debugln("ReleasePool", poolID)
	return nil
}

func (i *ipam) RequestAddress(poolID string, address net.IP, options map[string]string) (*net.IPNet, map[string]string, error) {
	Log.Debugln("RequestAddress", poolID, address, options)
	// Pass magic string to weave IPAM, which then stores the address under its own string
	if err := i.configureWeaveClient(); err == nil {
		ip, err := i.weave.AllocateIP("_")
		Log.Debugln("allocateIP returned", ip, err)
		return ip, nil, err
	} else {
		return nil, nil, err
	}
}

func (i *ipam) ReleaseAddress(poolID string, address net.IP) error {
	Log.Debugln("ReleaseAddress", poolID, address)
	if err := i.configureWeaveClient(); err == nil {
		return i.weave.ReleaseIP(address.String())
	} else {
		return err
	}
}
