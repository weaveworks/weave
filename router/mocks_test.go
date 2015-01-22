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
func NewTestRouter(t *testing.T, name PeerName) *Router {
	router := NewRouter(nil, name, nil, 10, 1024, nil)
	// Create dummy channels otherwise tests hang on nil channel
	router.ConnectionMaker.queryChan = make(chan *ConnectionMakerInteraction, ChannelSize)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

func (r1 *Router) AddTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	r1.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	conn := newMockConnection(r1.Ourself.Peer, toPeer)
	r1.Ourself.addConnection(conn)
	r1.Ourself.connectionEstablished(conn)
}

func (r0 *Router) AddTestRemoteConnection(r1, r2 *Router) {
	fromName := r2.Ourself.Peer.Name
	fromPeer := NewPeer(fromName, r1.Ourself.Peer.UID, 0)
	fromPeer = r0.Peers.FetchWithDefault(fromPeer)
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	toPeer = r0.Peers.FetchWithDefault(toPeer)
	r0.Ourself.addConnection(&RemoteConnection{fromPeer, toPeer, ""})
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
			wt.Fatalf(t, "Expected peer not found %s", name)
		}
	}
	if len(check) > 0 {
		wt.Fatalf(t, "Unexpected peers: %v", check)
	}
}

// Wrappers for building arguments to test functions
func rs(routers ...*Router) []*Router { return routers }
func cs(routers ...*Router) []Connection {
	ret := make([]Connection, len(routers))
	for i, r := range routers {
		ret[i] = newMockConnection(nil, r.Ourself.Peer)
	}
	return ret
}
func ca(cslices ...[]Connection) [][]Connection { return cslices }
