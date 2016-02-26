package ipamplugin

import (
	"fmt"
	"net"
	"strings"

	ipamapi "github.com/docker/go-plugins-helpers/ipam"
	"github.com/docker/libnetwork/netlabel"
	"github.com/weaveworks/weave/api"
	. "github.com/weaveworks/weave/common"
)

type ipam struct {
	weave *api.Client
}

func NewIpam(weave *api.Client) ipamapi.Ipam {
	return &ipam{weave: weave}
}

func (i *ipam) GetCapabilities() (*ipamapi.CapabilitiesResponse, error) {
	Log.Debugln("GetCapabilities")
	return &ipamapi.CapabilitiesResponse{RequiresMACAddress: false}, nil
}

func (i *ipam) GetDefaultAddressSpaces() (*ipamapi.AddressSpacesResponse, error) {
	Log.Debugln("GetDefaultAddressSpaces")
	return &ipamapi.AddressSpacesResponse{
		LocalDefaultAddressSpace:  "weavelocal",
		GlobalDefaultAddressSpace: "weaveglobal",
	}, nil
}

func (i *ipam) RequestPool(req *ipamapi.RequestPoolRequest) (res *ipamapi.RequestPoolResponse, err error) {
	Log.Debugln("RequestPool", req)
	var subnet *net.IPNet
	res = &ipamapi.RequestPoolResponse{}
	defer func() { Log.Debugln("RequestPool returning", res, err) }()
	if req.Pool == "" {
		subnet, err = i.weave.DefaultSubnet()
	} else {
		_, subnet, err = net.ParseCIDR(req.Pool)
	}
	if err != nil {
		return
	}
	iprange := subnet
	if req.SubPool != "" {
		if _, iprange, err = net.ParseCIDR(req.SubPool); err != nil {
			return
		}
	}
	// Cunningly-constructed pool "name" which gives us what we need later
	res.PoolID = strings.Join([]string{"weave", subnet.String(), iprange.String()}, "-")
	res.Pool = subnet.String()
	// Pass back a fake "gateway address"; we don't actually use it,
	// so just give the network address.
	res.Data = map[string]string{netlabel.Gateway: subnet.String()}
	return
}

func (i *ipam) ReleasePool(req *ipamapi.ReleasePoolRequest) error {
	Log.Debugln("ReleasePool", req)
	return nil
}

func (i *ipam) RequestAddress(req *ipamapi.RequestAddressRequest) (res *ipamapi.RequestAddressResponse, err error) {
	Log.Debugln("RequestAddress", req)
	address := net.ParseIP(req.Address)
	var ip *net.IPNet
	res = &ipamapi.RequestAddressResponse{}
	defer func() { Log.Debugln("allocateIP returned", res, err) }()
	// If we pass magic string "_" to weave IPAM it stores the address under its own string
	if req.PoolID == "weavepool" { // old-style
		ip, err = i.weave.AllocateIP("_")
		if err != nil {
			return
		}
		res.Address = ip.String()
		return
	}
	parts := strings.Split(req.PoolID, "-")
	if len(parts) != 3 || parts[0] != "weave" {
		err = fmt.Errorf("Unrecognized pool ID: %s", req.PoolID)
		return
	}
	var subnet, iprange *net.IPNet
	if _, subnet, err = net.ParseCIDR(parts[1]); err != nil {
		return
	}
	if address != nil { // try to claim specific address requested
		if err = i.weave.ClaimIP("_", address); err != nil {
			return
		}
		ip = &net.IPNet{IP: address, Mask: subnet.Mask}
	} else {
		if _, iprange, err = net.ParseCIDR(parts[2]); err != nil {
			return
		}
		// We are lying slightly to IPAM here: the range is not a subnet
		if ip, err = i.weave.AllocateIPInSubnet("_", iprange); err != nil {
			return
		}
		ip.Mask = subnet.Mask // fix up the subnet we lied about
	}
	res.Address = ip.String()
	return
}

func (i *ipam) ReleaseAddress(req *ipamapi.ReleaseAddressRequest) error {
	Log.Debugln("ReleaseAddress", req.PoolID, req.Address)
	return i.weave.ReleaseIP(req.Address)
}
