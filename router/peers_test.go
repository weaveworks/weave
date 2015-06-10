package router

import (
	"fmt"
	wt "github.com/weaveworks/weave/testing"
	"math/rand"
	"testing"
	"time"
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
	peers := NewPeers(peer, func(*Peer) {})
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

func TestShortIDCollisions(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	_, peers := newNode(PeerName(1 << PeerShortIDBits))

	// Make enough peers that short id collisions are
	// overwhelmingly likely
	ps := make([]*Peer, 1<<PeerShortIDBits)
	for i := 0; i < 1<<PeerShortIDBits; i++ {
		ps[i] = NewPeer(PeerName(i), "", PeerUID(i), 0,
			PeerShortID(rng.Intn(1<<PeerShortIDBits)))
	}

	shuffle := func() {
		for i := range ps {
			j := rng.Intn(i + 1)
			ps[i], ps[j] = ps[j], ps[i]
		}
	}

	// Fill peers
	shuffle()
	for _, p := range ps {
		peers.addByShortID(p)
	}

	// Check invariants
	counts := make([]int, 1<<PeerShortIDBits)
	saw := func(p *Peer) {
		if p != peers.ourself.Peer {
			counts[p.UID]++
		}
	}

	for shortID, entry := range peers.byShortID {
		if entry.peer == nil {
			// no principal peer for this short id, so
			// others must be empty
			if len(entry.others) != 0 {
				t.Fatal()
			}

			continue
		}

		if entry.peer.ShortID != shortID {
			t.Fatal()
		}

		saw(entry.peer)

		for _, p := range entry.others {
			saw(p)

			if p.ShortID != shortID {
				t.Fatal()
			}

			// the principal peer should have the lowest name
			if p.Name < entry.peer.Name {
				t.Fatal()
			}
		}
	}

	for _, n := range counts {
		if n != 1 {
			t.Fatal()
		}
	}

	// Delete all the peers
	shuffle()
	for _, p := range ps {
		peers.deleteByShortID(p)
	}

	for _, entry := range peers.byShortID {
		if entry.peer != nil && entry.peer != peers.ourself.Peer {
			t.Fatal()
		}

		if len(entry.others) != 0 {
			t.Fatal()
		}
	}
}

// Test the easy case of short id reassignment, when few short ids are taken
func TestShortIDReassignmentEasy(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	_, peers := newNode(PeerName(0))

	for i := 1; i <= 10; i++ {
		peers.FetchWithDefault(NewPeer(PeerName(i), "", PeerUID(i), 0,
			PeerShortID(rng.Intn(1<<PeerShortIDBits))))
	}

	checkShortIDReassignment(t, peers)
}

// Test the hard case of short id reassignment, when most short ids are taken
func TestShortIDReassignmentHard(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	_, peers := newNode(PeerName(1 << PeerShortIDBits))

	// Take all short ids
	ps := make([]*Peer, 1<<PeerShortIDBits)
	for i := 0; i < 1<<PeerShortIDBits; i++ {
		ps[i] = NewPeer(PeerName(i), "", PeerUID(i), 0,
			PeerShortID(i))
		peers.addByShortID(ps[i])
	}

	// As all short ids are taken, an attempted reassigment won't
	// do anything
	oldShortID := peers.ourself.ShortID
	peers.reassignLocalShortID()
	if peers.ourself.ShortID != oldShortID {
		t.Fatal()
	}

	// Free up a few ids
	for i := 0; i < 10; i++ {
		x := rng.Intn(len(ps))
		if ps[x] != nil {
			peers.deleteByShortID(ps[x])
			ps[x] = nil
		}
	}

	checkShortIDReassignment(t, peers)
}

func checkShortIDReassignment(t *testing.T, peers *Peers) {
	oldShortID := peers.ourself.ShortID
	peers.reassignLocalShortID()
	if peers.ourself.ShortID == oldShortID {
		t.Fatal()
	}

	if peers.byShortID[peers.ourself.ShortID].peer != peers.ourself.Peer {
		t.Fatal()
	}
}

func TestShortIDInvalidation(t *testing.T) {
	_, peers := newNode(PeerName(1 << PeerShortIDBits))

	// need to use a short id that is not the local peer's
	shortID := peers.ourself.ShortID + 1

	assertInvalidateShortIDs := func(expect bool) {
		if peers.invalidateShortIDsPending != expect {
			t.Fatal()
		}
		peers.invalidateShortIDsPending = false
	}

	// The use of a fresh short id does not cause invalidation
	a := NewPeer(PeerName(1), "", PeerUID(1), 0, shortID)
	peers.addByShortID(a)
	assertInvalidateShortIDs(false)

	// An addition which does not change the mapping
	b := NewPeer(PeerName(2), "", PeerUID(2), 0, shortID)
	peers.addByShortID(b)
	assertInvalidateShortIDs(false)

	// An addition which does change the mapping
	c := NewPeer(PeerName(0), "", PeerUID(0), 0, shortID)
	peers.addByShortID(c)
	assertInvalidateShortIDs(true)

	// A deletion which does not change the mapping
	peers.deleteByShortID(b)
	assertInvalidateShortIDs(false)

	// A deletion which does change the mapping
	peers.deleteByShortID(c)
	assertInvalidateShortIDs(true)

	// Deleting the last peer with a short id does not cause invalidation
	peers.deleteByShortID(a)
	assertInvalidateShortIDs(false)

	// But subsequent reuse that a short id does cause invalidation
	peers.addByShortID(a)
	assertInvalidateShortIDs(true)
}

func TestShortIDPropagation(t *testing.T) {
	_, peers1 := newNode(PeerName(0))
	_, peers2 := newNode(PeerName(1))

	peers1.AddTestConnection(peers2.ourself.Peer)
	peers1.ApplyUpdate(peers2.EncodePeers(peers2.Names()))
	peers12, _ := peers1.Fetch(PeerName(1))
	old := peers12.PeerSummary

	peers2.reassignLocalShortID()
	peers1.ApplyUpdate(peers2.EncodePeers(peers2.Names()))
	if peers12.Version == old.Version || peers12.ShortID == old.ShortID {
		t.Fatal()
	}
}

// Test the case where all short ids are taken, but then some peers go
// away, so the local peer reassigns
func TestDeferredShortIDReassignment(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	_, us := newNode(PeerName(1 << PeerShortIDBits))

	// Connect us to other peers occupying all short ids
	others := make([]*Peers, 1<<PeerShortIDBits)
	for i := range others {
		_, others[i] = newNode(PeerName(i))
		others[i].setLocalShortID(PeerShortID(i))
		us.AddTestConnection(others[i].ourself.Peer)
	}

	// Check that, as expected, the local peer does not own its
	// short id
	if us.byShortID[us.ourself.ShortID].peer == us.ourself.Peer {
		t.Fatal()
	}

	// Disconnect one peer, and we should now be able to claim its
	// short id
	other := others[rng.Intn(1<<PeerShortIDBits)]
	us.DeleteTestConnection(other.ourself.Peer)
	us.GarbageCollect()

	if us.byShortID[us.ourself.ShortID].peer != us.ourself.Peer {
		t.Fatal()
	}
}
