package router

import (
	wt "github.com/zettio/weave/testing"
	"testing"
)

// Check that ApplyUpdate copies the whole topology from r1
func checkApplyUpdate(t *testing.T, r1 *Router) {
	dummyName, _ := PeerNameFromString("99:00:00:01:00:00")
	// Testbed has to be a node outside of the network, with a connection into it
	testBed := NewTestRouter(t, dummyName)
	testBed.AddTestConnection(r1)
	testBed.Peers.ApplyUpdate(r1.Peers.EncodeAllPeers())

	checkTopologyPeers(t, true, testBed.Peers.allPeersExcept(dummyName), r1.Peers.allPeers()...)
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
	r1 := NewTestRouter(t, peer1Name)
	r2 := NewTestRouter(t, peer2Name)
	r3 := NewTestRouter(t, peer3Name)

	// Now try adding some connections
	r1.AddTestConnection(r2)
	checkApplyUpdate(t, r1)
	r2.AddTestConnection(r1)
	checkApplyUpdate(t, r2)

	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkApplyUpdate(t, r1)
	checkApplyUpdate(t, r2)
	checkApplyUpdate(t, r3)
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
	r1 := NewTestRouter(t, peer1Name)
	r2 := NewTestRouter(t, peer2Name)
	r3 := NewTestRouter(t, peer3Name)
	r1.AddTestConnection(r2)
	r2.AddTestRemoteConnection(r1, r2)
	r2.AddTestConnection(r1)
	r2.AddTestConnection(r3)
	r3.AddTestConnection(r1)
	r1.AddTestConnection(r3)
	r2.AddTestRemoteConnection(r1, r3)
	r2.AddTestRemoteConnection(r3, r1)

	// Drop the connection from 2 to 3, and 3 isn't garbage-collected because 1 has a connection to 3
	r2.DeleteTestConnection(r3)
	peersRemoved := r2.Peers.GarbageCollect()
	wt.AssertEmpty(t, peersRemoved, "peers removed")

	wt.AssertEmpty(t, r1.Peers.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, r2.Peers.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, r3.Peers.GarbageCollect(), "peers removed")

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	r1.DeleteTestConnection(r3)
	peersRemoved = r1.Peers.GarbageCollect()
	checkPeerArray(t, peersRemoved, tp(r3))
}
