package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	wt "github.com/zettio/weave/testing"
	"io"
	"sort"
	"testing"
)

type decodedPeerInfo struct {
	name     PeerName
	uid      uint64
	version  uint64
	connsBuf []byte
}

func (i *decodedPeerInfo) String() string {
	return fmt.Sprint("Peer ", i.name, " (v", i.version, ") (UID ", i.uid, ")")
}

type byName []*decodedPeerInfo

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i].name < a[j].name }

func AssertEqualPN(t *testing.T, got, wanted PeerName, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %s but got %s", wt.CallSite(4), desc, wanted, got)
	}
}

func checkPeerDetails(t *testing.T, got *decodedPeerInfo, wanted *Peer) {
	AssertEqualPN(t, got.name, wanted.Name, "Peer Name")
	wt.AssertEqualuint64(t, got.uid, wanted.UID, "Peer UID", 4)
	//Not checking the version because I haven't synthesised the data independently
	//and the 'real' version is often out of sync with another peers' view of it
	//wt.AssertEqualuint64(t, got.version, wanted.version, "Peer version", 4)
}

func checkConnsEncoding(t *testing.T, ourName PeerName, connsBuf []byte, connections []Connection) {
	checkConns := make(map[PeerName]bool)
	for _, conn := range connections {
		checkConns[conn.Remote().Name] = true
	}
	connsIterator(connsBuf, func(remoteNameByte []byte, _ string) {
		remoteName := PeerNameFromBin(remoteNameByte)
		if _, found := checkConns[remoteName]; found {
			delete(checkConns, remoteName)
		} else {
			t.Fatalf("%s: Unexpected connection decoded from %s to %s", wt.CallSite(5), ourName, remoteName)
		}
	})
	if len(checkConns) > 0 {
		t.Fatalf("%s: Expected connections not found: from %s to %v", wt.CallSite(3), ourName, checkConns)
	}
}

func decodePeerInfo(t *testing.T, decoder *gob.Decoder) []*decodedPeerInfo {
	peerInfo := make([]*decodedPeerInfo, 0)
	for {
		nameByte, uid, version, connsBuf, decErr := decodePeerNoConns(decoder)
		if decErr == io.EOF {
			break
		} else if decErr != nil {
			t.Fatalf("%s: Error when decoding peer (%s)", wt.CallSite(2), decErr)
		}
		peerInfo = append(peerInfo, &decodedPeerInfo{PeerNameFromBin(nameByte), uid, version, connsBuf})
	}
	return peerInfo
}

func checkBlank(t *testing.T, update []byte) {
	decoder := gob.NewDecoder(bytes.NewReader(update))
	peerInfo := decodePeerInfo(t, decoder)
	if len(peerInfo) != 0 {
		t.Fatalf("%s: Expected 0 items but got %s", wt.CallSite(2), peerInfo)
	}
}

func checkEncoding(t *testing.T, update []byte, routers []*Router, connections [][]Connection) {
	decoder := gob.NewDecoder(bytes.NewReader(update))

	// Peers can come either way round, so read them all in and sort them
	peerInfo := decodePeerInfo(t, decoder)
	sort.Sort(byName(peerInfo))
	N := len(peerInfo)
	if N != len(routers) {
		t.Fatalf("%s: Expected %d items but got %d: %s", wt.CallSite(2), len(routers), N, peerInfo)
	}
	for i := 0; i < N; i++ {
		checkPeerDetails(t, peerInfo[i], routers[i].Ourself.Peer)
		checkConnsEncoding(t, peerInfo[i].name, peerInfo[i].connsBuf, connections[i])
	}
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
	r1 := NewTestRouter(t, peer1Name, nil)
	r2 := NewTestRouter(t, peer2Name, nil)
	r3 := NewTestRouter(t, peer3Name, nil)

	// Check state when they have no connections
	checkEncoding(t, r1.Peers.EncodeAllPeers(), rs(r1), ca(nil))
	checkEncoding(t, r2.Peers.EncodeAllPeers(), rs(r2), ca(nil))

	// Now try adding some connections
	r1.AddTestConnection(r2)
	r2.AddTestConnection(r1)
	checkEncoding(t, r1.Peers.EncodeAllPeers(), rs(r1, r2), ca(cs(r2), nil))
	checkEncoding(t, r2.Peers.EncodeAllPeers(), rs(r1, r2), ca(nil, cs(r1)))
	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkEncoding(t, r2.Peers.EncodeAllPeers(), rs(r1, r2, r3), ca(nil, cs(r1, r3), nil))
	checkEncoding(t, r3.Peers.EncodeAllPeers(), rs(r3), ca(nil))
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
	r1 := NewTestRouter(t, peer1Name, nil)
	r2 := NewTestRouter(t, peer2Name, nil)
	r3 := NewTestRouter(t, peer3Name, nil)
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
	AssertEmpty(t, peersRemoved, "peers removed")

	AssertEmpty(t, r1.Peers.GarbageCollect(), "peers removed")
	AssertEmpty(t, r2.Peers.GarbageCollect(), "peers removed")
	AssertEmpty(t, r3.Peers.GarbageCollect(), "peers removed")

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	r1.DeleteTestConnection(r3)
	peersRemoved = r1.Peers.GarbageCollect()
	checkPeerArray(t, peersRemoved, rs(r3))
}
