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
	wnet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
)

type AWSVPCMonitor struct {
	ec2          *ec2.EC2
	instanceID   string // EC2 Instance ID
	routeTableID string // VPC Route Table ID
	linkIndex    int    // The weave bridge link index
}

// NewAWSVPCMonitor creates and initialises AWS VPC based monitor.
//
// The monitor updates AWS VPC and host route tables when any changes to allocated
// address ranges owned by a peer have been done.
func NewAWSVPCMonitor() (*AWSVPCMonitor, error) {
	var (
		err     error
		session = session.New()
		mon     = &AWSVPCMonitor{}
	)

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
	link, err := netlink.LinkByName(wnet.WeaveBridgeName)
	if err != nil {
		return nil, fmt.Errorf("cannot find \"%s\" interface: %s", wnet.WeaveBridgeName, err)
	}
	mon.linkIndex = link.Attrs().Index

	mon.infof("AWSVPC has been initialized on %s instance for %s route table at %s region",
		mon.instanceID, mon.routeTableID, region)

	return mon, nil
}

// HandleUpdate method updates the AWS VPC and the host route tables.
func (mon *AWSVPCMonitor) HandleUpdate(prevRanges, currRanges []address.Range) error {
	mon.debugf("replacing %q entries by %q", prevRanges, currRanges)

	prev, curr := removeCommon(address.NewCIDRs(prevRanges), address.NewCIDRs(currRanges))

	// It might make sense to do the removal first and then add entries
	// because of the 50 routes limit. However, in such case a container might
	// not be reachable for a short period of time which is not a desired behavior.

	// Add new entries
	for _, cidr := range curr {
		cidrStr := cidr.String()
		mon.debugf("adding route %s to %s", cidrStr, mon.instanceID)
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
	for _, cidr := range prev {
		cidrStr := cidr.String()
		mon.debugf("removing %s route", cidrStr)
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

// detectRouteTableID detects AWS VPC Route Table ID of the given monitor instance.
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
		return nil, fmt.Errorf("cannot find %s instance within reservations", mon.instanceID)
	}
	vpcID := instancesResp.Reservations[0].Instances[0].VpcId
	subnetID := instancesResp.Reservations[0].Instances[0].SubnetId

	// First try to find a routing table associated with the subnet of the instance
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
	// Fallback to the default routing table
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

func (mon *AWSVPCMonitor) debugf(fmt string, args ...interface{}) {
	common.Log.Debugf("[monitor] "+fmt, args...)
}

func (mon *AWSVPCMonitor) infof(fmt string, args ...interface{}) {
	common.Log.Infof("[monitor] "+fmt, args...)
}

// Helpers

// removeCommon filters out CIDR ranges which are contained in both a and b slices.
func removeCommon(a, b []address.CIDR) (newA, newB []address.CIDR) {
	i, j := 0, 0

	for i < len(a) && j < len(b) {
		switch {
		case a[i].Start() == b[j].Start() && a[i].End() == b[j].End():
			i++
			j++
			continue
		case a[i].End() < b[j].End():
			newA = append(newA, a[i])
			i++
		default:
			newB = append(newB, b[j])
			j++
		}
	}
	newA = append(newA, a[i:]...)
	newB = append(newB, b[j:]...)

	return
}

func parseCIDR(cidr string) (*net.IPNet, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ipnet.IP = ip

	return ipnet, nil
}
