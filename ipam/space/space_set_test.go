package space

import (
	"net"
	"testing"

	"github.com/zettio/weave/common"
	"github.com/zettio/weave/ipam/utils"
	wt "github.com/zettio/weave/testing"
)

func equal(ms1 *Space, ms2 *Space) bool {
	return ms1.Start.Equal(ms2.Start) &&
		ms1.Size == ms2.Size
}

// Note: does not check version
func (ps1 *Set) Equal(ps2 *Set) bool {
	if len(ps1.spaces) == len(ps2.spaces) {
		for i := 0; i < len(ps1.spaces); i++ {
			if !equal(ps1.spaces[i], ps2.spaces[i]) {
				return false
			}
		}
		return true
	}
	return false
}

func spaceSetWith(spaces ...Space) *Set {
	ps := Set{}
	for _, space := range spaces {
		ps.AddSpace(space)
	}
	return &ps
}

func TestGiveUpSimple(t *testing.T) {
	const (
		testAddr1 = "10.0.1.0"
		testAddr2 = "10.0.1.32"
	)

	var (
		ipAddr1 = net.ParseIP(testAddr1)
	)

	ps1 := spaceSetWith(Space{Start: ipAddr1, Size: 48})

	// Empty space set should split in two and give me the second half
	start, numGivenUp, ok := ps1.GiveUpSpace()
	wt.AssertBool(t, ok, true, "GiveUpSpace result")
	wt.AssertTrue(t, start.Equal(net.ParseIP("10.0.1.24")), "Invalid start")
	wt.AssertEqualUint32(t, numGivenUp, 24, "GiveUpSpace 1 size")
	wt.AssertEqualUint32(t, ps1.NumFreeAddresses(), 24, "num free addresses")

	// Now check we can give the rest up.
	count := 0 // count to avoid infinite loop
	for ; count < 1000; count++ {
		_, size, ok := ps1.GiveUpSpace()
		if !ok {
			break
		}
		numGivenUp += size
	}
	wt.AssertEqualUint32(t, ps1.NumFreeAddresses(), 0, "num free addresses")
	wt.AssertEqualUint32(t, numGivenUp, 48, "total space given up")
}

func TestGiveUpHard(t *testing.T) {
	common.InitDefaultLogging(true)
	var (
		start        = net.ParseIP("10.0.1.0")
		size  uint32 = 48
	)

	// Fill a fresh space set
	spaceset := spaceSetWith(Space{Start: start, Size: size})
	for i := uint32(0); i < size; i++ {
		ip := spaceset.Allocate()
		wt.AssertTrue(t, ip != nil, "Failed to get IP!")
	}

	// Now free all but the last address
	// this will force us to split the free list
	for i := uint32(0); i < size-1; i++ {
		wt.AssertSuccess(t, spaceset.Free(utils.Add(start, i)))
	}

	// Now split
	newRange, numGivenUp, ok := spaceset.GiveUpSpace()
	wt.AssertBool(t, ok, true, "GiveUpSpace result")
	wt.AssertTrue(t, newRange.Equal(net.ParseIP("10.0.1.23")), "Invalid start")
	wt.AssertEqualUint32(t, numGivenUp, 24, "GiveUpSpace 1 size")
	wt.AssertEqualUint32(t, spaceset.NumFreeAddresses(), 23, "num free addresses")

	//Space set should now have 2 spaces
	expected := spaceSetWith(Space{Start: start, Size: 23},
		Space{Start: net.ParseIP("10.0.1.47"), Size: 1})
	wt.AssertTrue(t, spaceset.Equal(expected), "Wrong sets")
}
