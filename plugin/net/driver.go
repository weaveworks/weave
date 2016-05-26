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
	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/plugin/skel"
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

	name, peerName := vethPair(j.EndpointID)
	if _, err := weavenet.CreateAndAttachVeth(name, peerName, weavenet.WeaveBridgeName, 0, nil); err != nil {
		return nil, driver.error("JoinEndpoint", "%s", err)
	}

	response := &api.JoinResponse{
		InterfaceName: &api.InterfaceName{
			SrcName:   peerName,
			DstPrefix: weavenet.VethName,
		},
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

func (driver *driver) LeaveEndpoint(leave *api.LeaveRequest) error {
	driver.logReq("LeaveEndpoint", leave, fmt.Sprintf("%s:%s", leave.NetworkID, leave.EndpointID))

	name, _ := vethPair(leave.EndpointID)
	veth := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkDel(veth); err != nil {
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

func vethPair(id string) (string, string) {
	return "vethwl" + id[:5], "vethwg" + id[:5]
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
