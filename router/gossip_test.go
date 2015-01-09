package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	wt "github.com/zettio/weave/common"
	"io"
	"net"
	"sort"
	"testing"
)

type peerQueue struct {
	peers []*Peer
}

func (q *peerQueue) clear() {
	q.peers = nil
}

// Construct a Router object with a mock interface to check peer
// garbage-collection, and without firing up any ancilliary goroutines
func NewTestRouter(t *testing.T, name PeerName, queue *peerQueue) *Router {
	onMacExpiry := func(mac net.HardwareAddr, peer *Peer) {}
	onPeerGC := func(peer *Peer) {
		//t.Log("Removing unreachable", peer)
		queue.peers = append(queue.peers, peer)
	}
	router := newRouter(nil, name, nil, 10, 1024, nil, onMacExpiry, onPeerGC)
	router.ConnectionMaker = &ConnectionMaker{
		ourself:   router.Ourself,
		queryChan: make(chan *ConnectionMakerInteraction, ChannelSize)}
	router.Routes = NewRoutes(router.Ourself.Peer, router.Peers)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

func (r1 *Router) AddTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	r1.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	r1.Ourself.Peer.connections[toName] = &RemoteConnection{r1.Ourself.Peer, toPeer, ""}
	r1.Ourself.Peer.version += 1
}

func (r1 *Router) DeleteTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer, _ := r1.Peers.Fetch(toName)
	toPeer.DecrementLocalRefCount()
	delete(r1.Ourself.Peer.connections, toName)
	r1.Ourself.Peer.version += 1
}

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

func AssertEmpty(t *testing.T, array []*Peer, desc string) {
	if len(array) != 0 {
		t.Fatalf("%s: Expected empty %s but got %s", wt.CallSite(2), desc, array)
	}
}

// Check that the peers slice matches the peers associated with the routers slice
func checkPeerArray(t *testing.T, peers []*Peer, routers []*Router) {
	check := make(map[PeerName]bool)
	for _, peer := range peers {
		check[peer.Name] = true
	}
	for _, router := range routers {
		name := router.Ourself.Peer.Name
		if _, found := check[name]; found {
			delete(check, name)
		} else {
			t.Fatalf("%s: Expected peer not found %s", wt.CallSite(2), name)
		}
	}
	if len(check) > 0 {
		t.Fatalf("%s: Unexpected peers: %v", wt.CallSite(2), check)
	}
}

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

type mockConnection struct {
	remote        *Peer
	remoteTCPAddr string // we are not currently checking the TCP address
}

func (conn *mockConnection) Local() *Peer          { return nil }
func (conn *mockConnection) Remote() *Peer         { return conn.remote }
func (conn *mockConnection) RemoteTCPAddr() string { return "" }
func (conn *mockConnection) Shutdown()             {}
func (conn *mockConnection) Established() bool     { return true }

// Wrappers for building arguments to test functions
func rs(routers ...*Router) []*Router { return routers }
func cs(routers ...*Router) []Connection {
	ret := make([]Connection, len(routers))
	for i, r := range routers {
		ret[i] = &mockConnection{r.Ourself.Peer, ""}
	}
	return ret
}
func ca(cslices ...[]Connection) [][]Connection { return cslices }

// Create a Peer object based on the name and UID of existing routers
func tp(r *Router, routers ...*Router) *Peer {
	peer := NewPeer(r.Ourself.Peer.Name, r.Ourself.Peer.UID, r.Ourself.Peer.version)
	for _, r2 := range routers {
		p2 := NewPeer(r2.Ourself.Peer.Name, r2.Ourself.Peer.UID, r2.Ourself.Peer.version)
		peer.connections[r2.Ourself.Peer.Name] = &mockConnection{p2, ""}
	}
	return peer
}

func TestGossipEncoding(t *testing.T) {
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

	// Create some peers that will talk to each other
	r1 := NewTestRouter(t, peer1Name, nil)
	r2 := NewTestRouter(t, peer2Name, nil)
	r3 := NewTestRouter(t, peer3Name, nil)

	// Check state when they have no connections
	checkEncoding(t, r1.Gossip(), rs(r1), ca(nil))
	checkEncoding(t, r2.Gossip(), rs(r2), ca(nil))

	// Now try adding some connections
	r1.AddTestConnection(r2)
	r2.AddTestConnection(r1)
	checkEncoding(t, r1.Gossip(), rs(r1, r2), ca(cs(r2), nil))
	checkEncoding(t, r2.Gossip(), rs(r1, r2), ca(nil, cs(r1)))
	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkEncoding(t, r2.Gossip(), rs(r1, r2, r3), ca(nil, cs(r1, r3), nil))
	checkEncoding(t, r3.Gossip(), rs(r3), ca(nil))
}

func checkEqualConns(t *testing.T, ourName PeerName, got, wanted map[PeerName]Connection) {
	checkConns := make(map[PeerName]bool)
	for _, conn := range wanted {
		checkConns[conn.Remote().Name] = true
	}
	for _, conn := range got {
		remoteName := conn.Remote().Name
		if _, found := checkConns[remoteName]; found {
			delete(checkConns, remoteName)
		} else {
			t.Fatalf("%s: Unexpected connection from %s to %s", wt.CallSite(5), ourName, remoteName)
		}
	}
	if len(checkConns) > 0 {
		t.Fatalf("%s: Expected connections not found: from %s to %v", wt.CallSite(3), ourName, checkConns)
	}
}

func checkTopology(t *testing.T, router *Router, wantedPeers ...*Peer) {
	check := make(map[PeerName]*Peer)
	for _, peer := range wantedPeers {
		check[peer.Name] = peer
	}
	for _, peer := range router.Peers.table {
		name := peer.Name
		if wantedPeer, found := check[name]; found {
			checkEqualConns(t, name, peer.connections, wantedPeer.connections)
			delete(check, name)
		} else {
			t.Fatalf("%s: Unexpected peer: %s", wt.CallSite(2), name)
		}
	}
	if len(check) > 0 {
		t.Fatalf("%s: Expected peers not found: %v", wt.CallSite(2), check)
	}
}

func TestGossipTopology(t *testing.T) {
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

	removed := &peerQueue{nil}

	// Create some peers that will talk to each other
	r1 := NewTestRouter(t, peer1Name, removed)
	r2 := NewTestRouter(t, peer2Name, removed)
	r3 := NewTestRouter(t, peer3Name, removed)

	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Check state when they have no connections
	checkTopology(t, r1, tp(r1))
	checkTopology(t, r2, tp(r2))

	// Now try adding some connections
	r1.AddTestConnection(r2)
	r2.AddTestConnection(r1)

	checkTopology(t, r1, tp(r1, r2), tp(r2))
	checkTopology(t, r2, tp(r2, r1), tp(r1))

	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkTopology(t, r2, tp(r1), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r3, tp(r3))
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Now r2 is going to gossip to r1
	newInfo1 := r1.OnGossip(r2.Gossip())
	// Check that r1 recognized the new info, and nothing else changed
	// 1 received an update from 2 that had an older version of 1, so 1 goes into the 'new info'
	checkEncoding(t, newInfo1, rs(r1, r2, r3), ca(cs(r2), cs(r1, r3), nil))
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r2, tp(r1), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r3, tp(r3))

	// r1 sends its new info to all connections, i.e. r2
	{
		newInfo1b := r2.OnGossip(newInfo1)
		checkEncoding(t, newInfo1b, rs(r1), ca(cs(r2)))
		checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3))

		// r2 sends its new info to all connections, i.e. r1
		newInfo1c := r1.OnGossip(newInfo1b)
		checkBlank(t, newInfo1c) // nothing new; stops here
	}

	// Now r2 gossips to r3, but 1 and 2 are unreachable from r3 so they get removed from the update
	{
		AssertEmpty(t, removed.peers, "garbage-collected peers")
		newInfo2 := r3.OnGossip(r2.Gossip())
		checkPeerArray(t, removed.peers, rs(r1, r2))
		checkBlank(t, newInfo2)
		checkTopology(t, r3, tp(r3))
		// r3 doesn't have any outgoing connections, so this doesn't go any further
	}
	removed.clear()

	// Add a connection from 3 to 1 and now r1 is reachable.
	r3.AddTestConnection(r1)
	r1.AddTestConnection(r3)
	// These two are going to gossip out their state; freeze it in variables
	r1Gossip := r1.Gossip()
	r3Gossip := r3.Gossip()
	newInfo3 := r3.OnGossip(r1Gossip)
	// 3 receives an update from 1 that has an older version of 3, so 3 goes into the 'new info'
	checkEncoding(t, newInfo3, rs(r1, r2, r3), ca(cs(r2, r3), cs(r1, r3), cs(r1)))
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Now the gossip from 3 to 1 that was 'simultaneous' with the one before
	newInfo4 := r1.OnGossip(r3Gossip)
	// r3 is newer and r1 is older so both go in the new items
	checkEncoding(t, newInfo4, rs(r1, r3), ca(cs(r2, r3), cs(r1)))
	checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))

	// Now 3 passes on its new info to 1, but there is nothing now new to 1
	checkBlank(t, r1.OnGossip(newInfo3))

	// Now 1 passes on the new info generated earlier to its connections 2 and 3
	{
		newInfo4b := r2.OnGossip(newInfo4)
		checkEncoding(t, newInfo4b, rs(r1, r3), ca(cs(r2, r3), cs(r1)))
		checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
		newInfo4c := r3.OnGossip(newInfo4)
		checkBlank(t, newInfo4c)
		checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
		// 2 now sends its 'new' info to its connected peers 1 and 3, but there is nothing new
		checkBlank(t, r1.OnGossip(newInfo4b))
		checkBlank(t, r3.OnGossip(newInfo4b))
	}

	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Drop the connection from 2 to 3
	r2.DeleteTestConnection(r3)
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	peersRemoved := r2.Peers.GarbageCollect()
	AssertEmpty(t, peersRemoved, "peers removed")
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Now r2 tells its connections
	{
		newInfo5 := r1.OnGossip(r2.Gossip())
		checkEncoding(t, newInfo5, rs(r2), ca(cs(r1)))
		checkBlank(t, r2.OnGossip(newInfo5))
		newInfo5b := r3.OnGossip(newInfo5)
		checkEncoding(t, newInfo5b, rs(r2), ca(cs(r1)))
		checkBlank(t, r1.OnGossip(newInfo5b))

		checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
		checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
		checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

		AssertEmpty(t, r1.Peers.GarbageCollect(), "peers removed")
		AssertEmpty(t, r2.Peers.GarbageCollect(), "peers removed")
		AssertEmpty(t, r3.Peers.GarbageCollect(), "peers removed")
		AssertEmpty(t, removed.peers, "garbage-collected peers")
	}

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	r1.DeleteTestConnection(r3)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1), tp(r3, r1))
	peersRemoved = r1.Peers.GarbageCollect()
	checkPeerArray(t, peersRemoved, rs(r3))
	checkPeerArray(t, removed.peers, rs(r3))
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	removed.clear()

	// Now r1 tells its remaining connection
	{
		newInfo6 := r2.OnGossip(r1.Gossip())
		checkEncoding(t, newInfo6, rs(r1), ca(cs(r2)))
		checkBlank(t, r1.OnGossip(newInfo6))
		checkPeerArray(t, removed.peers, rs(r3))
		removed.clear()
	}

	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	// On a timer, r3 will gossip to r1
	newInfo7 := r1.OnGossip(r3.Gossip())
	// 1 received an update that had an older version of 1
	checkEncoding(t, newInfo7, rs(r1), ca(cs(r2)))
	// r1 receives info about 3, but eliminates it through garbage collection
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkPeerArray(t, removed.peers, rs(r3))
	removed.clear()
}
