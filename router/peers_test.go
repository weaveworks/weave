package router

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO we should also test:
//
// - applying an incremental update, including the case where that
//   leads to an UnknownPeerError
//
// - the "improved update" calculation
//
// - non-gc of peers that are only referenced locally

func newNode(name PeerName) (*Peer, *Peers) {
	peer := NewLocalPeer(name, "", nil)
	peers := NewPeers(peer)
	return peer.Peer, peers
}

// Check that ApplyUpdate copies the whole topology from peers
func checkApplyUpdate(t *testing.T, peers *Peers) {
	dummyName, _ := PeerNameFromString("99:00:00:01:00:00")
	// We need a new node outside of the network, with a connection
	// into it.
	_, testBedPeers := newNode(dummyName)
	testBedPeers.AddTestConnection(peers.ourself.Peer)
	testBedPeers.ApplyUpdate(peers.EncodePeers(peers.Names()))

	checkTopologyPeers(t, true, testBedPeers.allPeersExcept(dummyName), peers.allPeers()...)
}

func TestPeersEncoding(t *testing.T) {
	const numNodes = 20
	const numIters = 1000
	var peer [numNodes]*Peer
	var ps [numNodes]*Peers
	for i := 0; i < numNodes; i++ {
		name, _ := PeerNameFromString(fmt.Sprintf("%02d:00:00:01:00:00", i))
		peer[i], ps[i] = newNode(name)
	}

	var conns []struct{ from, to int }
	for i := 0; i < numIters; i++ {
		oper := rand.Intn(2)
		switch oper {
		case 0:
			from, to := rand.Intn(numNodes), rand.Intn(numNodes)
			if from != to {
				if _, found := peer[from].connections[peer[to].Name]; !found {
					ps[from].AddTestConnection(peer[to])
					conns = append(conns, struct{ from, to int }{from, to})
					checkApplyUpdate(t, ps[from])
				}
			}
		case 1:
			if len(conns) > 0 {
				n := rand.Intn(len(conns))
				c := conns[n]
				ps[c.from].DeleteTestConnection(peer[c.to])
				ps[c.from].GarbageCollect()
				checkApplyUpdate(t, ps[c.from])
				conns = append(conns[:n], conns[n+1:]...)
			}
		}
	}
}

func garbageCollect(peers *Peers) []*Peer {
	var removed []*Peer
	peers.OnGC(func(peer *Peer) { removed = append(removed, peer) })
	peers.GarbageCollect()
	return removed
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
	ps1.AddTestConnection(p2)
	ps2.AddTestRemoteConnection(p1, p2)
	ps2.AddTestConnection(p1)
	ps2.AddTestConnection(p3)
	ps3.AddTestConnection(p1)
	ps1.AddTestConnection(p3)
	ps2.AddTestRemoteConnection(p1, p3)
	ps2.AddTestRemoteConnection(p3, p1)

	// Every peer is referenced, so nothing should be dropped
	require.Empty(t, garbageCollect(ps1), "peers removed")
	require.Empty(t, garbageCollect(ps2), "peers removed")
	require.Empty(t, garbageCollect(ps3), "peers removed")

	// Drop the connection from 2 to 3, and 3 isn't garbage-collected
	// because 1 has a connection to 3
	ps2.DeleteTestConnection(p3)
	require.Empty(t, garbageCollect(ps2), "peers removed")

	// Drop the connection from 1 to 3, and 3 will get removed by
	// garbage-collection
	ps1.DeleteTestConnection(p3)
	checkPeerArray(t, garbageCollect(ps1), p3)
}
