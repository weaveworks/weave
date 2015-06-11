package net

import (
	"net"
	"testing"

	wt "github.com/weaveworks/weave/testing"
)

func TestOverlap(t *testing.T) {
	_, subnet1, _ := net.ParseCIDR("10.0.3.0/24")
	_, subnet2, _ := net.ParseCIDR("10.0.4.0/24")
	_, subnet3, _ := net.ParseCIDR("10.0.3.128/25")
	_, subnet4, _ := net.ParseCIDR("10.0.3.192/25")
	_, universe, _ := net.ParseCIDR("10.0.0.0/8")
	wt.AssertEquals(t, overlaps(subnet1, subnet2), false)
	wt.AssertEquals(t, overlaps(subnet2, subnet1), false)
	wt.AssertEquals(t, overlaps(subnet1, subnet1), true)
	wt.AssertEquals(t, overlaps(subnet1, subnet3), true)
	wt.AssertEquals(t, overlaps(subnet1, subnet4), true)
	wt.AssertEquals(t, overlaps(subnet2, subnet4), false)
	wt.AssertEquals(t, overlaps(subnet4, subnet2), false)
	wt.AssertEquals(t, overlaps(universe, subnet1), true)
	wt.AssertEquals(t, overlaps(subnet1, universe), true)
}
