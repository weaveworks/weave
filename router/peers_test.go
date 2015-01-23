package router

import (
	wt "github.com/zettio/weave/testing"
	"testing"
)

func newNode(name PeerName) (*LocalPeer, *Peers) {
	localPeer := NewLocalPeer(name, nil)
	peers := NewPeers(localPeer.Peer, func(*Peer) {})
	peers.FetchWithDefault(localPeer.Peer)
	return localPeer, peers
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
	r2 := NewTestRouter(peer2Name)
	r3 := NewTestRouter(peer3Name)

	// Now try adding some connections
	ps1.AddTestConnection(p1, r2.Ourself.Peer)
	checkApplyUpdate(t, p1.Peer, ps1)
	r2.Peers.AddTestConnection(r2.Ourself, p1.Peer)
	checkApplyUpdate(t, r2.Ourself.Peer, r2.Peers)

	// Currently, the connection from 2 to 3 is one-way only
	r2.Peers.AddTestConnection(r2.Ourself, r3.Ourself.Peer)
	checkApplyUpdate(t, p1.Peer, ps1)
	checkApplyUpdate(t, r2.Ourself.Peer, r2.Peers)
	checkApplyUpdate(t, r3.Ourself.Peer, r3.Peers)
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
	r1 := NewTestRouter(peer1Name)
	r2 := NewTestRouter(peer2Name)
	r3 := NewTestRouter(peer3Name)
	r1.Peers.AddTestConnection(r1.Ourself, r2.Ourself.Peer)
	r2.Peers.AddTestRemoteConnection(r2.Ourself, r1.Ourself.Peer, r2.Ourself.Peer)
	r2.Peers.AddTestConnection(r2.Ourself, r1.Ourself.Peer)
	r2.Peers.AddTestConnection(r2.Ourself, r3.Ourself.Peer)
	r3.Peers.AddTestConnection(r3.Ourself, r1.Ourself.Peer)
	r1.Peers.AddTestConnection(r1.Ourself, r3.Ourself.Peer)
	r2.Peers.AddTestRemoteConnection(r2.Ourself, r1.Ourself.Peer, r3.Ourself.Peer)
	r2.Peers.AddTestRemoteConnection(r2.Ourself, r3.Ourself.Peer, r1.Ourself.Peer)

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
