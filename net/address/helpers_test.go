package address

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	r0to127     = cidr2("10.0.0.0", "10.0.0.127")
	r128to255   = cidr2("10.0.0.128", "10.0.0.255")
	r0to255     = cidr2("10.0.0.0", "10.0.0.255")
	r1dot0to255 = cidr2("10.0.1.0", "10.0.1.255")
	r2dot0to255 = cidr2("10.0.2.0", "10.0.2.255")
)

func TestRemoveCommon(t *testing.T) {
	a := []CIDR{r0to127, r1dot0to255}
	b := []CIDR{r1dot0to255, r2dot0to255}
	newA, newB := RemoveCommon(a, b)
	require.Equal(t, []CIDR{r0to127}, newA)
	require.Equal(t, []CIDR{r2dot0to255}, newB)
}

func TestMerge(t *testing.T) {
	ranges := []Range{
		r0to127.Range(),
		r128to255.Range(),
		r2dot0to255.Range(),
	}
	require.Equal(t, []Range{r0to255.Range(), r2dot0to255.Range()}, Merge(ranges))
}

// Helper

// [start; end]
func cidr2(start, end string) CIDR {
	c := Range{Start: ip(start), End: ip(end) + 1}.CIDRs()
	if len(c) != 1 {
		panic("invalid cidr")
	}
	return c[0]
}
