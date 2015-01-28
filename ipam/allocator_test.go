package ipam

import (
	"fmt"
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	ourUID     = 123456
	peerUID    = 654321
	testStart1 = "10.0.1.0"
	testStart2 = "10.0.2.0"
	testStart3 = "10.0.3.0"
)

func testAllocator(name string, ourUID uint64, universeCIDR string) *Allocator {
	ourName, _ := router.PeerNameFromString(name)
	alloc, _ := NewAllocator(ourName, ourUID, universeCIDR)
	alloc.gossip = new(mockGossipComms)
	return alloc
}

func (alloc *Allocator) addSpace(startAddr string, length uint32) *Allocator {
	alloc.manageSpace(net.ParseIP(startAddr), length)
	return alloc
}

func TestAllocFree(t *testing.T) {
	const (
		container2 = "baddf00d"
		testAddr1  = "10.0.3.4"
	)

	alloc := testAllocator("01:00:00:01:00:00", ourUID, testAddr1+"/30").addSpace(testAddr1, 4)

	addr1 := alloc.AllocateFor("abcdef")
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")

	// Ask for another address and check it's different
	addr2 := alloc.AllocateFor(container2)
	if addr2.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr2)
	}

	// Now free the first one, and we should get it back when we ask
	alloc.Free(net.ParseIP(testAddr1))
	addr3 := alloc.AllocateFor(container2)
	wt.AssertEqualString(t, addr3.String(), testAddr1, "address")
}

func equalByteBuffer(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMultiSpaces(t *testing.T) {
	alloc := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/30")
	alloc.addSpace(testStart1, 1)
	alloc.addSpace(testStart2, 3)

	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 4, "Total free addresses")

	addr1 := alloc.AllocateFor("abcdef")
	wt.AssertEqualString(t, addr1.String(), testStart1, "address")

	// First space should now be full and this address should come from second space
	addr2 := alloc.AllocateFor("fedcba")
	wt.AssertEqualString(t, addr2.String(), testStart2, "address")
	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 2, "Total free addresses")
}

func TestEncodeMerge(t *testing.T) {
	alloc := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/22")
	alloc.addSpace(testStart1, 16)
	alloc.addSpace(testStart2, 32)

	encodedState := alloc.Gossip()

	alloc2 := testAllocator("02:00:00:02:00:00", peerUID, testStart1+"/22").addSpace(testStart3, 32)
	encodedState2 := alloc2.Gossip()

	newBuf, err := alloc2.OnGossip(encodedState)
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(alloc2.peerInfo), 2, "spaceset count")
	decodedSpaceSet, found := alloc2.peerInfo[ourUID]
	if !found {
		t.Fatal("Decoded allocator did not contain spaceSet")
	}
	if decodedSpaceSet.PeerName() != alloc.ourName || decodedSpaceSet.UID() != ourUID || !alloc.ourSpaceSet.Equal(decodedSpaceSet.(*PeerSpaceSet)) {
		t.Fatalf("Allocator not decoded as expected: %+v vs %+v", alloc.ourSpaceSet, decodedSpaceSet)
	}

	// Returned buffer should be all the new ones - i.e. everything we passed in
	if !equalByteBuffer(encodedState, newBuf) {
		t.Fatal("Generated buffer does not match")
	}

	// Do it again, and nothing should be new
	newBuf, err = alloc2.OnGossip(encodedState)
	wt.AssertNoErr(t, err)
	if newBuf != nil {
		t.Fatal("Unexpected new items found")
	}

	// Now encode and merge the other way
	buf := alloc2.Gossip()

	newBuf, err = alloc.OnGossip(buf)
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(alloc.peerInfo), 2, "spaceset count")

	decodedSpaceSet, found = alloc.peerInfo[peerUID]
	if !found {
		t.Fatal("Decoded allocator did not contain spaceSet")
	}
	if decodedSpaceSet.PeerName() != alloc2.ourName || decodedSpaceSet.UID() != peerUID || !alloc2.ourSpaceSet.Equal(decodedSpaceSet.(*PeerSpaceSet)) {
		t.Fatalf("Allocator not decoded as expected: %+v vs %+v", alloc2.ourSpaceSet, decodedSpaceSet)
	}

	// Returned buffer should be all the new ones - i.e. just alloc2's state
	if !equalByteBuffer(encodedState2, newBuf) {
		t.Fatal("Generated buffer does not match")
	}

	// Do it again, and nothing should be new
	newBuf, err = alloc.OnGossip(buf)
	wt.AssertNoErr(t, err)
	if newBuf != nil {
		t.Fatal("Unexpected new items found")
	}
}

type mockMessage struct {
	dst router.PeerName
	buf []byte
}

func (m *mockMessage) String() string {
	return fmt.Sprintf("-> %s [%x]", m.dst, m.buf)
}

func toStringArray(messages []mockMessage) []string {
	out := make([]string, len(messages))
	for i := range out {
		out[i] = messages[i].String()
	}
	return out
}

type mockGossipComms struct {
	messages []mockMessage
}

func (f *mockGossipComms) GossipBroadcast(buf []byte) error {
	f.messages = append(f.messages, mockMessage{router.UnknownPeerName, buf})
	return nil
}

func (f *mockGossipComms) GossipUnicast(dstPeerName router.PeerName, buf []byte) error {
	f.messages = append(f.messages, mockMessage{dstPeerName, buf})
	return nil
}

// Note: this style of verification, using equalByteBuffer, requires
// that the contents of messages are never re-ordered.  Which, for instance,
// requires they are not based off iterating through a map.

func (m *mockGossipComms) VerifyMessage(t *testing.T, dst string, msgType byte, buf []byte) {
	if len(m.messages) == 0 {
		wt.Fatalf(t, "Expected Gossip message but none sent")
	} else if msg := m.messages[0]; msg.dst.String() != dst {
		wt.Fatalf(t, "Expected Gossip message to %s but got dest %s", dst, msg.dst)
	} else if msg.buf[0] != msgType {
		wt.Fatalf(t, "Expected Gossip message of type %d but got type %d", msgType, msg.buf[0])
	} else if !equalByteBuffer(msg.buf[1:], buf) {
		wt.Fatalf(t, "Gossip message not sent as expected: %+v", msg)
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
}

func (m *mockGossipComms) VerifyBroadcastMessage(t *testing.T, buf []byte) {
	if len(m.messages) == 0 {
		wt.Fatalf(t, "Expected Gossip message but none sent")
	} else if msg := m.messages[0]; msg.dst != router.UnknownPeerName {
		wt.Fatalf(t, "Expected Gossip broadcast message but got dest %s", msg.dst)
	} else if !equalByteBuffer(msg.buf, buf) {
		wt.Fatalf(t, "Gossip message not sent as expected: \nwant: %x\ngot : %x", buf, msg.buf)
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
}

func (m *mockGossipComms) VerifyNoMoreMessages(t *testing.T) {
	if len(m.messages) > 0 {
		wt.Fatalf(t, "Gossip message(s) unexpected: \n%s", strings.Join(toStringArray(m.messages), "\n"))
	}
}

func noMoreGossip(t *testing.T, gossips ...router.Gossip) {
	for _, g := range gossips {
		g.(*mockGossipComms).VerifyNoMoreMessages(t)
	}
}

type mockTimeProvider struct {
	myTime time.Time
}

type mockTimer struct {
	when time.Time
	f    func()
}

func (m *mockTimeProvider) SetTime(t time.Time) { m.myTime = t }
func (m *mockTimeProvider) Now() time.Time      { return m.myTime }

type SpaceByStart []Space

func (a SpaceByStart) Len() int           { return len(a) }
func (a SpaceByStart) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SpaceByStart) Less(i, j int) bool { return ip4int(a[i].GetStart()) < ip4int(a[j].GetStart()) }

func (alloc *Allocator) lookForOverlaps(t *testing.T) (ret bool) {
	ret = false
	allSpaces := make([]Space, 0)
	for _, peerSpaceSet := range alloc.peerInfo {
		peerSpaceSet.ForEachSpace(func(space Space) {
			allSpaces = append(allSpaces, space)
		})
	}
	sort.Sort(SpaceByStart(allSpaces))
	for i := 0; i < len(allSpaces)-1; i++ {
		if allSpaces[i].Overlaps(allSpaces[i+1]) {
			t.Logf("Spaces overlap: %s and %s", allSpaces[i], allSpaces[i+1])
			ret = true
		}
	}
	return
}

func assertNoOverlaps(t *testing.T, allocs ...*Allocator) {
	for _, alloc := range allocs {
		if alloc.lookForOverlaps(t) {
			wt.Fatalf(t, "Allocator has overlapping space: %s", alloc)
		}
	}
}

func (alloc *Allocator) considerWhileLocked() {
	alloc.Lock()
	alloc.considerOurPosition()
	alloc.Unlock()
}

func TestGossip(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() {
		implTestGossip(t)
	})
}

func implTestGossip(t *testing.T) {
	const (
		donateSize     = 5
		donateStart    = "10.0.1.7"
		peerNameString = "02:00:00:02:00:00"
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	alloc1 := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/22")
	alloc1.startForTesting()
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network
	alloc2 := testAllocator(peerNameString, peerUID, testStart1+"/22")
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(2 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Gossip())
	// At first, this peer has no space, so alloc1 should do nothing
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	noMoreGossip(t, alloc1.gossip)

	// Give alloc1 some space so we can test the choosing algorithm
	alloc1.manageSpace(net.ParseIP(testStart1), 1)
	alloc1.state = allocStateNeutral
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Now give alloc2 some space and tell alloc1 about it
	// Now let alloc2 tell alloc1 about its space
	alloc2.manageSpace(net.ParseIP(testStart2), 10)
	mockTime.SetTime(baseTime.Add(3 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Gossip())

	// Alloc1 should ask alloc2 for space
	mockGossip1 := alloc1.gossip.(*mockGossipComms)
	mockGossip1.VerifyMessage(t, peerNameString, msgSpaceRequest, encode(alloc1.ourSpaceSet))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Time out with no reply
	mockTime.SetTime(baseTime.Add(5 * time.Second))
	alloc1.considerWhileLocked()

	mockGossip1.VerifyMessage(t, peerNameString, msgSpaceRequest, encode(alloc1.ourSpaceSet))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].GetMinSpace().Size = donateSize
	alloc2.ourSpaceSet.version++

	donation := NewMinSpace(net.ParseIP(donateStart), donateSize)
	msg := router.Concat([]byte{msgSpaceDonate}, GobEncode(donation, 1, alloc2.ourSpaceSet))
	alloc1.OnGossipUnicast(alloc2.ourName, msg)
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 6, "Total free addresses")
	wt.AssertEqualuint64(t, alloc1.peerInfo[peerUID].Version(), 2, "Peer version")

	mockGossip1.VerifyBroadcastMessage(t, encode(alloc1.ourSpaceSet))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Now looking to trigger a timeout
	alloc1.OnDead(alloc2.ourName, alloc2.ourUID) // Simulate call from router
	mockTime.SetTime(baseTime.Add(11 * time.Minute))
	alloc1.considerWhileLocked()

	// Now make it look like alloc2 is a tombstone so we can check the message
	alloc2.ourSpaceSet.MakeTombstone()
	mockGossip1.VerifyBroadcastMessage(t, encode(alloc2.ourSpaceSet))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Now move the time forward so alloc1 reclaims alloc2's storage
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	alloc1.considerWhileLocked()
	mockGossip1.VerifyBroadcastMessage(t, encode(alloc1.ourSpaceSet))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)
}

func TestLeaks(t *testing.T) {
	const (
		universeSize = 16
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	// Give alloc1 the space from .0 to .7; nobody owns from .7 to .15
	alloc1 := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/27").addSpace(testStart1, 8)
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network, that is managing some of that space
	alloc2 := testAllocator("02:00:00:02:00:00", peerUID, testStart1+"/27")
	alloc2.addSpace("10.0.1.8", 4)
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(20 * time.Second))

	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip)

	// Allow alloc1 to note the leak
	mockTime.SetTime(baseTime.Add(30 * time.Second))
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Alloc2 tells alloc1 about itself
	mockTime.SetTime(baseTime.Add(40 * time.Second))
	alloc2.considerWhileLocked()
	noMoreGossip(t, alloc2.gossip)

	assertNoOverlaps(t, alloc1, alloc2)

	mockTime.SetTime(baseTime.Add(50 * time.Second))
	alloc1.OnGossipBroadcast(alloc2.Gossip())
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	assertNoOverlaps(t, alloc1, alloc2)

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	mockTime.SetTime(baseTime.Add(60 * time.Second))
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	// Now move the time forward so alloc1 reclaims the leak
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	assertNoOverlaps(t, alloc1, alloc2)

	// Fixme: why does nobody gossip anything here?
}

func TestAllocatorClaim1(t *testing.T) {
	const (
		containerID = "deadbeef"
		container2  = "baddf00d"
		testAddr1   = "10.0.1.5"
		testAddr2   = "10.0.1.6"
		oldUID      = 6464646
	)

	alloc1 := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/22")
	alloc1.startForTesting()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	alloc1.considerWhileLocked()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	noMoreGossip(t, alloc1.gossip)

	alloc2 := testAllocator("02:00:00:02:00:00", peerUID, testStart1+"/22").addSpace(testStart3, 32)

	// alloc2 has an echo of the former state of alloc1
	alloc2.peerInfo[oldUID] = spaceSetWith(alloc1.ourName, oldUID, NewSpace(net.ParseIP(testStart1), 64))

	alloc1.decodeUpdate(alloc2.Gossip())
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	err := alloc1.Claim(containerID, net.ParseIP(testAddr1))
	wt.AssertNoErr(t, err)
	alloc1.considerWhileLocked()
	mockGossip1 := alloc1.gossip.(*mockGossipComms)
	mockGossip1.VerifyBroadcastMessage(t, encode(tombstoneWith(alloc1.ourName, oldUID)))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)
}

// Same as TestAllocatorClaim1 but the claim and the gossip happen the other way round
func TestAllocatorClaim2(t *testing.T) {
	const (
		containerID = "deadbeef"
		container2  = "baddf00d"
		testAddr1   = "10.0.1.5"
		testAddr2   = "10.0.1.6"
		oldUID      = 6464646
	)

	alloc1 := testAllocator("01:00:00:01:00:00", ourUID, testStart1+"/22")
	alloc1.startForTesting()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	alloc1.considerWhileLocked()

	alloc2 := testAllocator("02:00:00:02:00:00", peerUID, testStart1+"/22").addSpace(testStart3, 32)

	// alloc2 has an echo of the former state of alloc1
	alloc2.peerInfo[oldUID] = spaceSetWith(alloc1.ourName, oldUID, NewSpace(net.ParseIP(testStart1), 64))

	err := alloc1.Claim(containerID, net.ParseIP(testAddr1))
	wt.AssertNoErr(t, err)
	alloc1.considerWhileLocked()
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)

	alloc1.decodeUpdate(alloc2.Gossip())
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)
	alloc1.considerWhileLocked()
	mockGossip1 := alloc1.gossip.(*mockGossipComms)
	mockGossip1.VerifyBroadcastMessage(t, encode(tombstoneWith(alloc1.ourName, oldUID)))
	noMoreGossip(t, alloc1.gossip, alloc2.gossip)
}
