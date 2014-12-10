package sortinghat

import (
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
)

func TestAllocFree(t *testing.T) {
	var (
		containerID   = "deadbeef"
		container2    = "baddf00d"
		testAddr1     = "10.0.3.4"
		ourNameString = "01:00:00:01:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	alloc := NewAllocator(ourName, nil, net.ParseIP(testAddr1), 3)
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

func TestMultiSpaces(t *testing.T) {
	var (
		containerID   = "deadbeef"
		container2    = "baddf00d"
		testStart1    = "10.0.1.0"
		testStart2    = "10.0.2.0"
		ourNameString = "01:00:00:01:00:00"
	)

	ourName, _ := router.PeerNameFromString(ourNameString)
	alloc := NewAllocator(ourName, nil, net.ParseIP(testStart1), 1024)
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

type fakeMessage struct {
	dst router.PeerName
	buf []byte
}

type fakeGossipComms struct {
	messages []fakeMessage
}

func (f *fakeGossipComms) Reset() {
	f.messages = make([]fakeMessage, 0)
}

func (f *fakeGossipComms) Gossip() {
}

func (f *fakeGossipComms) GossipSendTo(dstPeerName router.PeerName, buf []byte) error {
	f.messages = append(f.messages, fakeMessage{dstPeerName, buf})
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
	fakeGossip1 := new(fakeGossipComms)
	alloc1 := NewAllocator(ourName, fakeGossip1, net.ParseIP(testStart1), 1024)
	alloc1.manageSpace(net.ParseIP(testStart1), 1)

	// Simulate another peer on the gossip network
	fakeGossip2 := new(fakeGossipComms)
	pn, _ := router.PeerNameFromString(peerNameString)
	alloc2 := NewAllocator(pn, fakeGossip2, net.ParseIP(testStart1), 1024)
	alloc2.manageSpace(net.ParseIP(testStart2), origSize)

	buf, err := alloc2.encode()
	wt.AssertNoErr(t, err)

	alloc1.MergeRemoteState(buf, true)

	if len(fakeGossip1.messages) != 1 || fakeGossip1.messages[0].dst.String() != peerNameString {
		t.Fatalf("Gossip message not sent as expected: %+v", fakeGossip1)
	}

	if len(fakeGossip2.messages) != 0 {
		t.Fatalf("Gossip message unexpected: %+v", fakeGossip2)
	}

	fakeGossip1.Reset()

	// Now make it look like alloc2 has given up half its space
	alloc2.ourSpaceSet.spaces[0].Size = donateSize
	alloc2.ourSpaceSet.version++

	alloc2state, err := alloc2.encode()
	wt.AssertNoErr(t, err)

	size_encoding := intip4(donateSize) // hack! using intip4
	msg := router.Concat([]byte{gossipSpaceDonate}, net.ParseIP(donateStart).To4(), size_encoding, alloc2state)
	alloc1.NotifyMsg(pn, msg)
	if n := alloc1.ourSpaceSet.NumFreeAddresses(); n != 6 {
		t.Fatalf("Total free addresses should be 6 but got %d", n)
	}
	if n := alloc1.spacesets[pn].version; n != 2 {
		t.Fatalf("Peer version should be 2 but got %d", n)
	}
}
