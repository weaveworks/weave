package router

import (
	"fmt"
	wt "github.com/zettio/weave/testing"
	"math/rand"
	"testing"
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
	peer := NewPeer(name, "", 0, 0)
	peers := NewPeers(peer, func(*Peer) {})
	peers.FetchWithDefault(peer)
	return peer, peers
}

// Check that ApplyUpdate copies the whole topology from peers
func checkApplyUpdate(t *testing.T, peers *Peers) {
	dummyName, _ := PeerNameFromString("99:00:00:01:00:00")
	// We need a new node outside of the network, with a connection
	// into it.
	_, testBedPeers := newNode(dummyName)
	testBedPeers.AddTestConnection(peers.ourself)
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

	conns := make([]struct{ from, to int }, 0)
	for i := 0; i < numIters; i++ {
		oper := rand.Intn(2)
		switch oper {
		case 0:
			from, to := rand.Intn(numNodes), rand.Intn(numNodes)
			if from != to {
				if _, found := peer[from].ConnectionTo(peer[to].Name); !found {
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
	wt.AssertEmpty(t, ps1.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, ps2.GarbageCollect(), "peers removed")
	wt.AssertEmpty(t, ps3.GarbageCollect(), "peers removed")

	// Drop the connection from 2 to 3, and 3 isn't garbage-collected
	// because 1 has a connection to 3
	ps2.DeleteTestConnection(p3)
	wt.AssertEmpty(t, ps2.GarbageCollect(), "peers removed")

	// Drop the connection from 1 to 3, and 3 will get removed by
	// garbage-collection
	ps1.DeleteTestConnection(p3)
	checkPeerArray(t, ps1.GarbageCollect(), p3)
}
