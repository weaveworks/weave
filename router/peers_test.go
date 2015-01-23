package router

import (
	wt "github.com/zettio/weave/testing"
	"testing"
)

func newNode(name PeerName) (*Peer, *Peers) {
	peer := NewPeer(name, 0, 0)
	peers := NewPeers(peer, func(*Peer) {})
	peers.FetchWithDefault(peer)
	return peer, peers
}

// Check that ApplyUpdate copies the whole topology from r1
func checkApplyUpdate(t *testing.T, peer *Peer, peers *Peers) {
	dummyName, _ := PeerNameFromString("99:00:00:01:00:00")
	// Testbed has to be a node outside of the network, with a connection into it
	testBed, testBedPeers := newNode(dummyName)
	testBedPeers.AddTestConnection(testBed, peer)
	testBedPeers.ApplyUpdate(peers.EncodeAllPeers())

	checkTopologyPeers(t, true, testBedPeers.allPeersExcept(dummyName), peers.allPeers()...)
}

func TestPeersEncoding(t *testing.T) {
	const (
		peer1NameString = "01:00:00:01:00:00"
		peer2NameString = "02:00:00:02:00:00"
		peer3NameString = "03:00:00:03:00:00"
	)
	var (
		peer1Name, _ = PeerNameFromString(peer1NameString)
		peer2Name, _ = PeerNameFromString(peer2NameString)
		peer3Name, _ = PeerNameFromString(peer3NameString)
	)

	// Create some peers
	p1, ps1 := newNode(peer1Name)
	p2, ps2 := newNode(peer2Name)
	p3, ps3 := newNode(peer3Name)

	// Now try adding some connections
	ps1.AddTestConnection(p1, p2)
	checkApplyUpdate(t, p1, ps1)
	ps2.AddTestConnection(p2, p1)
	checkApplyUpdate(t, p2, ps2)

	// Currently, the connection from 2 to 3 is one-way only
	ps2.AddTestConnection(p2, p3)
	checkApplyUpdate(t, p1, ps1)
	checkApplyUpdate(t, p2, ps2)
	checkApplyUpdate(t, p3, ps3)
}

func TestPeersGarbageCollection(t *testing.T) {
	const (
		peer1NameString = "01:00:00:01:00:00"
		peer2NameString = "02:00:00:02:00:00"
		peer3NameString = "03:00:00:03:00:00"
	)
	var (
		peer1Name, _ = PeerNameFromString(peer1NameString)
		peer2Name, _ = PeerNameFromString(peer2NameString)
		peer3Name, _ = PeerNameFromString(peer3NameString)
	)

	// Create some peers with some connections to each other
	p1, ps1 := newNode(peer1Name)
	p2, ps2 := newNode(peer2Name)
	p3, ps3 := newNode(peer3Name)
	ps1.AddTestConnection(p1, p2)
	ps2.AddTestRemoteConnection(p2, p1, p2)
	ps2.AddTestConnection(p2, p1)
	ps2.AddTestConnection(p2, p3)
	ps3.AddTestConnection(p3, p1)
	ps1.AddTestConnection(p1, p3)
	ps2.AddTestRemoteConnection(p2, p1, p3)
	ps2.AddTestRemoteConnection(p2, p3, p1)

	// Drop the connection from 2 to 3, and 3 isn't garbage-collected because 1 has a connection to 3
	ps2.DeleteTestConnection(p2, p3)
	peersRemoved := ps2.GarbageCollect()
	wt.AssertEmpty(t, peersRemoved, "peers removed")

	wt.AssertEmpty(t, ps1.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, ps2.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, ps3.GarbageCollect(), "peers removed")

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	ps1.DeleteTestConnection(p1, p3)
	peersRemoved = ps1.GarbageCollect()
	checkPeerArray(t, peersRemoved, p3)
}
