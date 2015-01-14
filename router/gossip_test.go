package router

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	wt "github.com/zettio/weave/testing"
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
	r1.Ourself.Peer.connections[toName] = &mockChannelConnection{mockConnection{toPeer, ""}, r2}
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
func (conn *mockConnection) Shutdown(error)        {}
func (conn *mockConnection) Established() bool     { return true }

type mockChannelConnection struct {
	mockConnection
	dest *Router
}

func (conn *mockChannelConnection) SendTCP(msg []byte) {
	channelHash, payload := decodeGossipChannel(msg[1:])
	if channel, found := conn.dest.GossipChannels[channelHash]; !found {
		panic(errors.New(fmt.Sprintf("unknown channel: %d", channelHash)))
	} else {
		srcName, payload := decodePeerName(payload)
		deliverGossip(channel, srcName, msg, payload)
	}
}

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
			t.Fatalf("%s: Unexpected connection from %s to %s", wt.CallSite(3), ourName, remoteName)
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
	r1.NewGossip("topology", r1)
	r2.NewGossip("topology", r2)
	r3.NewGossip("topology", r3)

	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Check state when they have no connections
	checkTopology(t, r1, tp(r1))
	checkTopology(t, r2, tp(r2))

	// Now try adding some connections
	r1.AddTestConnection(r2)
	r2.AddTestConnection(r1)

	checkTopology(t, r1, tp(r1, r2), tp(r2))
	checkTopology(t, r2, tp(r2, r1), tp(r1))

	r1.SendAllGossip()

	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkTopology(t, r2, tp(r2, r1), tp(r1, r2))

	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestConnection(r3)
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r3, tp(r3))
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Now r2 is going to gossip to all its peers
	r2.SendAllGossip()
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r3, tp(r3))
	// When r2 gossiped to r3, 1 and 2 were unreachable from r3 so they got removed from the update
	checkPeerArray(t, removed.peers, rs(r1, r2))
	removed.clear()

	// Add a connection from 3 to 1 and now r1 is reachable.
	r3.AddTestConnection(r1)
	r3.SendAllGossip()
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r3, tp(r1), tp(r3, r1))
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	r1.AddTestConnection(r3)
	r1.SendAllGossip()
	checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Drop the connection from 2 to 3
	r2.DeleteTestConnection(r3)
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	peersRemoved := r2.Peers.GarbageCollect()
	AssertEmpty(t, peersRemoved, "peers removed")
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Now r2 tells its connections
	r2.SendAllGossip()
	checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	AssertEmpty(t, r1.Peers.GarbageCollect(), "peers removed")
	AssertEmpty(t, r2.Peers.GarbageCollect(), "peers removed")
	AssertEmpty(t, r3.Peers.GarbageCollect(), "peers removed")
	AssertEmpty(t, removed.peers, "garbage-collected peers")

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	r1.DeleteTestConnection(r3)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1), tp(r3, r1))
	peersRemoved = r1.Peers.GarbageCollect()
	checkPeerArray(t, peersRemoved, rs(r3))
	checkPeerArray(t, removed.peers, rs(r3))
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	removed.clear()

	// Now r1 tells its remaining connection
	r1.SendAllGossip()
	checkPeerArray(t, removed.peers, rs(r3))
	removed.clear()

	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	// On a timer, r3 will gossip to r1
	r3.SendAllGossip()
	// r1 receives info about 3, but eliminates it through garbage collection
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkPeerArray(t, removed.peers, rs(r3))
	removed.clear()
}
