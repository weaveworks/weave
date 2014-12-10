package sortinghat

import (
	"net"
	"testing"
)

func TestSpaceAllocate(t *testing.T) {
	const (
		testAddr1   = "10.0.3.4"
		containerID = "deadbeef"
	)

	var (
		ipAddr1 = net.ParseIP(testAddr1)
	)

	space1 := NewSpace(ipAddr1, 20)

	if f := space1.LargestFreeBlock(); f != 20 {
		t.Fatalf("LargestFreeBlock: expected %d, got %d", 20, f)
	}

	addr1 := space1.AllocateFor(containerID)
	if addr1.String() != testAddr1 {
		t.Fatalf("Expected address %s but got %s", testAddr1, addr1)
	}

	if f := space1.LargestFreeBlock(); f != 19 {
		t.Fatalf("LargestFreeBlock: expected %d, got %d", 19, f)
	}
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

	space1 := NewSpace(ipAddr1, 20).GetMinSpace()

	if s := add(ipAddr1, 10); s.String() != testAddr2 {
		t.Fatalf("Expected address %s but got %s", testAddr2, s)
	}
	if s := add(ipAddr1, 256); s.String() != testAddr3 {
		t.Fatalf("Expected address %s but got %s", testAddr3, s)
	}

	if d := subtract(ipAddr1, ipAddr2); d != -10 {
		t.Fatalf("Expected difference %d but got %d", 10, d)
	}
	if d := subtract(ipAddr2, ipAddr1); d != 10 {
		t.Fatalf("Expected difference %d but got %d", 10, d)
	}
	if d := subtract(ipAddr3, ipAddr1); d != 256 {
		t.Fatalf("Expected difference %d but got %d", 256, d)
	}

	space2 := NewSpace(ipAddr2, 10).GetMinSpace()
	space3 := NewSpace(ipAddr3, 10).GetMinSpace()
	space4 := NewSpace(ipAddr1, 10).GetMinSpace()
	space5 := NewSpace(ipAddr1, 11).GetMinSpace()
	space6 := NewSpace(ipAddr1, 9).GetMinSpace()
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
}
