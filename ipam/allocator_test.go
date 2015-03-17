package ipam

import (
	"fmt"
	"github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"sort"
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

func testAllocator(t *testing.T, name string, ourUID uint64, universeCIDR string) *Allocator {
	ourName, _ := router.PeerNameFromString(name)
	alloc, _ := NewAllocator(ourName, ourUID, universeCIDR)
	alloc.gossip = &mockGossipComms{t: t, name: name}
	alloc.startForTesting()
	return alloc
}

func (alloc *Allocator) addSpace(startAddr string, length uint32) *Allocator {
	alloc.ourSpaceSet.AddSpace(&MinSpace{net.ParseIP(startAddr), length})
	return alloc
}

func TestAllocFree(t *testing.T) {
	const (
		container1 = "abcdef"
		container2 = "baddf00d"
		container3 = "b01df00d"
		testAddr1  = "10.0.3.4"
		spaceSize  = 4
	)

	alloc := testAllocator(t, "01:00:00:01:00:00", ourUID, testAddr1+"/30").addSpace(testAddr1, spaceSize)
	defer alloc.Stop()

	addr1 := alloc.GetFor(container1, nil)
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")

	// Ask for another address for a different container and check it's different
	addr2 := alloc.GetFor(container2, nil)
	if addr2.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr2)
	}

	// Ask for the first container again and we should get the same address again
	addr1a := alloc.GetFor(container1, nil)
	wt.AssertEqualString(t, addr1a.String(), testAddr1, "address")

	// Now free the first one, and we should get it back when we ask
	alloc.Free(container1, net.ParseIP(testAddr1))
	addr3 := alloc.GetFor(container3, nil)
	wt.AssertEqualString(t, addr3.String(), testAddr1, "address")

	alloc.DeleteRecordsFor(container2)
	alloc.DeleteRecordsFor(container3)
	alloc.String() // force sync-up after async call
	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), spaceSize, "Total free addresses")
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

func equalUidSet(a, b uidSet) bool {
	if len(a) != len(b) {
		return false
	}
	for key, _ := range a {
		if _, found := b[key]; !found {
			return false
		}
	}
	return true
}

func assertEqualKeySet(t *testing.T, wanted, got uidSet) {
	if !equalUidSet(wanted, got) {
		wt.Fatalf(t, "Generated set does not match: wanted %v but got %v", wanted, got)
	}
}

func TestMultiSpaces(t *testing.T) {
	alloc := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/30")
	defer alloc.Stop()
	alloc.addSpace(testStart1, 1)
	alloc.addSpace(testStart2, 3)

	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 4, "Total free addresses")

	addr1 := alloc.GetFor("abcdef", nil)
	wt.AssertEqualString(t, addr1.String(), testStart1, "address")

	// First space should now be full and this address should come from second space
	addr2 := alloc.GetFor("fedcba", nil)
	wt.AssertEqualString(t, addr2.String(), testStart2, "address")
	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 2, "Total free addresses")
}

func TestEncodeMerge(t *testing.T) {
	alloc := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/22")
	defer alloc.Stop()
	alloc.addSpace(testStart1, 16)
	alloc.addSpace(testStart2, 32)

	startKeys := alloc.FullSet()
	encodedState := alloc.Encode(startKeys)

	alloc2 := testAllocator(t, "02:00:00:02:00:00", peerUID, testStart1+"/22").addSpace(testStart3, 32)
	defer alloc2.Stop()
	peerKeySet := alloc2.FullSet()

	newSet, err := alloc2.OnUpdate(encodedState)
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
	assertEqualKeySet(t, startKeys.(uidSet), newSet.(uidSet))

	// Do it again, and nothing should be new
	newSet, err = alloc2.OnUpdate(encodedState)
	wt.AssertNoErr(t, err)
	if newSet != nil && len(newSet.(uidSet)) != 0 {
		t.Fatal("Unexpected new items found")
	}

	// Now encode and merge the other way
	buf := alloc2.Encode(alloc2.FullSet())

	newSet, err = alloc.OnUpdate(buf)
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(alloc.peerInfo), 2, "spaceset count")

	decodedSpaceSet, found = alloc.peerInfo[peerUID]
	if !found {
		t.Fatal("Decoded allocator did not contain spaceSet")
	}
	if decodedSpaceSet.PeerName() != alloc2.ourName || decodedSpaceSet.UID() != peerUID || !alloc2.ourSpaceSet.Equal(decodedSpaceSet.(*PeerSpaceSet)) {
		t.Fatalf("Allocator not decoded as expected: %+v vs %+v", alloc2.ourSpaceSet, decodedSpaceSet)
	}

	// Returned set should be all the new ones - i.e. just alloc2's key
	assertEqualKeySet(t, peerKeySet.(uidSet), newSet.(uidSet))

	// Do it again, and nothing should be new
	newSet, err = alloc.OnUpdate(buf)
	wt.AssertNoErr(t, err)
	if newSet != nil && len(newSet.(uidSet)) != 0 {
		t.Fatal("Unexpected new items found")
	}
}

type mockMessage struct {
	dst     router.PeerName
	msgType byte
	buf     []byte
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
	t        *testing.T
	name     string
	messages []mockMessage
}

// Note: this style of verification, using equalByteBuffer, requires
// that the contents of messages are never re-ordered.  Which, for instance,
// requires they are not based off iterating through a map.

func (m *mockGossipComms) GossipBroadcast(buf []byte) error {
	if len(m.messages) == 0 {
		wt.Fatalf(m.t, "%s: Gossip broadcast message unexpected: \n%x", m.name, buf)
	} else if msg := m.messages[0]; msg.dst != router.UnknownPeerName {
		wt.Fatalf(m.t, "%s: Expected Gossip message to %s but got broadcast", m.name, msg.dst)
	} else if msg.buf != nil && !equalByteBuffer(msg.buf, buf) {
		wt.Fatalf(m.t, "%s: Gossip message not sent as expected: \nwant: %x\ngot : %x", m.name, msg.buf, buf)
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
	return nil
}

func (m *mockGossipComms) GossipUnicast(dstPeerName router.PeerName, buf []byte) error {
	if len(m.messages) == 0 {
		wt.Fatalf(m.t, "%s: Gossip message to %s unexpected: \n%s", m.name, dstPeerName, buf)
	} else if msg := m.messages[0]; msg.dst == router.UnknownPeerName {
		wt.Fatalf(m.t, "%s: Expected Gossip broadcast message but got dest %s", m.name, dstPeerName)
	} else if msg.dst != dstPeerName {
		wt.Fatalf(m.t, "%s: Expected Gossip message to %s but got dest %s", m.name, msg.dst, dstPeerName)
	} else if buf[0] != msg.msgType {
		wt.Fatalf(m.t, "%s: Expected Gossip message of type %d but got type %d", m.name, msg.msgType, buf[0])
	} else if msg.buf != nil && !equalByteBuffer(msg.buf, buf[1:]) {
		wt.Fatalf(m.t, "%s: Gossip message not sent as expected: \nwant: %x\ngot : %x", m.name, msg.buf, buf[1:])
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
	return nil
}

func ExpectMessage(alloc *Allocator, dst string, msgType byte, buf []byte) {
	m := alloc.gossip.(*mockGossipComms)
	dstPeerName, _ := router.PeerNameFromString(dst)
	m.messages = append(m.messages, mockMessage{dstPeerName, msgType, buf})
}

func ExpectBroadcastMessage(alloc *Allocator, buf []byte) {
	m := alloc.gossip.(*mockGossipComms)
	m.messages = append(m.messages, mockMessage{router.UnknownPeerName, 0, buf})
}

func CheckAllExpectedMessagesSent(allocs ...*Allocator) {
	for _, alloc := range allocs {
		m := alloc.gossip.(*mockGossipComms)
		if len(m.messages) > 0 {
			wt.Fatalf(m.t, "%s: Gossip message(s) not sent as expected: \n%x", m.name, m.messages)
		}
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

func TestGossip(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() { implTestGossip(t) })
}

func implTestGossip(t *testing.T) {
	common.InitDefaultLogging(true)
	const (
		donateSize     = 5
		donateStart    = "10.0.1.7"
		peerNameString = "02:00:00:02:00:00"
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	alloc1 := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/22")
	defer alloc1.Stop()
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network
	alloc2 := testAllocator(t, peerNameString, peerUID, testStart1+"/22")
	defer alloc2.Stop()
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(2 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Encode(alloc2.FullSet()))
	// At first, this peer has no space, so alloc1 should do nothing
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	alloc1.considerOurPosition()

	// Now give alloc2 some space and tell alloc1 about it
	alloc2.addSpace(testStart2, 10)
	ExpectBroadcastMessage(alloc2, nil)
	mockTime.SetTime(baseTime.Add(3 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Encode(alloc2.FullSet()))

	// Alloc1 should ask alloc2 for space when we call GetFor
	ExpectMessage(alloc1, peerNameString, msgSpaceRequest, encode(alloc1.ourSpaceSet))

	done := make(chan bool)
	go func() {
		alloc1.GetFor("somecontainer", nil)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, done)

	// Time out with no reply
	mockTime.SetTime(baseTime.Add(5 * time.Second))
	ExpectMessage(alloc1, peerNameString, msgSpaceRequest, encode(alloc1.ourSpaceSet))
	alloc1.considerOurPosition()
	AssertNothingSent(t, done)

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].(*MutableSpace).MinSpace.Size = donateSize
	alloc2.ourSpaceSet.version++

	donation := []Space{NewMinSpace(net.ParseIP(donateStart), donateSize)}
	msg := router.Concat([]byte{msgSpaceDonate}, GobEncode(donation, 1, alloc2.ourSpaceSet))
	ExpectBroadcastMessage(alloc1, nil)
	alloc1.OnGossipUnicast(alloc2.ourName, msg)
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 4, "Total free addresses")
	wt.AssertEqualuint64(t, alloc1.peerInfo[peerUID].Version(), 3, "Peer version")
	AssertSent(t, done)

	// Now looking to trigger a timeout
	alloc1.OnDead(alloc2.ourName, alloc2.ourUID) // Simulate call from router
	alloc1.String()                              // force sync

	// Now make it look like alloc2 is a tombstone so we can check the message
	alloc2.ourSpaceSet.MakeTombstone()
	ExpectBroadcastMessage(alloc1, encode(alloc2.ourSpaceSet))

	mockTime.SetTime(baseTime.Add(11 * time.Minute))
	alloc1.considerOurPosition()

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	alloc1.considerOurPosition()

	// Now move the time forward so alloc1 reclaims alloc2's storage
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	ExpectBroadcastMessage(alloc1, nil)
	alloc1.considerOurPosition()
}

// Test the election mechanism
func TestGossip2(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() { implTestGossip2(t) })
}

func implTestGossip2(t *testing.T) {
	const (
		donateSize     = 5
		donateStart    = "10.0.1.7"
		peerNameString = "02:00:00:02:00:00"
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	alloc1 := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/22")
	defer alloc1.Stop()
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network
	alloc2 := testAllocator(t, peerNameString, peerUID, testStart1+"/22")
	defer alloc2.Stop()
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(2 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Encode(alloc2.FullSet()))
	// At first, this peer has no space, so alloc1 should do nothing
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	mockTime.SetTime(baseTime.Add(3 * time.Second))
	alloc1.considerOurPosition()

	mockTime.SetTime(baseTime.Add(4 * time.Second))
	// On receipt of the GetFor, alloc1 should elect alloc2 as leader, because it has a higher UID
	ExpectMessage(alloc1, peerNameString, msgLeaderElected, nil)

	done := make(chan bool)
	go func() {
		alloc1.GetFor("somecontainer", nil)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, done)
	wt.AssertEqualInt(t, len(alloc1.inflight), 1, "inflight")

	// Time out with no reply
	mockTime.SetTime(baseTime.Add(15 * time.Second))
	ExpectMessage(alloc1, peerNameString, msgLeaderElected, nil)
	alloc1.considerOurPosition()
	AssertNothingSent(t, done)
	wt.AssertEqualInt(t, len(alloc1.inflight), 1, "inflight")

	// alloc2 receives the leader election message and broadcasts its winning state
	ExpectBroadcastMessage(alloc2, nil)
	msg := router.Concat([]byte{msgLeaderElected}, encode(alloc1.ourSpaceSet))
	alloc2.OnGossipUnicast(alloc1.ourName, msg)

	// On receipt of the broadcast, alloc1 should ask alloc2 for space
	ExpectMessage(alloc1, peerNameString, msgSpaceRequest, encode(alloc1.ourSpaceSet))
	alloc1.OnGossipBroadcast(alloc2.Encode(alloc2.FullSet()))

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].(*MutableSpace).MinSpace.Size = donateSize
	alloc2.ourSpaceSet.version++

	donation := []Space{NewMinSpace(net.ParseIP(donateStart), donateSize)}
	msg = router.Concat([]byte{msgSpaceDonate}, GobEncode(donation, 1, alloc2.ourSpaceSet))
	ExpectBroadcastMessage(alloc1, nil)
	alloc1.OnGossipUnicast(alloc2.ourName, msg)
	AssertSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)
}

func TestLeaks(t *testing.T) {
	const (
		universeSize = 16
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	// Give alloc1 the space from .0 to .7; nobody owns from .7 to .15
	alloc1 := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/27").addSpace(testStart1, 8)
	defer alloc1.Stop()
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network, that is managing some of that space
	alloc2 := testAllocator(t, "02:00:00:02:00:00", peerUID, testStart1+"/27")
	defer alloc2.Stop()
	alloc2.addSpace("10.0.1.8", 4)
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(20 * time.Second))

	alloc1.considerOurPosition()

	// Allow alloc1 to note the leak
	mockTime.SetTime(baseTime.Add(30 * time.Second))
	alloc1.considerOurPosition()

	// Alloc2 tells alloc1 about itself
	mockTime.SetTime(baseTime.Add(40 * time.Second))
	alloc2.considerOurPosition()

	assertNoOverlaps(t, alloc1, alloc2)

	mockTime.SetTime(baseTime.Add(50 * time.Second))
	alloc1.OnGossipBroadcast(alloc2.Encode(alloc2.FullSet()))

	assertNoOverlaps(t, alloc1, alloc2)

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	mockTime.SetTime(baseTime.Add(60 * time.Second))
	alloc1.considerOurPosition()

	// Now move the time forward so alloc1 reclaims the leak
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	alloc1.considerOurPosition()

	assertNoOverlaps(t, alloc1, alloc2)

	// Fixme: why does nobody gossip anything here?
}

func AssertSent(t *testing.T, ch <-chan bool) {
	timeout := time.After(time.Second)
	select {
	case <-ch:
		// This case is ok
	case <-timeout:
		wt.Fatalf(t, "Nothing sent on channel")
	}
}

func AssertNothingSent(t *testing.T, ch <-chan bool) {
	select {
	case val := <-ch:
		wt.Fatalf(t, "Unexpected value on channel: %t", val)
	default:
		// no message received
	}
}

// Re-claiming an address from a former life
func TestAllocatorClaim1(t *testing.T) {
	const (
		containerID = "deadbeef"
		container2  = "baddf00d"
		testAddr1   = "10.0.1.5"
		oldUID      = 6464646
	)

	alloc1 := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/22")
	defer alloc1.Stop()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	alloc1.considerOurPosition()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	alloc2 := testAllocator(t, "02:00:00:02:00:00", peerUID, testStart1+"/22").addSpace(testStart3, 32)
	defer alloc2.Stop()

	// alloc2 has an echo of the former state of alloc1
	alloc2.peerInfo[oldUID] = spaceSetWith(alloc1.ourName, oldUID, NewSpace(net.ParseIP(testStart1), 64))

	alloc1.decodeUpdate(alloc2.Encode(alloc2.FullSet()))
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")

	ExpectBroadcastMessage(alloc1, encode(tombstoneWith(alloc1.ourName, oldUID)))
	ExpectBroadcastMessage(alloc1, nil)
	done := make(chan bool)
	go func() {
		err := alloc1.Claim(containerID, net.ParseIP(testAddr1), nil)
		wt.AssertNoErr(t, err)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	AssertSent(t, done)
	alloc1.considerOurPosition()
}

// Claiming from another peer
func TestAllocatorClaim3(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() { implAllocatorClaim3(t) })
}

func implAllocatorClaim3(t *testing.T) {
	const (
		containerID     = "deadbeef"
		container2      = "baddf00d"
		testAddr1       = "10.0.1.63"
		peer1NameString = "01:00:00:01:00:00"
		peer2NameString = "02:00:00:02:00:00"
	)

	alloc1 := testAllocator(t, peer1NameString, ourUID, testStart1+"/22")
	defer alloc1.Stop()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	alloc1.considerOurPosition()

	alloc2 := testAllocator(t, peer2NameString, peerUID, testStart1+"/22").addSpace(testStart3, 32)
	defer alloc2.Stop()
	// alloc2 is managing the space that alloc1 wants to claim part of
	alloc2.addSpace(testStart1, 64)

	alloc1.decodeUpdate(alloc2.Encode(alloc2.FullSet()))
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")

	// Tell alloc1 that we want addr1, and it will send a message to alloc2
	addr1 := net.ParseIP(testAddr1)
	msgbuf := router.Concat(GobEncode(NewMinSpace(addr1, 1)), encode(alloc1.ourSpaceSet))
	ExpectMessage(alloc1, peer2NameString, msgSpaceClaim, msgbuf)
	done := make(chan bool)
	go func() {
		err := alloc1.Claim(containerID, addr1, nil)
		wt.AssertNoErr(t, err)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)

	AssertNothingSent(t, done)
	wt.AssertEqualInt(t, len(alloc1.claims), 1, "claims")
	alloc1.considerOurPosition()

	// Claiming the same address twice for the same container should stack up another claim record but not send another request
	done2 := make(chan bool)
	go func() {
		err := alloc1.Claim(containerID, addr1, nil)
		wt.AssertNoErr(t, err)
		done2 <- true
	}()
	time.Sleep(100 * time.Millisecond)

	AssertNothingSent(t, done2)
	wt.AssertEqualInt(t, len(alloc1.claims), 2, "claims")
	alloc1.considerOurPosition()

	// alloc2 receives this request, replies and broadcasts its new status
	ExpectMessage(alloc2, peer1NameString, msgSpaceDonate, nil)
	ExpectBroadcastMessage(alloc2, nil)
	err := alloc2.OnGossipUnicast(alloc1.ourName, router.Concat([]byte{msgSpaceClaim}, msgbuf))
	wt.AssertNoErr(t, err)
	AssertNothingSent(t, done)
	msgbuf = router.Concat(GobEncode([]Space{NewMinSpace(addr1, 1)}, 1, alloc2.ourSpaceSet))

	// alloc1 processes the response
	ExpectBroadcastMessage(alloc1, nil)
	err = alloc1.OnGossipUnicast(alloc2.ourName, router.Concat([]byte{msgSpaceDonate}, msgbuf))
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(alloc1.inflight), 0, "inflight")
	wt.AssertEqualInt(t, len(alloc1.claims), 0, "claims")
	AssertSent(t, done)
	AssertSent(t, done2)
}

// Claiming addresses with issues
func TestAllocatorClaim4(t *testing.T) {
	const (
		containerID = "deadbeef"
		container2  = "baddf00d"
		testAddr1   = "10.0.1.5"
		testAddr2   = "10.0.2.5"
		oldUID      = 6464646
	)

	alloc1 := testAllocator(t, "01:00:00:01:00:00", ourUID, testStart1+"/22")
	defer alloc1.Stop()
	alloc1.addSpace(testStart2, 64)

	// Claim an address that nobody is managing
	done := make(chan bool)
	go func() {
		err := alloc1.Claim(containerID, net.ParseIP(testAddr1), nil)
		wt.AssertNoErr(t, err)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)

	wt.AssertEqualInt(t, len(alloc1.claims), 1, "number of claims")
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 64, "free addresses")
	// Claiming the same address for different container should raise an error
	err := alloc1.Claim(container2, net.ParseIP(testAddr1), nil)
	wt.AssertErrorInterface(t, err, (*error)(nil), "duplicate claim error")
	wt.AssertEqualInt(t, len(alloc1.claims), 1, "number of claims")
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 64, "free addresses")

	// Now do the same again but for space it is managing
	err = alloc1.Claim(containerID, net.ParseIP(testAddr2), nil)
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(alloc1.claims), 1, "number of claims")
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 63, "free addresses")
	err = alloc1.Claim(containerID, net.ParseIP(testAddr2), nil)
	wt.AssertNoErr(t, err)
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 63, "free addresses")
	err = alloc1.Claim(container2, net.ParseIP(testAddr2), nil)
	wt.AssertErrorInterface(t, err, (*error)(nil), "duplicate claim error")
	wt.AssertEqualInt(t, len(alloc1.claims), 1, "number of claims")
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 63, "free addresses")

	// Original claim never returns as we are still waiting for someone to own it
	AssertNothingSent(t, done)
}

func TestCancel(t *testing.T) {
	wt.RunWithTimeout(t, 100*time.Second, func() { implTestCancel(t) })
}

type TestGossipRouter struct {
	peers map[router.PeerName]router.Gossiper
}

type TestGossipRouterClient struct {
	router *TestGossipRouter
	sender router.PeerName
}

func (router *TestGossipRouter) makeGossipClientFor(sender router.PeerName, gossiper router.Gossiper) router.Gossip {
	router.peers[sender] = gossiper
	return TestGossipRouterClient{router, sender}
}

func (client TestGossipRouterClient) GossipUnicast(dstPeerName router.PeerName, buf []byte) error {
	common.Debug.Printf("GossipUnicast from %s to %s", client.sender, dstPeerName)
	go client.router.peers[dstPeerName].OnGossipUnicast(client.sender, buf)
	return nil
}

func (client TestGossipRouterClient) GossipBroadcast(buf []byte) error {
	common.Debug.Printf("GossipBroadcast from %s", client.sender)
	for _, peer := range client.router.peers {
		go peer.OnGossipBroadcast(buf)
	}
	return nil
}

func implTestCancel(t *testing.T) {
	common.InitDefaultLogging(true)
	const (
		CIDR     = "10.0.1.7/22"
		peer1UID = 123456
		peer2UID = 789101
	)
	peer1Name, _ := router.PeerNameFromString("01:00:00:02:00:00")
	peer2Name, _ := router.PeerNameFromString("02:00:00:02:00:00")

	router := TestGossipRouter{make(map[router.PeerName]router.Gossiper)}

	alloc1, _ := NewAllocator(peer1Name, peer1UID, CIDR)
	alloc1.SetGossip(router.makeGossipClientFor(peer1Name, alloc1))

	alloc2, _ := NewAllocator(peer2Name, peer2UID, CIDR)
	alloc2.SetGossip(router.makeGossipClientFor(peer2Name, alloc2))

	alloc1.Start()
	alloc2.Start()

	// This is needed to tell one another about each other
	alloc1.OnGossipBroadcast(alloc2.Gossip())
	time.Sleep(100 * time.Millisecond)

	// Get some IPs
	res1 := alloc1.GetFor("foo", nil)
	common.Debug.Printf("res1 = %s", res1)
	res2 := alloc2.GetFor("bar", nil)
	common.Debug.Printf("res2 = %s", res2)
	if res1.Equal(res2) {
		wt.Fatalf(t, "Error: got same ips!")
	}

	// Now we're going to stop alloc2 and ask alloc1
	// for an allocation in alloc2's range
	alloc2.Stop()

	cancelChan := make(chan bool, 1)
	doneChan := make(chan error)
	go func () {
		err := alloc1.Claim("baz", res2, cancelChan)
		doneChan <- err
	} ()

	time.Sleep(1000 * time.Millisecond)
	cancelChan <- true
	err := <-doneChan
	if err != nil {
		wt.Fatalf(t, "Error: err should be nil!")
	}
}
