package ipamplugin

import (
	"crypto/rand"
	"fmt"
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

type poolData struct {
	subnet  *net.IPNet
	subPool *net.IPNet
}

type ipam struct {
	client *docker.Client
	weave  *api.Client
	pools  map[string]poolData
}

func NewIpam(client *docker.Client, version string) (ipamapi.Ipam, error) {
	resolver := func() (string, error) { return client.GetContainerIP(WeaveContainer) }
	return &ipam{client: client, weave: api.NewClientWithResolver(resolver)}, nil
}

func (i *ipam) GetDefaultAddressSpaces() (string, string, error) {
	Log.Debugln("GetDefaultAddressSpaces")
	return "weavelocal", "weaveglobal", nil
}

// Address space is a name denoting a set of pools.
// optional pool gives the range of addresses in CIDR notation.
// - this maps to the `--subnet` option on `docker network create`
// optional subPool indicates a smaller range of addresses within pool
// - this maps to the `--ip-range` option on `docker network create`
func (i *ipam) RequestPool(addressSpace, pool, subPool string, options map[string]string, v6 bool) (poolID string, cidr *net.IPNet, data map[string]string, err error) {
	Log.Debugln("RequestPool", addressSpace, pool, subPool, options)
	defer Log.Debugln("RequestPool returning ", cidr, err)
	poolID = randomID()
	var subPoolCidr *net.IPNet
	if pool != "" {
		_, cidr, err = net.ParseCIDR(pool)
		if err != nil {
			return
		}
		if subPool != "" {
			_, subPoolCidr, err = net.ParseCIDR(subPool)
			if err != nil {
				return
			}
		} else {
			subPoolCidr = cidr
		}
	} else {
		cidr, err = i.weave.DefaultSubnet()
	}
	i.pools[poolID] = poolData{subnet: cidr, subPool: subPoolCidr}
	// Pass back a fake "gateway address"; we don't actually use it,
	// so just give the network address.
	data = map[string]string{netlabel.Gateway: cidr.String()}
	return
}

func (i *ipam) ReleasePool(poolID string) error {
	Log.Debugln("ReleasePool", poolID)
	return nil
}

func (i *ipam) RequestAddress(poolID string, address net.IP, options map[string]string) (*net.IPNet, map[string]string, error) {
	Log.Debugln("RequestAddress", poolID, address, options)
	pool, found := i.pools[poolID]
	if !found {
		return nil, nil, fmt.Errorf("Unrecognized pool %s", poolID)
	}
	// Pass magic string to weave IPAM, which then stores the address under its own string
	ip, err := i.weave.AllocateIPInRange("_", pool.subPool)
	if err != nil {
		ip.Mask = pool.subnet.Mask
	}
	Log.Debugln("allocateIP returned", ip, err)
	return ip, nil, err
}

func (i *ipam) ReleaseAddress(poolID string, address net.IP) error {
	Log.Debugln("ReleaseAddress", poolID, address)
	return i.weave.ReleaseIP(address.String())
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%X", b)
}

// Two networks overlap if the start-point of one is inside the other.
func overlaps(n1, n2 *net.IPNet) bool {
	return n1.Contains(n2.IP) || n2.Contains(n1.IP)
}
