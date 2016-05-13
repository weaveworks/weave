package address

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func isPower2(x Count) bool {
	if x == 0 {
		return false
	}
	for ; x > 1; x /= 2 {
		if x&1 != 0 {
			return false
		}
	}
	return true
}

func TestBiggestPow2AlignedRange(t *testing.T) {
	require.Equal(t, NewRange(0, 1), NewRange(0, 1).BiggestCIDRRange())
	require.Equal(t, NewRange(1, 1), NewRange(1, 2).BiggestCIDRRange())
	require.Equal(t, NewRange(2, 2), NewRange(1, 3).BiggestCIDRRange())
	require.Equal(t, NewRange(0, 0x40000000), NewRange(0, 0x7fffffff).BiggestCIDRRange())
	require.Equal(t, NewRange(0xfffffffe, 1), NewRange(0xfffffffe, 1).BiggestCIDRRange())
	prop := func(start Address, size Offset) bool {
		if size > Offset(0xffffffff)-Offset(start) { // out of range
			return true
		}
		r := NewRange(start, size)
		result := r.BiggestCIDRRange()
		return r.Contains(result.Start) &&
			r.Contains(result.End-1) &&
			isPower2(result.Size()) &&
			result.Size() > r.Size()/4 &&
			Count(result.Start)%result.Size() == 0
	}
	require.NoError(t, quick.Check(prop, &quick.Config{MaxCount: 1000000}))
}
