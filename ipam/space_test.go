package ipam

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

	space1 := NewSpace(net.ParseIP(testAddr1), 20)
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 20, "Free addresses")
	space1.checkInvariant(t)

	addr1 := space1.Allocate()
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 19, "Free addresses")
	space1.checkInvariant(t)

	addr2 := space1.Allocate()
	wt.AssertNotEqualString(t, addr2.String(), testAddr1, "address")
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 18, "Free addresses")
	space1.checkInvariant(t)

	space1.Free(addr2)

	wt.AssertErrorInterface(t, space1.Free(addr2), (*error)(nil), "double free")
	wt.AssertErrorInterface(t, space1.Free(net.ParseIP(testAddrx)), (*error)(nil), "address not allocated")
	wt.AssertErrorInterface(t, space1.Free(net.ParseIP(testAddry)), (*error)(nil), "wrong out of range")

	space1.checkInvariant(t)
}

func NewMinSpace(start net.IP, size uint32) *MinSpace {
	return &MinSpace{Start: start, Size: size}
}

func (m *MutableSpace) checkInvariant(t *testing.T) {
	if m.MaxAllocated > m.Size {
		t.Fatalf("MaxAllocated must not be greater than size: %v", m)
	}
	for i := 0; i < len(m.free_list)-1; i++ {
		if subtract(m.free_list[i], m.free_list[i+1]) > 0 {
			t.Fatalf("Free list out of order: %v", m.free_list)
		}
	}
}

func TestSpaceClaim(t *testing.T) {
	const (
		containerID = "deadbeef"
		testAddr0   = "10.0.1.0"
		testAddr1   = "10.0.1.1"
		testAddr2   = "10.0.1.10"
		testAddr3   = "10.0.4.4"
	)

	space1 := NewSpace(net.ParseIP(testAddr0), 20)
	space1.checkInvariant(t)
	space1.Claim(net.ParseIP(testAddr1))
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 19, "free addresses")
	space1.checkInvariant(t)

	space1.Claim(net.ParseIP(testAddr2))
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 18, "free addresses")
	space1.checkInvariant(t)

	if done, _ := space1.Claim(net.ParseIP(testAddr3)); done {
		t.Fatalf("Space.Claim incorrect success")
	}

	space1.Free(net.ParseIP(testAddr1))
	space1.checkInvariant(t)
	wt.AssertEqualUint32(t, space1.NumFreeAddresses(), 19, "free addresses")
}

func TestSpaceSplit(t *testing.T) {
	const (
		containerID = "feedbacc"
		testAddr1   = "10.0.1.1"
		testAddr2   = "10.0.1.3"
	)

	space1 := NewSpace(net.ParseIP(testAddr1), 10)
	addr1 := space1.Allocate()
	addr2 := space1.Allocate()
	addr3 := space1.Allocate()
	space1.Free(addr2)
	space1.checkInvariant(t)
	split1, split2 := space1.Split(net.ParseIP(testAddr2))
	wt.AssertEqualUint32(t, split1.GetSize(), 2, "split size")
	wt.AssertEqualUint32(t, split2.GetSize(), 8, "split size")
	wt.AssertEqualUint32(t, split1.NumFreeAddresses(), 1, "Free addresses")
	wt.AssertEqualUint32(t, split2.NumFreeAddresses(), 7, "Free addresses")
	space1.checkInvariant(t)
	split1.checkInvariant(t)
	split2.checkInvariant(t)
	wt.AssertNoErr(t, split1.Free(addr1))
	wt.AssertErrorInterface(t, split1.Free(addr3), (*error)(nil), "free")
	wt.AssertErrorInterface(t, split2.Free(addr1), (*error)(nil), "free")
	wt.AssertNoErr(t, split2.Free(addr3))
}

func TestSpaceOverlap(t *testing.T) {
	const (
		testAddr1 = "10.0.3.4"
		testAddr2 = "10.0.3.14"
		testAddr3 = "10.0.4.4"
	)

	var (
		ipAddr1 = net.ParseIP(testAddr1)
		ipAddr2 = net.ParseIP(testAddr2)
		ipAddr3 = net.ParseIP(testAddr3)
	)

	space1 := NewMinSpace(ipAddr1, 20)

	wt.AssertEqualString(t, add(ipAddr1, 10).String(), testAddr2, "address")
	wt.AssertEqualString(t, add(ipAddr1, 256).String(), testAddr3, "address")
	wt.AssertEqualInt64(t, subtract(ipAddr1, ipAddr2), -10, "address difference")
	wt.AssertEqualInt64(t, subtract(ipAddr2, ipAddr1), 10, "address difference")
	wt.AssertEqualInt64(t, subtract(ipAddr3, ipAddr1), 256, "address difference")
	wt.AssertBool(t, space1.Contains(ipAddr1), true, "contains")
	wt.AssertBool(t, space1.Contains(ipAddr2), true, "contains")
	wt.AssertBool(t, space1.Contains(ipAddr3), false, "contains")

	space2 := NewMinSpace(ipAddr2, 10)
	space3 := NewMinSpace(ipAddr3, 10)
	space4 := NewMinSpace(ipAddr1, 10)
	space5 := NewMinSpace(ipAddr1, 11)
	space6 := NewMinSpace(ipAddr1, 9)
	space7 := NewMinSpace(ipAddr2, 9)
	space8 := NewMinSpace(ipAddr2, 11)
	if !space1.Overlaps(space2) || !space2.Overlaps(space1) {
		t.Fatalf("Space.Overlaps failed: %+v / %+v", space1, space2)
	}
	if space4.Overlaps(space2) || space2.Overlaps(space4) {
		t.Fatalf("Space.Overlaps failed: %+v / %+v", space4, space2)
	}
	if !space5.Overlaps(space2) || !space2.Overlaps(space5) {
		t.Fatalf("Space.Overlaps failed: %+v / %+v", space5, space2)
	}
	if space6.Overlaps(space2) || space2.Overlaps(space6) {
		t.Fatalf("Space.Overlaps failed: %+v / %+v", space6, space2)
	}
	if space3.Overlaps(space1) || space1.Overlaps(space3) {
		t.Fatalf("Space.Overlaps failed: %+v / %+v", space3, space1)
	}

	wt.AssertBool(t, space1.ContainsSpace(space2), true, "contains")
	wt.AssertBool(t, space1.ContainsSpace(space7), true, "contains")
	wt.AssertBool(t, space1.ContainsSpace(space8), false, "contains")
	wt.AssertBool(t, space1.ContainsSpace(space3), false, "contains")
	wt.AssertBool(t, space1.ContainsSpace(space4), true, "contains")
	wt.AssertBool(t, space1.ContainsSpace(space5), true, "contains")
	wt.AssertBool(t, space2.ContainsSpace(space6), false, "contains")
	wt.AssertBool(t, space5.ContainsSpace(space1), false, "contains")
	wt.AssertBool(t, space5.ContainsSpace(space6), true, "contains")
}

func TestSpaceHeirs(t *testing.T) {
	const (
		testAddr0 = "10.0.1.0"
		testAddr1 = "10.0.1.1"
		testAddr2 = "10.0.1.10"
		testAddr3 = "10.0.4.4"
		testAddr6 = "10.0.1.19"
	)

	var (
		ipAddr1 = net.ParseIP(testAddr1)
		ipAddr2 = net.ParseIP(testAddr2)
		ipAddr3 = net.ParseIP(testAddr3)
		ipAddr6 = net.ParseIP(testAddr6)
	)

	universe := NewMinSpace(net.ParseIP(testAddr0), 256)
	space1 := NewMinSpace(ipAddr1, 9)
	space2 := NewMinSpace(ipAddr2, 9)   // 1 is heir to 2
	space3 := NewMinSpace(ipAddr3, 8)   // 3 is nowhere near 1 or 2
	space4 := NewMinSpace(ipAddr1, 8)   // 4 is just too small to be heir to 2
	space5 := NewMinSpace(ipAddr2, 247) // 5 is heir to 1, considering wrap-around
	space6 := NewMinSpace(ipAddr6, 2)   // 2 is heir to 6

	if !space1.IsHeirTo(space2, universe) {
		t.Fatalf("Space.IsHeirTo false negative: %+v / %+v", space1, space2)
	}
	if space2.IsHeirTo(space1, universe) {
		t.Fatalf("Space.IsHeirTo false positive: %+v / %+v", space2, space1)
	}
	if space1.IsHeirTo(space3, universe) {
		t.Fatalf("Space.IsHeirTo false positive: %+v / %+v", space1, space3)
	}
	if space3.IsHeirTo(space2, universe) {
		t.Fatalf("Space.IsHeirTo false positive: %+v / %+v", space3, space2)
	}
	if space4.IsHeirTo(space2, universe) {
		t.Fatalf("Space.IsHeirTo false positive: %+v / %+v", space4, space2)
	}
	if !space5.IsHeirTo(space1, universe) {
		t.Fatalf("Space.IsHeirTo false negative: %+v / %+v", space5, space1)
	}

	spaceM := NewSpace(ipAddr2, 9)
	merged := spaceM.mergeBlank(space3)
	if merged {
		t.Fatalf("Space.merge incorrect success")
	}
	merged = spaceM.mergeBlank(space1)
	if !merged {
		t.Fatalf("Space.merge incorrect failure")
	}
	wt.AssertEqualUint32(t, spaceM.GetSize(), 18, "Merged size")
	spaceM.checkInvariant(t)
	merged = spaceM.mergeBlank(space6)
	if !merged {
		t.Fatalf("Space.merge incorrect failure")
	}
	wt.AssertEqualUint32(t, spaceM.GetSize(), 20, "Merged size")
	spaceM.checkInvariant(t)
}
