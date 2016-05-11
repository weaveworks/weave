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
	require.Equal(t, NewRange(0, 1), NewRange(0, 1).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 2), NewRange(0, 3).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(1, 1), NewRange(1, 2).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 0x40000000), NewRange(0, 0x7fffffff).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0xfffffffe, 1), NewRange(0xfffffffe, 1).BiggestPow2AlignedRange())
	prop := func(start Address, size Offset) bool {
		if size > Offset(0xffffffff)-Offset(start) { // out of range
			return true
		}
		r := NewRange(start, size)
		result := r.BiggestPow2AlignedRange()
		return r.Contains(result.Start) &&
			r.Contains(result.End-1) &&
			isPower2(result.Size()) &&
			result.Size() > r.Size()/4 &&
			Count(result.Start)%result.Size() == 0
	}
	require.NoError(t, quick.Check(prop, &quick.Config{MaxCount: 1000000}))
}
