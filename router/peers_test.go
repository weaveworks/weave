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

func checkConnsEncoding(t *testing.T, ourName PeerName, connsBuf []byte, connections map[PeerName]Connection) {
	checkConns := make(map[PeerName]bool)
	for _, conn := range connections {
		checkConns[conn.Remote().Name] = true
	}
	connsIterator(connsBuf, func(remoteNameByte []byte, _ string) {
		remoteName := PeerNameFromBin(remoteNameByte)
		if _, found := checkConns[remoteName]; found {
			delete(checkConns, remoteName)
		} else {
			wt.Fatalf(t, "Unexpected connection decoded from %s to %s", ourName, remoteName)
		}
	})
	if len(checkConns) > 0 {
		wt.Fatalf(t, "Expected connections not found: from %s to %v", ourName, checkConns)
	}
}

func decodePeerInfo(t *testing.T, decoder *gob.Decoder) []*decodedPeerInfo {
	peerInfo := make([]*decodedPeerInfo, 0)
	for {
		nameByte, uid, version, connsBuf, decErr := decodePeerNoConns(decoder)
		if decErr == io.EOF {
			break
		} else if decErr != nil {
			wt.Fatalf(t, "Error when decoding peer (%s)", decErr)
		}
		peerInfo = append(peerInfo, &decodedPeerInfo{PeerNameFromBin(nameByte), uid, version, connsBuf})
	}
	return peerInfo
}

func checkEncoding(t *testing.T, update []byte, wantedPeers ...*Peer) {
	decoder := gob.NewDecoder(bytes.NewReader(update))

	// Peers can come in any order, so read them all in and sort them
	peerInfo := decodePeerInfo(t, decoder)
	sort.Sort(byName(peerInfo))
	N := len(peerInfo)
	if N != len(wantedPeers) {
		wt.Fatalf(t, "Expected %d items but got %d: %s", len(wantedPeers), N, peerInfo)
	}
	for i, wanted := range wantedPeers {
		if peerInfo[i].name != wanted.Name {
			wt.Fatalf(t, "Expected Peer Name %s but got %s", wanted.Name, peerInfo[i].name)
		}
		wt.AssertEqualuint64(t, peerInfo[i].uid, wanted.UID, "Peer UID")
		//Not checking the version because I haven't synthesised the data independently
		//and the 'real' version is often out of sync with another peers' view of it
		//wt.AssertEqualuint64(t, peerInfo[i].version, wanted.version, "Peer version")
		checkConnsEncoding(t, peerInfo[i].name, peerInfo[i].connsBuf, wanted.connections)
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
	r1 := NewTestRouter(t, peer1Name)
	r2 := NewTestRouter(t, peer2Name)
	r3 := NewTestRouter(t, peer3Name)

	// Check state when they have no connections
	checkEncoding(t, r1.Peers.EncodeAllPeers(), tp(r1))
	checkEncoding(t, r2.Peers.EncodeAllPeers(), tp(r2))

	// Now try adding some connections
	r1.AddTestConnection(r2)
	r2.AddTestConnection(r1)
	checkEncoding(t, r1.Peers.EncodeAllPeers(), tp(r1, r2), tp(r2))
	checkEncoding(t, r2.Peers.EncodeAllPeers(), tp(r1), tp(r2, r1))
	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkEncoding(t, r2.Peers.EncodeAllPeers(), tp(r1), tp(r2, r1, r3), tp(r3))
	checkEncoding(t, r3.Peers.EncodeAllPeers(), tp(r3))
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
