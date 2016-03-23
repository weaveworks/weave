package monitor

// TODO(mp) docs

import (
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/vishvananda/netlink"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
)

type AWSVPCMonitor struct {
	ec2          *ec2.EC2
	instanceID   string
	routeTableID string
	linkIndex    int
}

const (
	bridgeIfName = "weave"
)

// NewAWSVPCMonitor creates and initialises AWS VPC based monitor.
//
// The monitor updates AWS VPC and host route tables when any changes to allocated
// address ranges owner by a peer have been committed.
func NewAWSVPCMonitor() (*AWSVPCMonitor, error) {
	var err error
	session := session.New()
	mon := &AWSVPCMonitor{}

	// Detect region and instance id
	meta := ec2metadata.New(session)
	mon.instanceID, err = meta.GetMetadata("instance-id")
	if err != nil {
		return nil, fmt.Errorf("cannot detect instance-id: %s", err)
	}
	region, err := meta.Region()
	if err != nil {
		return nil, fmt.Errorf("cannot detect region: %s", err)
	}

	mon.ec2 = ec2.New(session, aws.NewConfig().WithRegion(region))

	routeTableID, err := mon.detectRouteTableID()
	if err != nil {
		return nil, err
	}
	mon.routeTableID = *routeTableID

	// Detect Weave bridge link index
	link, err := netlink.LinkByName(bridgeIfName)
	if err != nil {
		return nil, fmt.Errorf("cannot find \"%s\" interface: %s", bridgeIfName, err)
	}
	mon.linkIndex = link.Attrs().Index

	common.Log.Debugf(
		"AWSVPC monitor has been initialized on %s instance for %s route table at %s region",
		mon.instanceID, mon.routeTableID, region)

	return mon, nil
}

// HandleUpdate method updates the AWS VPC and the host route tables.
func (mon *AWSVPCMonitor) HandleUpdate(old, new []address.Range) error {
	oldCIDRs, newCIDRs := filterOutSameCIDRs(address.NewCIDRs(old), address.NewCIDRs(new))
	common.Log.Debugf("HandleUpdate: old(%q) new(%q)", old, new)

	// It might make sense to do removal first and then add entries
	// because of the 50 routes limit. However, in such case a container might
	// not be reachable for short period of time which we we would like to
	// avoid.

	// Add new entries
	for _, cidr := range newCIDRs {
		cidrStr := cidr.String()
		common.Log.Debugf("Creating %s route to %s", cidrStr, mon.instanceID)
		_, err := mon.createVPCRoute(cidrStr)
		// TODO(mp) check for 50 routes limit
		// TODO(mp) maybe check for auth related errors
		if err != nil {
			return fmt.Errorf("createVPCRoutes failed: %s", err)
		}
		err = mon.createHostRoute(cidrStr)
		if err != nil {
			return fmt.Errorf("createHostRoute failed: %s", err)
		}
	}

	// Remove obsolete entries
	for _, cidr := range oldCIDRs {
		cidrStr := cidr.String()
		common.Log.Debugf("Removing %s route", cidrStr)
		_, err := mon.deleteVPCRoute(cidrStr)
		if err != nil {
			return fmt.Errorf("deleteVPCRoute failed: %s", err)
		}
		err = mon.deleteHostRoute(cidrStr)
		if err != nil {
			return fmt.Errorf("deleteHostRoute failed: %s", err)
		}
	}

	return nil
}

func (mon *AWSVPCMonitor) String() string {
	return "awsvpc"
}

func (mon *AWSVPCMonitor) createVPCRoute(cidr string) (*ec2.CreateRouteOutput, error) {
	route := &ec2.CreateRouteInput{
		RouteTableId:         &mon.routeTableID,
		InstanceId:           &mon.instanceID,
		DestinationCidrBlock: &cidr,
	}
	return mon.ec2.CreateRoute(route)
}

func (mon *AWSVPCMonitor) createHostRoute(cidr string) error {
	dst, err := parseCIDR(cidr)
	if err != nil {
		return err
	}
	route := &netlink.Route{
		LinkIndex: mon.linkIndex,
		Dst:       dst,
		Scope:     netlink.SCOPE_LINK,
	}
	return netlink.RouteAdd(route)
}

func (mon *AWSVPCMonitor) deleteVPCRoute(cidr string) (*ec2.DeleteRouteOutput, error) {
	route := &ec2.DeleteRouteInput{
		RouteTableId:         &mon.routeTableID,
		DestinationCidrBlock: &cidr,
	}
	return mon.ec2.DeleteRoute(route)
}

func (mon *AWSVPCMonitor) deleteHostRoute(cidr string) error {
	dst, err := parseCIDR(cidr)
	if err != nil {
		return err
	}
	route := &netlink.Route{
		LinkIndex: mon.linkIndex,
		Dst:       dst,
		Scope:     netlink.SCOPE_LINK,
	}
	return netlink.RouteDel(route)
}

func (mon *AWSVPCMonitor) detectRouteTableID() (*string, error) {
	instancesParams := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(mon.instanceID)},
	}
	instancesResp, err := mon.ec2.DescribeInstances(instancesParams)
	if err != nil {
		return nil, fmt.Errorf("DescribeInstances failed: %s", err)
	}
	if len(instancesResp.Reservations) == 0 ||
		len(instancesResp.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf(
			"cannot find %s instance within reservations", mon.instanceID)
	}
	vpcID := instancesResp.Reservations[0].Instances[0].VpcId
	subnetID := instancesResp.Reservations[0].Instances[0].SubnetId

	tablesParams := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{subnetID},
			},
		},
	}
	tablesResp, err := mon.ec2.DescribeRouteTables(tablesParams)
	if err != nil {
		return nil, fmt.Errorf("DescribeRouteTables failed: %s", err)
	}
	if len(tablesResp.RouteTables) != 0 {
		return tablesResp.RouteTables[0].RouteTableId, nil
	}
	tablesParams = &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.main"),
				Values: []*string{aws.String("true")},
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpcID},
			},
		},
	}
	tablesResp, err = mon.ec2.DescribeRouteTables(tablesParams)
	if err != nil {
		return nil, fmt.Errorf("DescribeRouteTables failed: %s", err)
	}
	if len(tablesResp.RouteTables) != 0 {
		return tablesResp.RouteTables[0].RouteTableId, nil
	}

	return nil, fmt.Errorf("cannot find routetable for %s instance", mon.instanceID)
}

// Only for debugging
func (mon *AWSVPCMonitor) printRoutes() error {
	link, err := netlink.LinkByIndex(mon.linkIndex)
	if err != nil {
		return err
	}
	routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
	if err != nil {
		return err
	}

	common.Log.Infof("Routes: %q", routes)
	return nil
}

// Helpers

// filterOutSameCIDRs filters out CIDR ranges which are contained in both new
// and old slices.
func filterOutSameCIDRs(old, new []address.CIDR) (filteredOld, filteredNew []address.CIDR) {
	i, j := 0, 0
	for i < len(old) && j < len(new) {
		switch {
		case old[i].Start() == new[j].Start() && old[i].End() == new[j].End():
			i++
			j++
			continue
		case old[i].End() < new[j].End():
			filteredOld = append(filteredOld, old[i])
			i++
		default:
			filteredNew = append(filteredNew, new[j])
			j++
		}
	}
	filteredOld = append(filteredOld, old[i:]...)
	filteredNew = append(filteredNew, new[j:]...)

	return filteredOld, filteredNew
}

func parseCIDR(cidr string) (*net.IPNet, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ipnet.IP = ip

	return ipnet, nil
}
