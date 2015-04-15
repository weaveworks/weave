package ring

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/router"
	wt "github.com/weaveworks/weave/testing"
)

var (
	peer1name, _ = router.PeerNameFromString("01:00:00:00:02:00")
	peer2name, _ = router.PeerNameFromString("02:00:00:00:02:00")
	peer3name, _ = router.PeerNameFromString("03:00:00:00:02:00")

	start, end    = ParseIP("10.0.0.0"), ParseIP("10.0.0.255")
	dot10, dot245 = ParseIP("10.0.0.10"), ParseIP("10.0.0.245")
	dot250        = ParseIP("10.0.0.250")
	middle        = ParseIP("10.0.0.128")
)

func ParseIP(s string) address.Address {
	addr, _ := address.ParseIP(s)
	return addr
}

func TestInvariants(t *testing.T) {
	ring := New(start, end, peer1name)

	// Check ring is sorted
	ring.Entries = []*entry{{Token: dot245, Peer: peer1name}, {Token: dot10, Peer: peer2name}}
	wt.AssertTrue(t, ring.checkInvariants() == ErrNotSorted, "Expected error")

	// Check tokens don't appear twice
	ring.Entries = []*entry{{Token: dot245, Peer: peer1name}, {Token: dot245, Peer: peer2name}}
	wt.AssertTrue(t, ring.checkInvariants() == ErrTokenRepeated, "Expected error")

	// Check tokens are in bounds
	ring = New(dot10, dot245, peer1name)
	ring.Entries = []*entry{{Token: start, Peer: peer1name}}
	wt.AssertTrue(t, ring.checkInvariants() == ErrTokenOutOfRange, "Expected error")

	ring.Entries = []*entry{{Token: end, Peer: peer1name}}
	wt.AssertTrue(t, ring.checkInvariants() == ErrTokenOutOfRange, "Expected error")
}

func TestInsert(t *testing.T) {
	ring := New(start, end, peer1name)
	ring.Entries = []*entry{{Token: start, Peer: peer1name, Free: 255}}

	wt.AssertPanic(t, func() {
		ring.Entries.insert(entry{Token: start, Peer: peer1name})
	})

	ring.Entries.entry(0).Free = 0
	ring.Entries.insert(entry{Token: dot245, Peer: peer1name})
	ring2 := New(start, end, peer1name)
	ring2.Entries = []*entry{{Token: start, Peer: peer1name, Free: 0}, {Token: dot245, Peer: peer1name}}
	wt.AssertEquals(t, ring, ring2)

	ring.Entries.insert(entry{Token: dot10, Peer: peer1name})
	ring2.Entries = []*entry{{Token: start, Peer: peer1name, Free: 0}, {Token: dot10, Peer: peer1name}, {Token: dot245, Peer: peer1name}}
	wt.AssertEquals(t, ring, ring2)
}

func TestBetween(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring1.Entries = []*entry{{Token: start, Peer: peer1name, Free: 255}}

	// First off, in a ring where everything is owned by the peer
	// between should return true for everything
	for i := 1; i <= 255; i++ {
		ip := ParseIP(fmt.Sprintf("10.0.0.%d", i))
		wt.AssertTrue(t, ring1.Entries.between(ip, 0, 1), "between should be true!")
	}

	// Now, construct a ring with entries at +10 and -10
	// And check the correct behaviour

	ring1.Entries = []*entry{{Token: dot10, Peer: peer1name}, {Token: dot245, Peer: peer2name}}
	ring1.assertInvariants()
	for i := 10; i <= 244; i++ {
		ipStr := fmt.Sprintf("10.0.0.%d", i)
		ip := ParseIP(ipStr)
		wt.AssertTrue(t, ring1.Entries.between(ip, 0, 1),
			fmt.Sprintf("Between should be true for %s!", ipStr))
		wt.AssertFalse(t, ring1.Entries.between(ip, 1, 2),
			fmt.Sprintf("Between should be false for %s!", ipStr))
	}
	for i := 0; i <= 9; i++ {
		ipStr := fmt.Sprintf("10.0.0.%d", i)
		ip := ParseIP(ipStr)
		wt.AssertFalse(t, ring1.Entries.between(ip, 0, 1),
			fmt.Sprintf("Between should be false for %s!", ipStr))
		wt.AssertTrue(t, ring1.Entries.between(ip, 1, 2),
			fmt.Sprintf("Between should be true for %s!", ipStr))
	}
	for i := 245; i <= 255; i++ {
		ipStr := fmt.Sprintf("10.0.0.%d", i)
		ip := ParseIP(ipStr)
		wt.AssertFalse(t, ring1.Entries.between(ip, 0, 1),
			fmt.Sprintf("Between should be false for %s!", ipStr))
		wt.AssertTrue(t, ring1.Entries.between(ip, 1, 2),
			fmt.Sprintf("Between should be true for %s!", ipStr))
	}
}

func TestGrantSimple(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	// Claim everything for peer1
	ring1.ClaimItAll()
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer1name, Free: 255}})

	// Now grant everything to peer2
	ring1.GrantRangeToHost(start, end, peer2name)
	ring2.Entries = []*entry{{Token: start, Peer: peer2name, Free: 255, Version: 1}}
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// Now spint back to peer 1
	ring2.GrantRangeToHost(dot10, end, peer1name)
	ring1.Entries = []*entry{{Token: start, Peer: peer2name, Free: 10, Version: 2},
		{Token: dot10, Peer: peer1name, Free: 245}}
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// And spint back to peer 2 again
	ring1.GrantRangeToHost(dot245, end, peer2name)
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer2name, Free: 10, Version: 2},
		{Token: dot10, Peer: peer1name, Free: 235, Version: 1},
		{Token: dot245, Peer: peer2name, Free: 10}})

	// Grant range spanning a live token
	ring1.Entries = []*entry{{Token: start, Peer: peer1name, Free: 10, Version: 2},
		{Token: dot10, Peer: peer1name, Free: 235}, {Token: dot245, Peer: peer1name, Free: 10}}
	ring1.GrantRangeToHost(dot10, end, peer2name)
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer1name, Free: 10, Version: 2},
		{Token: dot10, Peer: peer2name, Free: 235, Version: 1},
		{Token: dot245, Peer: peer2name, Free: 10, Version: 1}})
}

func TestGrantSplit(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	// Claim everything for peer1
	ring1.Entries = []*entry{{Token: start, Peer: peer1name, Free: 255}}
	ring2.Merge(*ring1)
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// Now grant a split range to peer2
	ring1.GrantRangeToHost(dot10, dot245, peer2name)
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer1name, Free: 10, Version: 1},
		{Token: dot10, Peer: peer2name, Free: 235},
		{Token: dot245, Peer: peer1name, Free: 10}})
}

func TestMergeSimple(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	// Claim everything for peer1
	ring1.ClaimItAll()
	ring1.GrantRangeToHost(middle, end, peer2name)
	wt.AssertSuccess(t, ring2.Merge(*ring1))

	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer1name, Free: 128, Version: 1},
		{Token: middle, Peer: peer2name, Free: 127}})
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// Now to two different operations on either side,
	// check we can Merge again
	ring1.GrantRangeToHost(start, middle, peer2name)
	ring2.GrantRangeToHost(middle, end, peer1name)

	wt.AssertSuccess(t, ring2.Merge(*ring1))
	wt.AssertSuccess(t, ring1.Merge(*ring2))

	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127, Version: 1}})
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)
}

func TestMergeErrors(t *testing.T) {
	// Cannot Merge in an invalid ring
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)
	ring2.Entries = []*entry{{Token: middle, Peer: peer2name}, {Token: start, Peer: peer2name}}
	wt.AssertTrue(t, ring1.Merge(*ring2) == ErrNotSorted, "Expected ErrNotSorted")

	// Should Merge two rings for different ranges
	ring2 = New(start, middle, peer2name)
	ring2.Entries = []*entry{}
	wt.AssertTrue(t, ring1.Merge(*ring2) == ErrDifferentSubnets, "Expected ErrDifferentSubnets")

	// Cannot Merge newer version of entry I own
	ring2 = New(start, end, peer2name)
	ring1.Entries = []*entry{{Token: start, Peer: peer1name}}
	ring2.Entries = []*entry{{Token: start, Peer: peer1name, Version: 1}}
	wt.AssertTrue(t, ring1.Merge(*ring2) == ErrNewerVersion, "Expected ErrNewerVersion")

	// Cannot Merge two entries with same version but different hosts
	ring1.Entries = []*entry{{Token: start, Peer: peer1name}}
	ring2.Entries = []*entry{{Token: start, Peer: peer2name}}
	wt.AssertTrue(t, ring1.Merge(*ring2) == ErrInvalidEntry, "Expected ErrInvalidEntry")

	// Cannot Merge an entry into a range I own
	ring1.Entries = []*entry{{Token: start, Peer: peer1name}}
	ring2.Entries = []*entry{{Token: middle, Peer: peer2name}}
	wt.AssertTrue(t, ring1.Merge(*ring2) == ErrEntryInMyRange, "Expected ErrEntryInMyRange")
}

func TestMergeMore(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	assertRing := func(ring *Ring, entries entries) {
		wt.AssertEquals(t, ring.Entries, entries)
	}

	assertRing(ring1, []*entry{})
	assertRing(ring2, []*entry{})

	// Claim everything for peer1
	ring1.ClaimItAll()
	assertRing(ring1, []*entry{{Token: start, Peer: peer1name, Free: 255}})
	assertRing(ring2, []*entry{})

	// Check the Merge sends it to the other ring
	wt.AssertSuccess(t, ring2.Merge(*ring1))
	assertRing(ring1, []*entry{{Token: start, Peer: peer1name, Free: 255}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer1name, Free: 255}})

	// Give everything to peer2
	ring1.GrantRangeToHost(start, end, peer2name)
	assertRing(ring1, []*entry{{Token: start, Peer: peer2name, Free: 255, Version: 1}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer1name, Free: 255}})

	wt.AssertSuccess(t, ring2.Merge(*ring1))
	assertRing(ring1, []*entry{{Token: start, Peer: peer2name, Free: 255, Version: 1}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer2name, Free: 255, Version: 1}})

	// And carve off some space
	ring2.GrantRangeToHost(middle, end, peer1name)
	assertRing(ring2, []*entry{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127}})
	assertRing(ring1, []*entry{{Token: start, Peer: peer2name, Free: 255, Version: 1}})

	// And Merge back
	wt.AssertSuccess(t, ring1.Merge(*ring2))
	assertRing(ring1, []*entry{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127}})

	// This should be a no-op
	wt.AssertSuccess(t, ring2.Merge(*ring1))
	assertRing(ring1, []*entry{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer2name, Free: 128, Version: 2},
		{Token: middle, Peer: peer1name, Free: 127}})
}

func TestMergeSplit(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	// Claim everything for peer2
	ring1.Entries = []*entry{{Token: start, Peer: peer2name, Free: 255}}
	wt.AssertSuccess(t, ring2.Merge(*ring1))
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// Now grant a split range to peer1
	ring2.GrantRangeToHost(dot10, dot245, peer1name)
	wt.AssertEquals(t, ring2.Entries, entries{{Token: start, Peer: peer2name, Free: 10, Version: 1},
		{Token: dot10, Peer: peer1name, Free: 235},
		{Token: dot245, Peer: peer2name, Free: 10}})
	wt.AssertSuccess(t, ring1.Merge(*ring2))
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer2name, Free: 10, Version: 1},
		{Token: dot10, Peer: peer1name, Free: 235},
		{Token: dot245, Peer: peer2name, Free: 10}})
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)
}

func TestMergeSplit2(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	// Claim everything for peer2
	ring1.Entries = []*entry{{Token: start, Peer: peer2name, Free: 250}, {Token: dot250, Peer: peer2name, Free: 5}}
	wt.AssertSuccess(t, ring2.Merge(*ring1))
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)

	// Now grant a split range to peer1
	ring2.GrantRangeToHost(dot10, dot245, peer1name)
	wt.AssertEquals(t, ring2.Entries, entries{{Token: start, Peer: peer2name, Free: 10, Version: 1},
		{Token: dot10, Peer: peer1name, Free: 235},
		{Token: dot245, Peer: peer2name, Free: 5}, {Token: dot250, Peer: peer2name, Free: 5}})
	wt.AssertSuccess(t, ring1.Merge(*ring2))
	wt.AssertEquals(t, ring1.Entries, entries{{Token: start, Peer: peer2name, Free: 10, Version: 1},
		{Token: dot10, Peer: peer1name, Free: 235},
		{Token: dot245, Peer: peer2name, Free: 5}, {Token: dot250, Peer: peer2name, Free: 5}})
	wt.AssertEquals(t, ring1.Entries, ring2.Entries)
}

// A simple test, very similar to above, but using the marshalling to byte[]s
func TestGossip(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	assertRing := func(ring *Ring, entries entries) {
		wt.AssertEquals(t, ring.Entries, entries)
	}

	assertRing(ring1, []*entry{})
	assertRing(ring2, []*entry{})

	// Claim everything for peer1
	ring1.ClaimItAll()
	assertRing(ring1, []*entry{{Token: start, Peer: peer1name, Free: 255}})
	assertRing(ring2, []*entry{})

	// Check the Merge sends it to the other ring
	wt.AssertSuccess(t, ring2.Merge(*ring1))
	assertRing(ring1, []*entry{{Token: start, Peer: peer1name, Free: 255}})
	assertRing(ring2, []*entry{{Token: start, Peer: peer1name, Free: 255}})
}

func TestFindFree(t *testing.T) {
	ring1 := New(start, end, peer1name)

	_, err := ring1.ChoosePeerToAskForSpace()
	wt.AssertTrue(t, err == ErrNoFreeSpace, "Expected ErrNoFreeSpace")

	ring1.Entries = []*entry{{Token: start, Peer: peer1name}}
	_, err = ring1.ChoosePeerToAskForSpace()
	wt.AssertTrue(t, err == ErrNoFreeSpace, "Expected ErrNoFreeSpace")

	// We shouldn't return outselves
	ring1.ReportFree(map[address.Address]address.Offset{start: 10})
	_, err = ring1.ChoosePeerToAskForSpace()
	wt.AssertTrue(t, err == ErrNoFreeSpace, "Expected ErrNoFreeSpace")

	ring1.Entries = []*entry{{Token: start, Peer: peer1name, Free: 1},
		{Token: start, Peer: peer1name, Free: 1}}
	_, err = ring1.ChoosePeerToAskForSpace()
	wt.AssertTrue(t, err == ErrNoFreeSpace, "Expected ErrNoFreeSpace")

	// We should return others
	ring1.Entries = []*entry{{Token: start, Peer: peer2name, Free: 1}}
	peer, err := ring1.ChoosePeerToAskForSpace()
	wt.AssertSuccess(t, err)
	wt.AssertEquals(t, peer, peer2name)

	ring1.Entries = []*entry{{Token: start, Peer: peer2name, Free: 1},
		{Token: start, Peer: peer2name, Free: 1}}
	peer, err = ring1.ChoosePeerToAskForSpace()
	wt.AssertSuccess(t, err)
	wt.AssertEquals(t, peer, peer2name)
}

func TestReportFree(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	ring1.ClaimItAll()
	ring1.GrantRangeToHost(middle, end, peer2name)
	wt.AssertSuccess(t, ring2.Merge(*ring1))

	freespace := make(map[address.Address]address.Offset)
	for _, r := range ring2.OwnedRanges() {
		freespace[r.Start] = 0
	}
	ring2.ReportFree(freespace)
}

func TestMisc(t *testing.T) {
	ring := New(start, end, peer1name)

	wt.AssertTrue(t, ring.Empty(), "empty")

	ring.ClaimItAll()
	println(ring.String())
}

func TestEmptyGossip(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	ring1.ClaimItAll()
	// This used to panic, and it shouldn't
	wt.AssertSuccess(t, ring1.Merge(*ring2))
}

func TestMergeOldMessage(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	ring1.ClaimItAll()
	wt.AssertSuccess(t, ring2.Merge(*ring1))

	ring1.GrantRangeToHost(middle, end, peer1name)
	wt.AssertSuccess(t, ring1.Merge(*ring2))
}

func TestSplitRangeAtBeginning(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring2 := New(start, end, peer2name)

	ring1.ClaimItAll()
	wt.AssertSuccess(t, ring2.Merge(*ring1))

	ring1.GrantRangeToHost(start, middle, peer2name)
	wt.AssertSuccess(t, ring2.Merge(*ring1))
}

func TestOwnedRange(t *testing.T) {
	ring1 := New(start, end, peer1name)
	ring1.ClaimItAll()

	wt.AssertEquals(t, ring1.OwnedRanges(), []address.Range{{Start: start, End: end}})

	ring1.GrantRangeToHost(middle, end, peer2name)
	wt.AssertEquals(t, ring1.OwnedRanges(), []address.Range{{Start: start, End: middle}})

	ring2 := New(start, end, peer2name)
	ring2.Merge(*ring1)
	wt.AssertEquals(t, ring2.OwnedRanges(), []address.Range{{Start: middle, End: end}})

	ring2.Entries = []*entry{{Token: middle, Peer: peer2name}}
	wt.AssertEquals(t, ring2.OwnedRanges(),
		[]address.Range{{Start: start, End: middle}, {Start: middle, End: end}})

	ring2.Entries = []*entry{{Token: dot10, Peer: peer2name}, {Token: middle, Peer: peer2name}}
	wt.AssertEquals(t, ring2.OwnedRanges(),
		[]address.Range{{Start: start, End: dot10}, {Start: dot10, End: middle},
			{Start: middle, End: end}})
}

func TestTransfer(t *testing.T) {
	// First test just checks if we can grant some range to a host, when we transfer it, we get it back
	ring1 := New(start, end, peer1name)
	ring1.ClaimItAll()
	ring1.GrantRangeToHost(middle, end, peer2name)
	ring1.Transfer(peer2name, peer1name)
	wt.AssertEquals(t, ring1.OwnedRanges(), []address.Range{{start, middle}, {middle, end}})

	// Second test is what happens when a token exists at the end of a range but is transferred
	// - does it get resurrected correctly?
	ring1 = New(start, end, peer1name)
	ring1.ClaimItAll()
	ring1.GrantRangeToHost(middle, end, peer2name)
	ring1.Transfer(peer2name, peer1name)
	ring1.GrantRangeToHost(dot10, middle, peer2name)
	wt.AssertEquals(t, ring1.OwnedRanges(), []address.Range{{start, dot10}, {middle, end}})
}

func TestOwner(t *testing.T) {
	ring1 := New(start, end, peer1name)
	wt.AssertTrue(t, ring1.Contains(start), "start should be in ring")
	wt.AssertFalse(t, ring1.Contains(end), "end should not be in ring")

	wt.AssertEquals(t, ring1.Owner(start), router.UnknownPeerName)

	ring1.ClaimItAll()
	ring1.GrantRangeToHost(middle, end, peer2name)
	wt.AssertEquals(t, ring1.Owner(start), peer1name)
	wt.AssertEquals(t, ring1.Owner(middle), peer2name)
	wt.AssertPanic(t, func() {
		ring1.Owner(end)
	})
}

type addressSlice []address.Address

func (s addressSlice) Len() int           { return len(s) }
func (s addressSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s addressSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func TestFuzzRing(t *testing.T) {
	var (
		numPeers   = 25
		iterations = 1000
	)

	peers := make([]router.PeerName, numPeers)
	for i := 0; i < numPeers; i++ {
		peer, _ := router.PeerNameFromString(fmt.Sprintf("%02d:00:00:00:02:00", i))
		peers[i] = peer
	}

	// Make a valid, random ring
	makeGoodRandomRing := func() *Ring {
		addressSpace := end - start
		numTokens := rand.Intn(int(addressSpace))

		tokenMap := make(map[address.Address]bool)
		for i := 0; i < numTokens; i++ {
			tokenMap[address.Address(rand.Intn(int(addressSpace)))] = true
		}
		var tokens []address.Address
		for token := range tokenMap {
			tokens = append(tokens, token)
		}
		sort.Sort(addressSlice(tokens))

		peer := peers[rand.Intn(len(peers))]
		ring := New(start, end, peer)
		for _, token := range tokens {
			peer = peers[rand.Intn(len(peers))]
			ring.Entries = append(ring.Entries, &entry{Token: start + token, Peer: peer})
		}

		ring.assertInvariants()
		return ring
	}

	for i := 0; i < iterations; i++ {
		// make 2 random rings
		ring1 := makeGoodRandomRing()
		ring2 := makeGoodRandomRing()

		// Merge them - this might fail, we don't care
		// We just want to make sure it doesn't panic
		ring1.Merge(*ring2)

		// Check whats left still passes assertions
		ring1.assertInvariants()
		ring2.assertInvariants()
	}

	// Make an invalid, random ring
	makeBadRandomRing := func() *Ring {
		addressSpace := end - start
		numTokens := rand.Intn(int(addressSpace))
		tokens := make([]address.Address, numTokens)
		for i := 0; i < numTokens; i++ {
			tokens[i] = address.Address(rand.Intn(int(addressSpace)))
		}

		peer := peers[rand.Intn(len(peers))]
		ring := New(start, end, peer)
		for _, token := range tokens {
			peer = peers[rand.Intn(len(peers))]
			ring.Entries = append(ring.Entries, &entry{Token: start + token, Peer: peer})
		}

		return ring
	}

	for i := 0; i < iterations; i++ {
		// make 2 random rings
		ring1 := makeGoodRandomRing()
		ring2 := makeBadRandomRing()

		// Merge them - this might fail, we don't care
		// We just want to make sure it doesn't panic
		ring1.Merge(*ring2)

		// Check whats left still passes assertions
		ring1.assertInvariants()
	}
}

func TestFuzzRingHard(t *testing.T) {
	//common.InitDefaultLogging(true)
	var (
		numPeers   = 100
		iterations = 3000
		peers      []router.PeerName
		rings      []*Ring
		nextPeerID = 0
	)

	addPeer := func() {
		peer, _ := router.PeerNameFromString(fmt.Sprintf("%02d:%02d:00:00:00:00", nextPeerID/10, nextPeerID%10))
		common.Debug.Printf("%s: Adding peer", peer)
		nextPeerID++
		peers = append(peers, peer)
		rings = append(rings, New(start, end, peer))
	}

	for i := 0; i < numPeers; i++ {
		addPeer()
	}

	rings[0].ClaimItAll()

	randomPeer := func(exclude int) (int, router.PeerName, *Ring) {
		var peerIndex int
		if exclude >= 0 {
			peerIndex = rand.Intn(len(peers) - 1)
			if peerIndex == exclude {
				peerIndex++
			}
		} else {
			peerIndex = rand.Intn(len(peers))
		}
		return peerIndex, peers[peerIndex], rings[peerIndex]
	}

	// Keep a map of index -> ranges, as these are a little expensive to
	// calculate for every ring on every iteration.
	var theRanges = make(map[int][]address.Range)
	theRanges[0] = rings[0].OwnedRanges()

	addOrRmPeer := func() {
		if len(peers) < numPeers {
			addPeer()
			return
		}

		peerIndex, peername, _ := randomPeer(-1)
		// Remove peer from our state
		peers = append(peers[:peerIndex], peers[peerIndex+1:]...)
		rings = append(rings[:peerIndex], rings[peerIndex+1:]...)
		theRanges = make(map[int][]address.Range)

		// Transfer the space for this peer on another peer, but not this one
		_, otherPeername, otherRing := randomPeer(peerIndex)

		// We need to be in a ~converged ring to rmpeer
		for _, ring := range rings {
			wt.AssertSuccess(t, otherRing.Merge(*ring))
		}

		common.Debug.Printf("%s: transferring from peer %s", otherPeername, peername)
		otherRing.Transfer(peername, peername)

		// And now tell everyone about the transfer - rmpeer is
		// not partition safe
		for i, ring := range rings {
			wt.AssertSuccess(t, ring.Merge(*otherRing))
			theRanges[i] = ring.OwnedRanges()
		}
	}

	doGrantOrGossip := func() {
		var ringsWithRanges = make([]int, 0, len(rings))
		for index, ranges := range theRanges {
			if len(ranges) > 0 {
				ringsWithRanges = append(ringsWithRanges, index)
			}
		}

		if len(ringsWithRanges) > 0 {
			// Produce a random split in a random owned range, given to a random peer
			indexWithRanges := ringsWithRanges[rand.Intn(len(ringsWithRanges))]
			ownedRanges := theRanges[indexWithRanges]
			ring := rings[indexWithRanges]

			rangeToSplit := ownedRanges[rand.Intn(len(ownedRanges))]
			size := address.Subtract(rangeToSplit.End, rangeToSplit.Start)
			ipInRange := address.Add(rangeToSplit.Start, address.Offset(rand.Intn(int(size))))
			_, peerToGiveTo, _ := randomPeer(-1)
			common.Debug.Printf("%s: Granting [%v, %v) to %s", ring.Peer, ipInRange, rangeToSplit.End, peerToGiveTo)
			ring.GrantRangeToHost(ipInRange, rangeToSplit.End, peerToGiveTo)

			// Now 'gossip' this to a random host (note, note could be same host as above)
			otherIndex, _, otherRing := randomPeer(-1)
			common.Debug.Printf("%s: 'Gossiping' to %s", ring.Peer, otherRing.Peer)
			wt.AssertSuccess(t, otherRing.Merge(*ring))

			theRanges[indexWithRanges] = ring.OwnedRanges()
			theRanges[otherIndex] = otherRing.OwnedRanges()
			return
		}

		// No rings think they own anything (as gossip might be behind)
		// We're going to pick a random host (which has entries) and gossip
		// it to a random host (which may or may not have entries).
		var ringsWithEntries = make([]*Ring, 0, len(rings))
		for _, ring := range rings {
			if len(ring.Entries) > 0 {
				ringsWithEntries = append(ringsWithEntries, ring)
			}
		}
		ring1 := ringsWithEntries[rand.Intn(len(ringsWithEntries))]
		ring2index, _, ring2 := randomPeer(-1)
		common.Debug.Printf("%s: 'Gossiping' to %s", ring1.Peer, ring2.Peer)
		wt.AssertSuccess(t, ring2.Merge(*ring1))
		theRanges[ring2index] = ring2.OwnedRanges()
	}

	for i := 0; i < iterations; i++ {
		// about 1 in 10 times, rmpeer or add host
		n := rand.Intn(10)
		switch {
		case n < 1:
			addOrRmPeer()
		default:
			doGrantOrGossip()
		}
	}
}

func (es entries) String() string {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "[")
	for i, entry := range es {
		fmt.Fprintf(&buffer, "%+v", *entry)
		if i+1 < len(es) {
			fmt.Fprintf(&buffer, " ")
		}
	}
	fmt.Fprintf(&buffer, "]")
	return buffer.String()
}
