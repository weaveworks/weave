package ipamplugin

import (
	"net"

	"github.com/docker/libnetwork/ipamapi"
	"github.com/docker/libnetwork/netlabel"
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
	resolver := func() (string, error) { return client.GetContainerIP(WeaveContainer) }
	return &ipam{client: client, weave: api.NewClientWithResolver(resolver)}, nil
}

func (i *ipam) GetDefaultAddressSpaces() (string, string, error) {
	Log.Debugln("GetDefaultAddressSpaces")
	return "weavelocal", "weaveglobal", nil
}

func (i *ipam) RequestPool(addressSpace, pool, subPool string, options map[string]string, v6 bool) (string, *net.IPNet, map[string]string, error) {
	Log.Debugln("RequestPool", addressSpace, pool, subPool, options)
	cidr, err := i.weave.DefaultSubnet()
	Log.Debugln("RequestPool returning ", cidr, err)
	// Pass back a fake "gateway address"; we don't actually use it,
	// so just give the network address.
	data := map[string]string{netlabel.Gateway: cidr.String()}
	return "weavepool", cidr, data, err
}

func (i *ipam) ReleasePool(poolID string) error {
	Log.Debugln("ReleasePool", poolID)
	return nil
}

func (i *ipam) RequestAddress(poolID string, address net.IP, options map[string]string) (*net.IPNet, map[string]string, error) {
	Log.Debugln("RequestAddress", poolID, address, options)
	// Pass magic string to weave IPAM, which then stores the address under its own string
	ip, err := i.weave.AllocateIP("_")
	Log.Debugln("allocateIP returned", ip, err)
	return ip, nil, err
}

func (i *ipam) ReleaseAddress(poolID string, address net.IP) error {
	Log.Debugln("ReleaseAddress", poolID, address)
	return i.weave.ReleaseIP(address.String())
}
