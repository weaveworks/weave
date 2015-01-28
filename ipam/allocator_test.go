package ipam

import (
	"fmt"
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"strings"
	"testing"
	"time"
)

const (
	ourUID  = 123456
	peerUID = 654321
)

func TestAllocFree(t *testing.T) {
	var (
		containerID   = "deadbeef"
		container2    = "baddf00d"
		testAddr1     = "10.0.3.4"
		ourNameString = "01:00:00:01:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	alloc, _ := NewAllocator(ourName, ourUID, testAddr1+"/30")
	alloc.manageSpace(net.ParseIP(testAddr1), 4)

	addr1 := alloc.AllocateFor(containerID)
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
	var (
		containerID   = "deadbeef"
		container2    = "baddf00d"
		testStart1    = "10.0.1.0"
		testStart2    = "10.0.2.0"
		ourNameString = "01:00:00:01:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	alloc, _ := NewAllocator(ourName, ourUID, testStart1+"/30")
	alloc.manageSpace(net.ParseIP(testStart1), 1)
	alloc.manageSpace(net.ParseIP(testStart2), 3)

	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 4, "Total free addresses")

	addr1 := alloc.AllocateFor(containerID)
	wt.AssertEqualString(t, addr1.String(), testStart1, "address")

	// First space should now be full and this address should come from second space
	addr2 := alloc.AllocateFor(container2)
	wt.AssertEqualString(t, addr2.String(), testStart2, "address")
	wt.AssertEqualUint32(t, alloc.ourSpaceSet.NumFreeAddresses(), 2, "Total free addresses")
}

func TestEncodeMerge(t *testing.T) {
	var (
		testStart1     = "10.0.1.0"
		testStart2     = "10.0.2.0"
		testStart3     = "10.0.3.0"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	alloc, _ := NewAllocator(ourName, ourUID, testStart1+"/22")
	alloc.manageSpace(net.ParseIP(testStart1), 16)
	alloc.manageSpace(net.ParseIP(testStart2), 32)

	encodedState := alloc.Gossip()

	peerName, _ := router.PeerNameFromString(peerNameString)
	alloc2, _ := NewAllocator(peerName, peerUID, testStart1+"/22")
	alloc2.manageSpace(net.ParseIP(testStart3), 32)
	encodedState2 := alloc2.Gossip()

	newBuf, err := alloc2.OnGossip(encodedState)
	wt.AssertNoErr(t, err)
	if len(alloc2.peerInfo) != 2 {
		t.Fatalf("Decoded wrong number of spacesets: %d vs %d", len(alloc2.peerInfo), 2)
	}
	decodedSpaceSet, found := alloc2.peerInfo[ourUID]
	if !found {
		t.Fatal("Decoded allocator did not contain spaceSet")
	}
	if decodedSpaceSet.PeerName() != ourName || decodedSpaceSet.UID() != ourUID || !alloc.ourSpaceSet.Equal(decodedSpaceSet.(*PeerSpaceSet)) {
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
	if len(alloc.peerInfo) != 2 {
		t.Fatalf("Decoded wrong number of spacesets: %d vs %d", len(alloc.peerInfo), 2)
	}
	decodedSpaceSet, found = alloc.peerInfo[peerUID]
	if !found {
		t.Fatal("Decoded allocator did not contain spaceSet")
	}
	if decodedSpaceSet.PeerName() != peerName || decodedSpaceSet.UID() != peerUID || !alloc2.ourSpaceSet.Equal(decodedSpaceSet.(*PeerSpaceSet)) {
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

func (f *mockGossipComms) Reset() {
	f.messages = make([]mockMessage, 0)
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

type mockTimeProvider struct {
	myTime  time.Time
	pending []mockTimer
}

type mockTimer struct {
	when time.Time
	f    func()
}

func (m *mockTimeProvider) SetTime(t time.Time) { m.myTime = t }
func (m *mockTimeProvider) Now() time.Time      { return m.myTime }

func (m *mockTimeProvider) AfterFunc(d time.Duration, f func()) {
	t := mockTimer{
		when: m.myTime.Add(d),
		f:    f,
	}
	m.pending = append(m.pending, t)
}

func goFunc(arg interface{}) {
}

func (m *mockTimeProvider) runPending(newTime time.Time) {
	m.myTime = newTime
	for _, t := range m.pending {
		if t.when.After(m.myTime) {
			t.f()
		}
	}
}

func assertNoOverlaps(t *testing.T, allocs ...*Allocator) {
	for _, alloc := range allocs {
		if alloc.lookForOverlaps() {
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
		testStart1     = "10.0.1.0"
		testStart2     = "10.0.1.1"
		origSize       = 10
		donateSize     = 5
		donateStart    = "10.0.1.7"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1, _ := NewAllocator(ourName, ourUID, testStart1+"/22")
	alloc1.SetGossip(mockGossip1)
	alloc1.startForTesting()
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network
	mockGossip2 := new(mockGossipComms)
	pn, _ := router.PeerNameFromString(peerNameString)
	alloc2, _ := NewAllocator(pn, peerUID, testStart1+"/22")
	alloc2.SetGossip(mockGossip2)
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(2 * time.Second))

	alloc1.OnGossipBroadcast(alloc2.Gossip())
	// At first, this peer has no space, so alloc1 should do nothing
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Give alloc1 some space so we can test the choosing algorithm
	alloc1.manageSpace(net.ParseIP(testStart1), 1)
	alloc1.state = allocStateNeutral
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)

	// Now give alloc2 some space and tell alloc1 about it
	mockTime.SetTime(baseTime.Add(3 * time.Second))
	alloc2.manageSpace(net.ParseIP(testStart2), origSize)

	alloc1.OnGossipBroadcast(alloc2.Gossip())

	// Alloc1 should ask alloc2 for space
	mockGossip1.VerifyMessage(t, peerNameString, gossipSpaceRequest, encode(alloc1.ourSpaceSet))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Time out with no reply
	mockTime.SetTime(baseTime.Add(5 * time.Second))
	alloc1.considerWhileLocked()

	mockGossip1.VerifyMessage(t, peerNameString, gossipSpaceRequest, encode(alloc1.ourSpaceSet))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].GetMinSpace().Size = donateSize
	alloc2.ourSpaceSet.version++

	alloc2state := encode(alloc2.ourSpaceSet)

	size_encoding := intip4(donateSize) // hack! using intip4
	msg := router.Concat([]byte{gossipSpaceDonate}, net.ParseIP(donateStart).To4(), size_encoding, alloc2state)
	alloc1.OnGossipUnicast(pn, msg)
	wt.AssertEqualUint32(t, alloc1.ourSpaceSet.NumFreeAddresses(), 6, "Total free addresses")
	wt.AssertEqualuint64(t, alloc1.peerInfo[peerUID].Version(), 2, "Peer version")

	mockGossip1.VerifyBroadcastMessage(t, encode(alloc1.ourSpaceSet))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Now looking to trigger a timeout
	alloc1.OnDead(pn, peerUID) // Simulate call from router
	mockTime.SetTime(baseTime.Add(11 * time.Minute))
	alloc1.considerWhileLocked()

	// Now make it look like alloc2 is a tombstone so we can check the message
	alloc2.ourSpaceSet.MakeTombstone()
	mockGossip1.VerifyBroadcastMessage(t, encode(alloc2.ourSpaceSet))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Now move the time forward so alloc1 reclaims alloc2's storage
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	alloc1.considerWhileLocked()
	mockGossip1.VerifyBroadcastMessage(t, encode(alloc1.ourSpaceSet))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)
}

func TestLeaks(t *testing.T) {
	const (
		testStart1     = "10.0.1.0"
		testStart2     = "10.0.1.8"
		origSize       = 10
		universeSize   = 16
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	baseTime := time.Date(2014, 9, 7, 12, 0, 0, 0, time.UTC)
	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1, _ := NewAllocator(ourName, ourUID, testStart1+"/27")
	alloc1.state = allocStateNeutral
	alloc1.SetGossip(mockGossip1)
	mockTime := new(mockTimeProvider)
	mockTime.SetTime(baseTime)
	alloc1.timeProvider = mockTime

	mockTime.SetTime(baseTime.Add(1 * time.Second))

	// Simulate another peer on the gossip network
	mockGossip2 := new(mockGossipComms)
	pn, _ := router.PeerNameFromString(peerNameString)
	alloc2, _ := NewAllocator(pn, peerUID, testStart1+"/27")
	alloc2.SetGossip(mockGossip2)
	alloc2.timeProvider = alloc1.timeProvider

	mockTime.SetTime(baseTime.Add(20 * time.Second))

	// Give alloc1 the space from .0 to .7; nobody owns from .7 to .15
	alloc1.manageSpace(net.ParseIP(testStart1), 8)
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)

	// Allow alloc1 to note the leak
	mockTime.SetTime(baseTime.Add(30 * time.Second))
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Alloc2 is managing some of that space; it tells alloc1 about itself
	mockTime.SetTime(baseTime.Add(40 * time.Second))
	alloc2.manageSpace(net.ParseIP(testStart2), 4)
	alloc2.considerWhileLocked()
	mockGossip2.VerifyNoMoreMessages(t)

	assertNoOverlaps(t, alloc1, alloc2)

	mockTime.SetTime(baseTime.Add(50 * time.Second))
	alloc1.OnGossipBroadcast(alloc2.Gossip())
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	assertNoOverlaps(t, alloc1, alloc2)

	// Allow alloc1 to note the leak, but at this point it doesn't do anything
	mockTime.SetTime(baseTime.Add(60 * time.Second))
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	// Now move the time forward so alloc1 reclaims the leak
	mockTime.SetTime(baseTime.Add(12 * time.Minute))
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	assertNoOverlaps(t, alloc1, alloc2)
}

func TestGossipBreakage(t *testing.T) {
	const (
		ourNameString = "01:00:00:01:00:00"
		testStart1    = "10.0.1.0"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1, _ := NewAllocator(ourName, ourUID, testStart1+"/22")
	alloc1.SetGossip(mockGossip1)
	alloc1.startForTesting()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	// Simulate another peer on the gossip network, but WITH SAME PEER NAME
	mockGossip2 := new(mockGossipComms)
	alloc2, _ := NewAllocator(ourName, peerUID, testStart1+"/22")
	alloc2.SetGossip(mockGossip2)

	// Give alloc1 some space
	alloc1.manageSpace(net.ParseIP(testStart1), 0)
	alloc1.state = allocStateNeutral
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)

	// Call decodeUpdate rather than OnGossip which also calls considerOurPosition
	alloc2.decodeUpdate(alloc1.Gossip())
	wt.AssertStatus(t, alloc2.state, allocStateLeaderless, "allocator state")
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)
}

func TestAllocatorClaim1(t *testing.T) {
	const (
		containerID    = "deadbeef"
		container2     = "baddf00d"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
		testStart1     = "10.0.1.0"
		testStart3     = "10.0.3.0"
		testAddr1      = "10.0.1.5"
		testAddr2      = "10.0.1.6"
		oldUID         = 6464646
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1, _ := NewAllocator(ourName, ourUID, testStart1+"/22")
	alloc1.SetGossip(mockGossip1)
	alloc1.startForTesting()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")

	alloc1.considerWhileLocked()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	mockGossip1.VerifyNoMoreMessages(t)

	mockGossip2 := new(mockGossipComms)
	peerName, _ := router.PeerNameFromString(peerNameString)
	alloc2, _ := NewAllocator(peerName, peerUID, testStart1+"/22")
	alloc2.manageSpace(net.ParseIP(testStart3), 32)

	// alloc2 has an echo of the former state of alloc1
	alloc2.peerInfo[oldUID] = spaceSetWith(ourName, oldUID, NewSpace(net.ParseIP(testStart1), 64))

	alloc1.decodeUpdate(alloc2.Gossip())
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	err := alloc1.Claim(containerID, net.ParseIP(testAddr1))
	wt.AssertNoErr(t, err)
	alloc1.considerWhileLocked()
	mockGossip1.VerifyBroadcastMessage(t, encode(tombstoneWith(ourName, oldUID)))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)
}

// Same as TestAllocatorClaim1 but the claim and the gossip happen the other way round
func TestAllocatorClaim2(t *testing.T) {
	const (
		containerID    = "deadbeef"
		container2     = "baddf00d"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
		testStart1     = "10.0.1.0"
		testStart3     = "10.0.3.0"
		testAddr1      = "10.0.1.5"
		testAddr2      = "10.0.1.6"
		oldUID         = 6464646
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1, _ := NewAllocator(ourName, ourUID, testStart1+"/22")
	alloc1.SetGossip(mockGossip1)
	alloc1.startForTesting()
	wt.AssertStatus(t, alloc1.state, allocStateLeaderless, "allocator state")
	alloc1.considerWhileLocked()

	mockGossip2 := new(mockGossipComms)
	peerName, _ := router.PeerNameFromString(peerNameString)
	alloc2, _ := NewAllocator(peerName, peerUID, testStart1+"/22")
	alloc2.manageSpace(net.ParseIP(testStart3), 32)

	// alloc2 has an echo of the former state of alloc1
	alloc2.peerInfo[oldUID] = spaceSetWith(ourName, oldUID, NewSpace(net.ParseIP(testStart1), 64))

	err := alloc1.Claim(containerID, net.ParseIP(testAddr1))
	wt.AssertNoErr(t, err)
	alloc1.considerWhileLocked()
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)

	alloc1.decodeUpdate(alloc2.Gossip())
	wt.AssertStatus(t, alloc1.state, allocStateNeutral, "allocator state")
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)
	alloc1.considerWhileLocked()
	mockGossip1.VerifyBroadcastMessage(t, encode(tombstoneWith(ourName, oldUID)))
	mockGossip1.VerifyNoMoreMessages(t)
	mockGossip2.VerifyNoMoreMessages(t)
}
