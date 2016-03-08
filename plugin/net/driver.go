package plugin

import (
	"fmt"
	"sync"

	"github.com/docker/libnetwork/drivers/remote/api"
	"github.com/docker/libnetwork/types"

	weaveapi "github.com/weaveworks/weave/api"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/common/odp"
	"github.com/weaveworks/weave/plugin/skel"

	"github.com/vishvananda/netlink"
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

func errorf(format string, a ...interface{}) error {
	Log.Errorf(format, a...)
	return fmt.Errorf(format, a...)
}

// === protocol handlers

func (driver *driver) GetCapabilities() (*api.GetCapabilityResponse, error) {
	var caps = &api.GetCapabilityResponse{
		Scope: driver.scope,
	}
	Log.Debugf("Get capabilities: responded with %+v", caps)
	return caps, nil
}

func (driver *driver) CreateNetwork(create *api.CreateNetworkRequest) error {
	Log.Debugf("Create network request %+v", create)
	Log.Infof("Create network %s", create.NetworkID)
	return nil
}

func (driver *driver) DeleteNetwork(delete *api.DeleteNetworkRequest) error {
	Log.Debugf("Delete network request: %+v", delete)
	Log.Infof("Destroy network %s", delete.NetworkID)
	return nil
}

func (driver *driver) CreateEndpoint(create *api.CreateEndpointRequest) (*api.CreateEndpointResponse, error) {
	Log.Debugf("Create endpoint request %+v", create)
	endID := create.EndpointID

	if create.Interface == nil {
		return nil, fmt.Errorf("Not supported: creating an interface from within CreateEndpoint")
	}
	driver.Lock()
	driver.endpoints[endID] = struct{}{}
	driver.Unlock()
	resp := &api.CreateEndpointResponse{}

	Log.Infof("Create endpoint %s %+v", endID, resp)
	return resp, nil
}

func (driver *driver) DeleteEndpoint(deleteReq *api.DeleteEndpointRequest) error {
	Log.Debugf("Delete endpoint request: %+v", deleteReq)
	Log.Infof("Delete endpoint %s", deleteReq.EndpointID)
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
	Log.Debugf("Endpoint info request: %+v", req)
	Log.Infof("Endpoint info %s", req.EndpointID)
	return &api.EndpointInfoResponse{Value: map[string]interface{}{}}, nil
}

func (driver *driver) JoinEndpoint(j *api.JoinRequest) (*api.JoinResponse, error) {
	local, err := createAndAttach(j.EndpointID, WeaveBridge, 0)
	if err != nil {
		return nil, err
	}

	if err := netlink.LinkSetUp(local); err != nil {
		return nil, errorf(`unable to bring veth up: %s`, err)
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
	Log.Infof("Join endpoint %s:%s to %s", j.NetworkID, j.EndpointID, j.SandboxKey)
	return response, nil
}

// create and attach local name to the Weave bridge
func createAndAttach(id, bridgeName string, mtu int) (*netlink.Veth, error) {
	maybeBridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil, errorf(`bridge "%s" not present; did you launch weave?`, bridgeName)
	}

	local := vethPair(id[:5])
	if mtu == 0 {
		local.Attrs().MTU = maybeBridge.Attrs().MTU
	} else {
		local.Attrs().MTU = mtu
	}
	if err := netlink.LinkAdd(local); err != nil {
		return nil, errorf("could not create veth pair: %s", err)
	}

	switch maybeBridge.(type) {
	case *netlink.Bridge:
		if err := netlink.LinkSetMasterByIndex(local, maybeBridge.Attrs().Index); err != nil {
			return nil, errorf(`unable to set master: %s`, err)
		}
	case *netlink.GenericLink:
		if maybeBridge.Type() != "openvswitch" {
			return nil, errorf(`device "%s" is of type "%s"`, bridgeName, maybeBridge.Type())
		}
		odp.AddDatapathInterface(bridgeName, local.Name)
	case *netlink.Device:
		Log.Warnf("kernel does not report what kind of device %s is, just %+v", bridgeName, maybeBridge)
		// Assume it's our openvswitch device, and the kernel has not been updated to report the kind.
		odp.AddDatapathInterface(bridgeName, local.Name)
	default:
		return nil, errorf(`device "%s" not a bridge`, bridgeName)
	}
	return local, nil
}

func (driver *driver) LeaveEndpoint(leave *api.LeaveRequest) error {
	Log.Debugf("Leave request: %+v", leave)

	local := vethPair(leave.EndpointID[:5])
	if err := netlink.LinkDel(local); err != nil {
		Log.Warningf("unable to delete veth on leave: %s", err)
	}
	Log.Infof("Leave %s:%s", leave.NetworkID, leave.EndpointID)
	return nil
}

func (driver *driver) DiscoverNew(disco *api.DiscoveryNotification) error {
	Log.Debugf("Dicovery new notification: %+v", disco)
	return nil
}

func (driver *driver) DiscoverDelete(disco *api.DiscoveryNotification) error {
	Log.Debugf("Dicovery delete notification: %+v", disco)
	return nil
}

// ===

func vethPair(suffix string) *netlink.Veth {
	return &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: "vethwl" + suffix},
		PeerName:  "vethwg" + suffix,
	}
}
