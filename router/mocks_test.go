package router

import (
	wt "github.com/zettio/weave/testing"
	"net"
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
		if queue != nil {
			queue.peers = append(queue.peers, peer)
		}
	}
	router := NewRouter(nil, name, nil, 10, 1024, nil)
	router.Macs.onExpiry = onMacExpiry
	router.Peers.onGC = onPeerGC
	// Create dummy channels otherwise tests hang on nil channel
	router.ConnectionMaker.queryChan = make(chan *ConnectionMakerInteraction, ChannelSize)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

func (r1 *Router) AddTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	r1.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	r1.Ourself.Peer.connections[toName] = &mockConnection{toPeer, ""}
	r1.Ourself.Peer.version += 1
}

func (r0 *Router) AddTestRemoteConnection(r1, r2 *Router) {
	fromName := r2.Ourself.Peer.Name
	fromPeer := NewPeer(fromName, r1.Ourself.Peer.UID, 0)
	fromPeer = r0.Peers.FetchWithDefault(fromPeer)
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	toPeer = r0.Peers.FetchWithDefault(toPeer)
	r0.Ourself.Peer.connections[toName] = &RemoteConnection{fromPeer, toPeer, ""}
	r0.Ourself.Peer.version += 1
}

func (r1 *Router) DeleteTestConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer, _ := r1.Peers.Fetch(toName)
	toPeer.DecrementLocalRefCount()
	delete(r1.Ourself.Peer.connections, toName)
	r1.Ourself.Peer.version += 1
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
