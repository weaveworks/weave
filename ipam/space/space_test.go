package space

import (
	"reflect"
	"testing"

	"github.com/weaveworks/weave/ipam/address"
	wt "github.com/weaveworks/weave/testing"
)

func makeSpace(start address.Address, size address.Offset) *Space {
	s := New()
	s.Add(start, size)
	return s
}

func ip(s string) address.Address {
	addr, _ := address.ParseIP(s)
	return addr
}

func TestLowlevel(t *testing.T) {
	a := []address.Address{}
	a = add(a, 100, 200)
	wt.AssertEquals(t, a, []address.Address{100, 200})
	wt.AssertTrue(t, !contains(a, 99), "")
	wt.AssertTrue(t, contains(a, 100), "")
	wt.AssertTrue(t, contains(a, 199), "")
	wt.AssertTrue(t, !contains(a, 200), "")
	a = add(a, 700, 800)
	wt.AssertEquals(t, a, []address.Address{100, 200, 700, 800})
	a = add(a, 300, 400)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 400, 700, 800})
	a = add(a, 400, 500)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 500, 700, 800})
	a = add(a, 600, 700)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 500, 600, 800})
	a = add(a, 500, 600)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 800})
	a = subtract(a, 500, 600)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 500, 600, 800})
	a = subtract(a, 600, 700)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 500, 700, 800})
	a = subtract(a, 400, 500)
	wt.AssertEquals(t, a, []address.Address{100, 200, 300, 400, 700, 800})
	a = subtract(a, 300, 400)
	wt.AssertEquals(t, a, []address.Address{100, 200, 700, 800})
	a = subtract(a, 700, 800)
	wt.AssertEquals(t, a, []address.Address{100, 200})
	a = subtract(a, 100, 200)
	wt.AssertEquals(t, a, []address.Address{})

	s := New()
	wt.AssertEquals(t, s.NumFreeAddresses(), address.Offset(0))
	ok, got := s.Allocate()
	wt.AssertFalse(t, ok, "allocate in empty space should fail")

	s.Add(100, 100)
	wt.AssertEquals(t, s.NumFreeAddresses(), address.Offset(100))
	ok, got = s.Allocate()
	wt.AssertTrue(t, ok && got == 100, "allocate")
	wt.AssertEquals(t, s.NumFreeAddresses(), address.Offset(99))
	wt.AssertNoErr(t, s.Claim(150))
	wt.AssertEquals(t, s.NumFreeAddresses(), address.Offset(98))
	wt.AssertNoErr(t, s.Free(100))
	wt.AssertEquals(t, s.NumFreeAddresses(), address.Offset(99))
	wt.AssertErrorInterface(t, s.Free(0), (*error)(nil), "free not allocated")
	wt.AssertErrorInterface(t, s.Free(100), (*error)(nil), "double free")

	start, size, ok := s.Donate()
	wt.AssertTrue(t, ok && start == 125 && size == 25, "donate")

	// test Donate when addresses are scarce
	s = New()
	start, size, ok = s.Donate()
	wt.AssertTrue(t, !ok, "donate on empty space should fail")
	s.Add(0, 3)
	wt.AssertNoErr(t, s.Claim(0))
	wt.AssertNoErr(t, s.Claim(2))
	start, size, ok = s.Donate()
	wt.AssertTrue(t, ok && start == 1 && size == 1, "donate")
	start, size, ok = s.Donate()
	wt.AssertTrue(t, !ok, "donate should fail")
}

func TestSpaceAllocate(t *testing.T) {
	const (
		testAddr1   = "10.0.3.4"
		testAddrx   = "10.0.3.19"
		testAddry   = "10.0.9.19"
		containerID = "deadbeef"
	)

	space1 := makeSpace(ip(testAddr1), 20)
	wt.AssertEquals(t, space1.NumFreeAddresses(), address.Offset(20))
	space1.assertInvariants()

	_, addr1 := space1.Allocate()
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")
	wt.AssertEquals(t, space1.NumFreeAddresses(), address.Offset(19))
	space1.assertInvariants()

	_, addr2 := space1.Allocate()
	wt.AssertNotEqualString(t, addr2.String(), testAddr1, "address")
	wt.AssertEquals(t, space1.NumFreeAddresses(), address.Offset(18))
	wt.AssertEquals(t, space1.NumFreeAddressesInRange(ip(testAddr1), ip(testAddrx)), address.Offset(13))
	wt.AssertEquals(t, space1.NumFreeAddressesInRange(ip(testAddr1), ip(testAddry)), address.Offset(18))
	space1.assertInvariants()

	space1.Free(addr2)
	space1.assertInvariants()

	wt.AssertErrorInterface(t, space1.Free(addr2), (*error)(nil), "double free")
	wt.AssertErrorInterface(t, space1.Free(ip(testAddrx)), (*error)(nil), "address not allocated")
	wt.AssertErrorInterface(t, space1.Free(ip(testAddry)), (*error)(nil), "wrong out of range")

	space1.assertInvariants()
}

func TestSpaceFree(t *testing.T) {
	const (
		testAddr1   = "10.0.3.4"
		testAddrx   = "10.0.3.19"
		testAddry   = "10.0.9.19"
		containerID = "deadbeef"
	)

	space := makeSpace(ip(testAddr1), 20)

	// Check we are prepared to give up the entire space
	pos, size := space.biggestFreeRange()
	wt.AssertTrue(t, space.free[pos] == ip(testAddr1) && size == 20, "Wrong space")

	for i := 0; i < 20; i++ {
		ok, _ := space.Allocate()
		wt.AssertTrue(t, ok, "Failed to get address")
	}

	// Check we are full
	ok, _ := space.Allocate()
	wt.AssertTrue(t, !ok, "Should have failed to get address")
	_, size, ok = space.Donate()
	wt.AssertTrue(t, size == 0, "Wrong space")

	// Free in the middle
	wt.AssertSuccess(t, space.Free(ip("10.0.3.13")))
	pos, size = space.biggestFreeRange()
	wt.AssertTrue(t, space.free[pos] == ip("10.0.3.13") && size == 1, "Wrong space")

	// Free one at the end
	wt.AssertSuccess(t, space.Free(ip("10.0.3.23")))
	pos, size = space.biggestFreeRange()
	wt.AssertTrue(t, space.free[pos] == ip("10.0.3.23") && size == 1, "Wrong space")

	// Now free a few at the end
	wt.AssertSuccess(t, space.Free(ip("10.0.3.22")))
	wt.AssertSuccess(t, space.Free(ip("10.0.3.21")))

	wt.AssertEquals(t, space.NumFreeAddresses(), address.Offset(4))

	// Now get the biggest free space; should be 3.21
	pos, size = space.biggestFreeRange()
	wt.AssertTrue(t, space.free[pos] == ip("10.0.3.21") && size == 3, "Wrong space")

	// Now free a few in the middle
	wt.AssertSuccess(t, space.Free(ip("10.0.3.12")))
	wt.AssertSuccess(t, space.Free(ip("10.0.3.11")))
	wt.AssertSuccess(t, space.Free(ip("10.0.3.10")))

	wt.AssertEquals(t, space.NumFreeAddresses(), address.Offset(7))

	// Now get the biggest free space; should be 3.21
	pos, size = space.biggestFreeRange()
	wt.AssertTrue(t, space.free[pos] == ip("10.0.3.10") && size == 4, "Wrong space")

	wt.AssertEquals(t, space.OwnedRanges(), []address.Range{address.Range{Start: ip("10.0.3.4"), End: ip("10.0.3.24")}})
}

func (s1 *Space) Equal(s2 *Space) bool {
	return reflect.DeepEqual(s1.ours, s2.ours) && reflect.DeepEqual(s1.free, s2.free)
}

func TestDonateSimple(t *testing.T) {
	const (
		testAddr1 = "10.0.1.0"
		testAddr2 = "10.0.1.32"
	)

	var (
		ipAddr1 = ip(testAddr1)
	)

	ps1 := makeSpace(ipAddr1, 48)

	// Empty space set should split in two and give me the second half
	start, numGivenUp, ok := ps1.Donate()
	wt.AssertBool(t, ok, true, "Donate result")
	wt.AssertEqualString(t, start.String(), "10.0.1.24", "Invalid start")
	wt.AssertEquals(t, numGivenUp, address.Offset(24))
	wt.AssertEquals(t, ps1.NumFreeAddresses(), address.Offset(24))

	// Now check we can give the rest up.
	count := 0 // count to avoid infinite loop
	for ; count < 1000; count++ {
		_, size, ok := ps1.Donate()
		if !ok {
			break
		}
		numGivenUp += size
	}
	wt.AssertEquals(t, ps1.NumFreeAddresses(), address.Offset(0))
	wt.AssertEquals(t, numGivenUp, address.Offset(48))
}

func TestDonateHard(t *testing.T) {
	//common.InitDefaultLogging(true)
	var (
		start                = ip("10.0.1.0")
		size  address.Offset = 48
	)

	// Fill a fresh space
	spaceset := makeSpace(start, size)
	for i := address.Offset(0); i < size; i++ {
		ok, _ := spaceset.Allocate()
		wt.AssertTrue(t, ok, "Failed to get IP!")
	}

	wt.AssertEquals(t, spaceset.NumFreeAddresses(), address.Offset(0))

	// Now free all but the last address
	// this will force us to split the free list
	for i := address.Offset(0); i < size-1; i++ {
		wt.AssertSuccess(t, spaceset.Free(address.Add(start, i)))
	}

	// Now split
	newRange, numGivenUp, ok := spaceset.Donate()
	wt.AssertBool(t, ok, true, "GiveUpSpace result")
	wt.AssertEquals(t, newRange, ip("10.0.1.23"))
	wt.AssertEquals(t, numGivenUp, address.Offset(24))
	wt.AssertEquals(t, spaceset.NumFreeAddresses(), address.Offset(23))

	//Space set should now have 2 spaces
	expected := New()
	expected.Add(start, 23)
	expected.ours = add(nil, ip("10.0.1.47"), ip("10.0.1.48"))
	wt.AssertEquals(t, spaceset, expected)
}
