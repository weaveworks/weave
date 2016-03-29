package plugin

import (
	"fmt"
	"sync"

	"github.com/docker/libnetwork/drivers/remote/api"
	"github.com/docker/libnetwork/types"

	"github.com/vishvananda/netlink"
	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/common/odp"
	"github.com/weaveworks/weave/plugin/skel"
)

const (
	WeaveBridge = "weave"
)

type driver struct {
	scope            string
	noMulticastRoute bool
	sync.RWMutex
	endpoints map[string]struct{}
}

func New(client *docker.Client, weave *weaveapi.Client, scope string, noMulticastRoute bool) (skel.Driver, error) {
	driver := &driver{
		noMulticastRoute: noMulticastRoute,
		scope:            scope,
		endpoints:        make(map[string]struct{}),
	}

	_, err := NewWatcher(client, weave, driver)
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// === protocol handlers

func (driver *driver) GetCapabilities() (*api.GetCapabilityResponse, error) {
	driver.logReq("GetCapabilities", nil, "")
	var caps = &api.GetCapabilityResponse{
		Scope: driver.scope,
	}
	driver.logRes("GetCapabilities", caps)
	return caps, nil
}

func (driver *driver) CreateNetwork(create *api.CreateNetworkRequest) error {
	driver.logReq("CreateNetwork", create, create.NetworkID)
	return nil
}

func (driver *driver) DeleteNetwork(delete *api.DeleteNetworkRequest) error {
	driver.logReq("DeleteNetwork", delete, delete.NetworkID)
	return nil
}

func (driver *driver) CreateEndpoint(create *api.CreateEndpointRequest) (*api.CreateEndpointResponse, error) {
	driver.logReq("CreateEndpoint", create, create.EndpointID)
	endID := create.EndpointID

	if create.Interface == nil {
		return nil, driver.error("CreateEndpoint", "Not supported: creating an interface from within CreateEndpoint")
	}
	driver.Lock()
	driver.endpoints[endID] = struct{}{}
	driver.Unlock()
	resp := &api.CreateEndpointResponse{}

	driver.logRes("CreateEndpoint", resp)
	return resp, nil
}

func (driver *driver) DeleteEndpoint(deleteReq *api.DeleteEndpointRequest) error {
	driver.logReq("DeleteEndpoint", deleteReq, deleteReq.EndpointID)
	driver.Lock()
	delete(driver.endpoints, deleteReq.EndpointID)
	driver.Unlock()
	return nil
}

func (driver *driver) HasEndpoint(endpointID string) bool {
	driver.Lock()
	_, found := driver.endpoints[endpointID]
	driver.Unlock()
	return found
}

func (driver *driver) EndpointInfo(req *api.EndpointInfoRequest) (*api.EndpointInfoResponse, error) {
	driver.logReq("EndpointInfo", req, req.EndpointID)
	return &api.EndpointInfoResponse{Value: map[string]interface{}{}}, nil
}

func (driver *driver) JoinEndpoint(j *api.JoinRequest) (*api.JoinResponse, error) {
	driver.logReq("JoinEndpoint", j, fmt.Sprintf("%s:%s to %s", j.NetworkID, j.EndpointID, j.SandboxKey))

	local, err := createAndAttach(j.EndpointID, WeaveBridge, 0)
	if err != nil {
		return nil, driver.error("JoinEndpoint", "%s", err)
	}

	if err := netlink.LinkSetUp(local); err != nil {
		return nil, driver.error("JoinEndpoint", "unable to bring up veth %s-%s: %s", local.Name, local.PeerName, err)
	}

	ifname := &api.InterfaceName{
		SrcName:   local.PeerName,
		DstPrefix: "ethwe",
	}

	response := &api.JoinResponse{
		InterfaceName: ifname,
	}
	if !driver.noMulticastRoute {
		multicastRoute := api.StaticRoute{
			Destination: "224.0.0.0/4",
			RouteType:   types.CONNECTED,
		}
		response.StaticRoutes = append(response.StaticRoutes, multicastRoute)
	}
	driver.logRes("JoinEndpoint", response)
	return response, nil
}

// create and attach local name to the Weave bridge
func createAndAttach(id, bridgeName string, mtu int) (*netlink.Veth, error) {
	maybeBridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, fmt.Errorf(`bridge "%s" not present; did you launch weave?`, bridgeName)
	}

	local := vethPair(id[:5])
	if mtu == 0 {
		local.Attrs().MTU = maybeBridge.Attrs().MTU
	} else {
		local.Attrs().MTU = mtu
	}
	if err := netlink.LinkAdd(local); err != nil {
		return nil, fmt.Errorf(`could not create veth pair %s-%s: %s`, local.Name, local.PeerName, err)
	}

	switch maybeBridge.(type) {
	case *netlink.Bridge:
		if err := netlink.LinkSetMasterByIndex(local, maybeBridge.Attrs().Index); err != nil {
			return nil, fmt.Errorf(`unable to set master of %s: %s`, local.Name, err)
		}
	case *netlink.GenericLink:
		if maybeBridge.Type() != "openvswitch" {
			return nil, fmt.Errorf(`device "%s" is of type "%s"`, bridgeName, maybeBridge.Type())
		}
		if err := odp.AddDatapathInterface(bridgeName, local.Name); err != nil {
			return nil, fmt.Errorf(`failed to attach %s to device "%s": %s`, local.Name, bridgeName, err)
		}
	case *netlink.Device:
		// Assume it's our openvswitch device, and the kernel has not been updated to report the kind.
		if err := odp.AddDatapathInterface(bridgeName, local.Name); err != nil {
			return nil, fmt.Errorf(`failed to attach %s to device "%s": %s`, local.Name, bridgeName, err)
		}
	default:
		return nil, fmt.Errorf(`device "%s" is not a bridge`, bridgeName)
	}
	return local, nil
}

func (driver *driver) LeaveEndpoint(leave *api.LeaveRequest) error {
	driver.logReq("LeaveEndpoint", leave, fmt.Sprintf("%s:%s", leave.NetworkID, leave.EndpointID))

	local := vethPair(leave.EndpointID[:5])
	if err := netlink.LinkDel(local); err != nil {
		driver.warn("LeaveEndpoint", "unable to delete veth: %s", err)
	}
	return nil
}

func (driver *driver) DiscoverNew(disco *api.DiscoveryNotification) error {
	driver.logReq("DiscoverNew", disco, "")
	return nil
}

func (driver *driver) DiscoverDelete(disco *api.DiscoveryNotification) error {
	driver.logReq("DiscoverDelete", disco, "")
	return nil
}

// ===

func vethPair(suffix string) *netlink.Veth {
	return &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: "vethwl" + suffix},
		PeerName:  "vethwg" + suffix,
	}
}

// logging

func (driver *driver) logReq(fun string, req interface{}, short string) {
	driver.log(common.Log.Debugf, " %+v", fun, req)
	common.Log.Infof("[net] %s %s", fun, short)
}

func (driver *driver) logRes(fun string, res interface{}) {
	driver.log(common.Log.Debugf, " %+v", fun, res)
}

func (driver *driver) warn(fun string, format string, a ...interface{}) {
	driver.log(common.Log.Warnf, ": "+format, fun, a...)
}

func (driver *driver) debug(fun string, format string, a ...interface{}) {
	driver.log(common.Log.Debugf, ": "+format, fun, a...)
}

func (driver *driver) error(fun string, format string, a ...interface{}) error {
	driver.log(common.Log.Errorf, ": "+format, fun, a...)
	return fmt.Errorf(format, a...)
}

func (driver *driver) log(f func(string, ...interface{}), format string, fun string, a ...interface{}) {
	f("[net] %s"+format, append([]interface{}{fun}, a...)...)
}
