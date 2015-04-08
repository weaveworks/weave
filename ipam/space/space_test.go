package space

import (
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
)

func TestSpaceAllocate(t *testing.T) {
	const (
		testAddr1   = "10.0.3.4"
		testAddrx   = "10.0.3.19"
		testAddry   = "10.0.9.19"
		containerID = "deadbeef"
	)

	space1 := Space{Start: net.ParseIP(testAddr1), Size: 20}
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 20, "Free addresses")
	space1.assertInvariants()

	addr1 := space1.Allocate()
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 19, "Free addresses")
	space1.assertInvariants()

	addr2 := space1.Allocate()
	wt.AssertNotEqualString(t, addr2.String(), testAddr1, "address")
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 18, "Free addresses")
	space1.assertInvariants()

	space1.Free(addr2)
	space1.assertInvariants()

	wt.AssertErrorInterface(t, space1.Free(addr2), (*error)(nil), "double free")
	wt.AssertErrorInterface(t, space1.Free(net.ParseIP(testAddrx)), (*error)(nil), "address not allocated")
	wt.AssertErrorInterface(t, space1.Free(net.ParseIP(testAddry)), (*error)(nil), "wrong out of range")

	space1.assertInvariants()
}

func TestSpaceFree(t *testing.T) {
	const (
		testAddr1   = "10.0.3.4"
		testAddrx   = "10.0.3.19"
		testAddry   = "10.0.9.19"
		containerID = "deadbeef"
	)

	space := Space{Start: net.ParseIP(testAddr1), Size: 20}

	// Check we are prepared to give up the entire space
	start, size := space.BiggestFreeChunk()
	wt.AssertTrue(t, start.Equal(net.ParseIP(testAddr1)) && size == 20, "Wrong space")

	for i := 0; i < 20; i++ {
		addr := space.Allocate()
		wt.AssertTrue(t, addr != nil, "Failed to get address")
	}

	// Check we are full
	addr := space.Allocate()
	wt.AssertTrue(t, addr == nil, "Should have failed to get address")
	start, size = space.BiggestFreeChunk()
	wt.AssertTrue(t, start == nil && size == 0, "Wrong space")

	// Free in the middle
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.13")))
	start, size = space.BiggestFreeChunk()
	wt.AssertTrue(t, start.Equal(net.ParseIP("10.0.3.13")) && size == 1, "Wrong space")

	// Free one at the end
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.23")))
	start, size = space.BiggestFreeChunk()
	wt.AssertTrue(t, start.Equal(net.ParseIP("10.0.3.23")) && size == 1, "Wrong space")

	// Now free a few at the end
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.22")))
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.21")))

	// These free should have shrunk the free list
	wt.AssertTrue(t, space.MaxAllocated == 17, "Free list didn't shrink!")

	// Now get the biggest free space; should be 3.21
	start, size = space.BiggestFreeChunk()
	wt.AssertTrue(t, start.Equal(net.ParseIP("10.0.3.21")) && size == 3, "Wrong space")

	// Now free a few in the middle
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.12")))
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.11")))
	wt.AssertSuccess(t, space.Free(net.ParseIP("10.0.3.10")))

	// These free should not have shrunk the free list
	wt.AssertTrue(t, space.MaxAllocated == 17, "Free list didn't shrink!")

	// Now get the biggest free space; should be 3.21
	start, size = space.BiggestFreeChunk()
	wt.AssertTrue(t, start.Equal(net.ParseIP("10.0.3.10")) && size == 4, "Wrong space")
}

func TestSpaceSplit(t *testing.T) {
	const (
		containerID = "feedbacc"
		testAddr1   = "10.0.1.1"
		testAddr2   = "10.0.1.3"
	)

	space1 := Space{Start: net.ParseIP(testAddr1), Size: 10}
	addr1 := space1.Allocate()
	addr2 := space1.Allocate()
	addr3 := space1.Allocate()
	space1.Free(addr2)
	space1.assertInvariants()
	split1, split2 := space1.Split(net.ParseIP(testAddr2))
	wt.AssertEqualUint32(t, split1.Size, 2, "split size")
	wt.AssertEqualUint32(t, split2.Size, 8, "split size")
	wt.AssertEqualUint32(t, split1.NumFreeAddresses(), 1, "Free addresses")
	wt.AssertEqualUint32(t, split2.NumFreeAddresses(), 7, "Free addresses")
	space1.assertInvariants()
	split1.assertInvariants()
	split2.assertInvariants()
	wt.AssertNoErr(t, split1.Free(addr1))
	wt.AssertErrorInterface(t, split1.Free(addr3), (*error)(nil), "free")
	wt.AssertErrorInterface(t, split2.Free(addr1), (*error)(nil), "free")
	wt.AssertNoErr(t, split2.Free(addr3))
}
