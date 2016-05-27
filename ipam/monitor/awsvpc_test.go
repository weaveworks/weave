package monitor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
)

var (
	r0to255     = cidr("10.0.0.0", "10.0.0.255")
	r0to3       = cidr("10.0.0.0", "10.0.0.3")
	r2to3       = cidr("10.0.0.2", "10.0.0.3")
	r12to13     = cidr("10.0.0.12", "10.0.0.13")
	r18to19     = cidr("10.0.0.18", "10.0.0.19")
	r22to23     = cidr("10.0.0.22", "10.0.0.23")
	r24to27     = cidr("10.0.0.24", "10.0.0.27")
	r128to255   = cidr("10.0.0.128", "10.0.0.255")
	r1dot0to255 = cidr("10.0.1.0", "10.0.1.255")
)

func TestRemoveCommonNoChanges(t *testing.T) {
	a := []address.CIDR{r0to255}
	b := []address.CIDR{r0to255}
	newA, newB := removeCommon(a, b)
	require.Len(t, newA, 0, "")
	require.Len(t, newB, 0, "")

	a = []address.CIDR{r0to255}
	b = []address.CIDR{r0to3, r128to255}
	newA, newB = removeCommon(a, b)
	require.Equal(t, a, newA, "")
	require.Equal(t, b, newB, "")

	a = []address.CIDR{r0to255}
	b = []address.CIDR{r128to255}
	newA, newB = removeCommon(a, b)
	require.Equal(t, a, newA, "")
	require.Equal(t, b, newB, "")

	a = []address.CIDR{r0to255}
	b = []address.CIDR{r0to3}
	newA, newB = removeCommon(a, b)
	require.Equal(t, a, newA, "")
	require.Equal(t, b, newB, "")
}

func TestRemoveCommon(t *testing.T) {
	a := []address.CIDR{r0to3, r18to19, r22to23, r24to27}
	b := []address.CIDR{r2to3, r12to13, r18to19, r1dot0to255}
	newA, newB := removeCommon(a, b)
	require.Equal(t, []address.CIDR{r0to3, r22to23, r24to27}, newA, "")
	require.Equal(t, []address.CIDR{r2to3, r12to13, r1dot0to255}, newB, "")
}

// Helper

func ip(s string) address.Address {
	addr, _ := address.ParseIP(s)
	return addr
}

// [start; end]
func cidr(start, end string) address.CIDR {
	c := address.Range{Start: ip(start), End: ip(end) + 1}.CIDRs()
	common.AssertWithMsg(len(c) == 1,
		fmt.Sprintf("Multiple CIDRs (%s) from %s to %s!", c, start, end))
	return c[0]
}
