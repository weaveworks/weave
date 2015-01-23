// No mocks are tested by this file.
// It supplies some mock implementations to other unit tests,
// and is named "...test.go" so it is only compiled under `go test`.

package router

import (
	wt "github.com/zettio/weave/testing"
	"testing"
)

// Construct a Router object with a mock interface to check peer
// garbage-collection, and without firing up any ancilliary goroutines
func NewTestRouter(name PeerName) *Router {
	router := NewRouter(nil, name, nil, 10, 1024, nil)
	// Create dummy channels otherwise tests hang on nil channel
	router.ConnectionMaker.queryChan = make(chan *ConnectionMakerInteraction, ChannelSize)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

func (peers *Peers) AddTestConnection(ourself *LocalPeer, peer *Peer) {
	toName := peer.Name
	toPeer := NewPeer(toName, peer.UID, 0)
	peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	conn := newMockConnection(ourself.Peer, toPeer)
	ourself.addConnection(conn)
	ourself.connectionEstablished(conn)
}

func (peers *Peers) AddTestRemoteConnection(p0 *LocalPeer, p1, p2 *Peer) {
	fromName := p1.Name
	fromPeer := NewPeer(fromName, p1.UID, 0)
	fromPeer = peers.FetchWithDefault(fromPeer)
	toName := p2.Name
	toPeer := NewPeer(toName, p2.UID, 0)
	toPeer = peers.FetchWithDefault(toPeer)
	p0.addConnection(&RemoteConnection{fromPeer, toPeer, ""})
}

func (r1 *Router) DeleteTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer, _ := r1.Peers.Fetch(toName)
	toPeer.DecrementLocalRefCount()
	conn, _ := r1.Ourself.Peer.ConnectionTo(toName)
	r1.Ourself.deleteConnection(conn)
}

// mockConnection used in testing is very similar to a RemoteConnection, without
// the RemoteTCPAddr(), but I want to keep a separate type in order to distinguish
// what is created by the test from what is created by the real code.
func newMockConnection(from, to *Peer) Connection {
	type mockConnection struct{ RemoteConnection }
	return &mockConnection{RemoteConnection{from, to, ""}}
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
			wt.Fatalf(t, "Unexpected connection from %s to %s", ourName, remoteName)
		}
	}
	if len(checkConns) > 0 {
		wt.Fatalf(t, "Expected connections not found: from %s to %v", ourName, checkConns)
	}
}

// Get all the peers from a Peers in a slice
func (peers *Peers) allPeers() []*Peer {
	res := make([]*Peer, 0)
	for _, peer := range peers.table {
		res = append(res, peer)
	}
	return res
}

func (peers *Peers) allPeersExcept(excludeName PeerName) []*Peer {
	res := peers.allPeers()
	for i, peer := range res {
		if peer.Name == excludeName {
			return append(res[:i], res[i+1:]...)
		}
	}
	return res
}

// Check that the peers slice matches the wanted peers
func checkPeerArray(t *testing.T, peers []*Peer, wantedPeers ...*Peer) {
	checkTopologyPeers(t, false, peers, wantedPeers...)
}

// Check that the topology of router matches the peers and all of their connections
func checkTopology(t *testing.T, router *Router, wantedPeers ...*Peer) {
	checkTopologyPeers(t, true, router.Peers.allPeers(), wantedPeers...)
}

// Check that the peers slice matches the wanted peers and optionally all of their connections
func checkTopologyPeers(t *testing.T, checkConns bool, peers []*Peer, wantedPeers ...*Peer) {
	check := make(map[PeerName]*Peer)
	for _, peer := range wantedPeers {
		check[peer.Name] = peer
	}
	for _, peer := range peers {
		name := peer.Name
		if wantedPeer, found := check[name]; found {
			if checkConns {
				checkEqualConns(t, name, peer.connections, wantedPeer.connections)
			}
			delete(check, name)
		} else {
			wt.Fatalf(t, "Unexpected peer: %s", name)
		}
	}
	if len(check) > 0 {
		wt.Fatalf(t, "Expected peers not found: %v", check)
	}
}

// Create a remote Peer object plus all of its connections, based on the name and UIDs of existing routers
func tp(r *Router, routers ...*Router) *Peer {
	peer := NewPeer(r.Ourself.Peer.Name, r.Ourself.Peer.UID, 0)
	connections := make(map[PeerName]Connection)
	for _, r2 := range routers {
		p2 := NewPeer(r2.Ourself.Peer.Name, r2.Ourself.Peer.UID, r2.Ourself.Peer.version)
		connections[r2.Ourself.Peer.Name] = newMockConnection(peer, p2)
	}
	peer.SetVersionAndConnections(r.Ourself.Peer.version, connections)
	return peer
}
