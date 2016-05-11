package address

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBiggestPow2AlignedRange(t *testing.T) {
	require.Equal(t, NewRange(0, 1), NewRange(0, 1).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 2), NewRange(0, 2).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 2), NewRange(0, 3).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 4), NewRange(0, 7).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(1, 1), NewRange(1, 1).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(1, 1), NewRange(1, 2).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(2, 2), NewRange(1, 3).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(2, 2), NewRange(1, 4).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(4, 4), NewRange(1, 8).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(4, 4), NewRange(2, 8).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(4, 4), NewRange(4, 8).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(8, 4), NewRange(8, 7).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(8, 8), NewRange(8, 8).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0, 0x4000), NewRange(0, 0x7fff).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0x2000, 2), NewRange(0x2000, 3).BiggestPow2AlignedRange())
	require.Equal(t, NewRange(0xffff, 1), NewRange(0xffff, 1).BiggestPow2AlignedRange())
}
