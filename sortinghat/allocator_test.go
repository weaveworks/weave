package sortinghat

import (
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
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
	alloc := NewAllocator(ourName, ourUID, nil, net.ParseIP(testAddr1), 3)
	alloc.manageSpace(net.ParseIP(testAddr1), 3)

	addr1 := alloc.AllocateFor(containerID)
	if addr1.String() != testAddr1 {
		t.Fatalf("Expected address %s but got %s", testAddr1, addr1)
	}

	// Ask for another address and check it's different
	addr2 := alloc.AllocateFor(container2)
	if addr2.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr2)
	}

	// Now free the first one, and we should get it back when we ask
	alloc.Free(net.ParseIP(testAddr1))
	addr3 := alloc.AllocateFor(container2)
	if addr3.String() != testAddr1 {
		t.Fatalf("Expected address %s but got %s", testAddr1, addr1)
	}
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
	alloc := NewAllocator(ourName, ourUID, nil, net.ParseIP(testStart1), 1024)
	alloc.manageSpace(net.ParseIP(testStart1), 1)
	alloc.manageSpace(net.ParseIP(testStart2), 3)

	if n := alloc.ourSpaceSet.NumFreeAddresses(); n != 4 {
		t.Fatalf("Total free addresses should be 4 but got %d", n)
	}

	addr1 := alloc.AllocateFor(containerID)
	if addr1.String() != testStart1 {
		t.Fatalf("Expected address %s but got %s", testStart1, addr1)
	}

	// First space should now be full and this address should come from second space
	addr2 := alloc.AllocateFor(container2)
	if addr2.String() != testStart2 {
		t.Fatalf("Expected address %s but got %s", testStart2, addr2)
	}

	if n := alloc.ourSpaceSet.NumFreeAddresses(); n != 2 {
		t.Fatalf("Free addresses should be 2 but got %d", n)
	}
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
	alloc := NewAllocator(ourName, ourUID, nil, net.ParseIP(testStart1), 1024)
	alloc.manageSpace(net.ParseIP(testStart1), 16)
	alloc.manageSpace(net.ParseIP(testStart2), 32)

	encodedState, err := alloc.encode(true)
	wt.AssertNoErr(t, err)

	peerName, _ := router.PeerNameFromString(peerNameString)
	alloc2 := NewAllocator(peerName, peerUID, nil, net.ParseIP(testStart1), 1024)
	alloc2.manageSpace(net.ParseIP(testStart3), 32)
	encodedState2, err := alloc2.encode(true)
	wt.AssertNoErr(t, err)

	newBuf := alloc2.MergeRemoteState(encodedState)
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
	newBuf = alloc2.MergeRemoteState(encodedState)
	if newBuf != nil {
		t.Fatal("Unexpected new items found")
	}

	// Now encode and merge the other way
	buf, err := alloc2.encode(true)
	wt.AssertNoErr(t, err)

	newBuf = alloc.MergeRemoteState(buf)
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
	newBuf = alloc.MergeRemoteState(buf)
	if newBuf != nil {
		t.Fatal("Unexpected new items found")
	}
}

type mockMessage struct {
	dst router.PeerName
	buf []byte
}

type mockGossipComms struct {
	messages []mockMessage
}

func (f *mockGossipComms) Reset() {
	f.messages = make([]mockMessage, 0)
}

func (f *mockGossipComms) Gossip() {
}

func (f *mockGossipComms) GossipBroadcast(buf []byte) error {
	// fixme?
	return nil
}

func (f *mockGossipComms) GossipSendTo(dstPeerName router.PeerName, buf []byte) error {
	f.messages = append(f.messages, mockMessage{dstPeerName, buf})
	return nil
}

func TestGossip(t *testing.T) {
	const (
		testStart1     = "10.0.1.0"
		testStart2     = "10.0.1.1"
		origSize       = 10
		donateSize     = 5
		donateStart    = "10.0.1.7"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	mockGossip1 := new(mockGossipComms)
	alloc1 := NewAllocator(ourName, ourUID, mockGossip1, net.ParseIP(testStart1), 1024)
	alloc1.manageSpace(net.ParseIP(testStart1), 1)

	// Simulate another peer on the gossip network
	mockGossip2 := new(mockGossipComms)
	pn, _ := router.PeerNameFromString(peerNameString)
	alloc2 := NewAllocator(pn, peerUID, mockGossip2, net.ParseIP(testStart1), 1024)
	alloc2.manageSpace(net.ParseIP(testStart2), origSize)

	buf, err := alloc2.encode(true)
	wt.AssertNoErr(t, err)

	alloc1.MergeRemoteState(buf)

	if len(mockGossip1.messages) != 1 || mockGossip1.messages[0].dst.String() != peerNameString {
		t.Fatalf("Gossip message not sent as expected: %+v", mockGossip1)
	}

	if len(mockGossip2.messages) != 0 {
		t.Fatalf("Gossip message unexpected: %+v", mockGossip2)
	}

	mockGossip1.Reset()

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].GetMinSpace().Size = donateSize
	alloc2.ourSpaceSet.version++

	alloc2state, err := alloc2.encode(false)
	wt.AssertNoErr(t, err)

	size_encoding := intip4(donateSize) // hack! using intip4
	msg := router.Concat([]byte{gossipSpaceDonate}, net.ParseIP(donateStart).To4(), size_encoding, alloc2state)
	alloc1.NotifyMsg(pn, msg)
	if n := alloc1.ourSpaceSet.NumFreeAddresses(); n != 6 {
		t.Fatalf("Total free addresses should be 6 but got %d", n)
	}
	if n := alloc1.peerInfo[peerUID].Version(); n != 2 {
		t.Fatalf("Peer version should be 2 but got %d", n)
	}
}
