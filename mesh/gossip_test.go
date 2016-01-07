package mesh

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO test gossip unicast; atm we only test topology gossip and
// surrogates, neither of which employ unicast.

type MockGossipConnection struct {
	RemoteConnection
	dest          *Router
	gossipSenders *GossipSenders
	start         chan struct{}
}

func NewTestRouter(name string) *Router {
	peerName, _ := PeerNameFromString(name)
	router := NewRouter(Config{}, peerName, "nick", nil)
	router.Start()
	return router
}

func (conn *MockGossipConnection) SendProtocolMsg(protocolMsg ProtocolMsg) error {
	<-conn.start
	return conn.dest.handleGossip(protocolMsg.tag, protocolMsg.msg)
}

func (conn *MockGossipConnection) GossipSenders() *GossipSenders {
	return conn.gossipSenders
}

func (conn *MockGossipConnection) Start() {
	close(conn.start)
}

func sendPendingGossip(routers ...*Router) {
	// Loop until all routers report they didn't send anything
	for sentSomething := true; sentSomething; {
		sentSomething = false
		for _, router := range routers {
			sentSomething = router.sendPendingGossip() || sentSomething
		}
	}
}

func AddTestGossipConnection(r1, r2 *Router) {
	c1 := r1.NewTestGossipConnection(r2)
	c2 := r2.NewTestGossipConnection(r1)
	c1.Start()
	c2.Start()
}

func (router *Router) NewTestGossipConnection(r *Router) *MockGossipConnection {
	to := r.Ourself.Peer
	toPeer := NewPeer(to.Name, to.NickName, to.UID, 0, to.ShortID)
	toPeer = router.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount

	conn := &MockGossipConnection{
		RemoteConnection: RemoteConnection{router.Ourself.Peer, toPeer, "", false, true},
		dest:             r,
		start:            make(chan struct{}),
	}
	conn.gossipSenders = NewGossipSenders(conn, make(chan struct{}))
	router.Ourself.handleAddConnection(conn)
	router.Ourself.handleConnectionEstablished(conn)
	return conn
}

func (router *Router) DeleteTestGossipConnection(r *Router) {
	toName := r.Ourself.Peer.Name
	conn, _ := router.Ourself.ConnectionTo(toName)
	router.Peers.Dereference(conn.Remote())
	router.Ourself.handleDeleteConnection(conn)
}

// Create a Peer representing the receiver router, with connections to
// the routers supplied as arguments, carrying across all UID and
// version information.
func (router *Router) tp(routers ...*Router) *Peer {
	peer := NewPeerFrom(router.Ourself.Peer)
	connections := make(map[PeerName]Connection)
	for _, r := range routers {
		p := NewPeerFrom(r.Ourself.Peer)
		connections[r.Ourself.Peer.Name] = newMockConnection(peer, p)
	}
	peer.Version = router.Ourself.Peer.Version
	peer.connections = connections
	return peer
}

// Check that the topology of router matches the peers and all of their connections
func checkTopology(t *testing.T, router *Router, wantedPeers ...*Peer) {
	router.Peers.RLock()
	checkTopologyPeers(t, true, router.Peers.allPeers(), wantedPeers...)
	router.Peers.RUnlock()
}

func flushAndCheckTopology(t *testing.T, routers []*Router, wantedPeers ...*Peer) {
	sendPendingGossip(routers...)
	for _, r := range routers {
		checkTopology(t, r, wantedPeers...)
	}
}

func TestGossipTopology(t *testing.T) {
	// Create some peers that will talk to each other
	r1 := NewTestRouter("01:00:00:01:00:00")
	r2 := NewTestRouter("02:00:00:02:00:00")
	r3 := NewTestRouter("03:00:00:03:00:00")
	routers := []*Router{r1, r2, r3}
	// Check state when they have no connections
	checkTopology(t, r1, r1.tp())
	checkTopology(t, r2, r2.tp())

	// Now try adding some connections
	AddTestGossipConnection(r1, r2)
	sendPendingGossip(r1, r2)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1))

	AddTestGossipConnection(r2, r3)
	flushAndCheckTopology(t, routers, r1.tp(r2), r2.tp(r1, r3), r3.tp(r2))

	AddTestGossipConnection(r3, r1)
	flushAndCheckTopology(t, routers, r1.tp(r2, r3), r2.tp(r1, r3), r3.tp(r1, r2))

	// Drop the connection from 2 to 3
	r2.DeleteTestGossipConnection(r3)
	flushAndCheckTopology(t, routers, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1, r2))

	// Drop the connection from 1 to 3
	r1.DeleteTestGossipConnection(r3)
	sendPendingGossip(r1, r2, r3)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1, r2))
}

func TestGossipSurrogate(t *testing.T) {
	// create the topology r1 <-> r2 <-> r3
	r1 := NewTestRouter("01:00:00:01:00:00")
	r2 := NewTestRouter("02:00:00:02:00:00")
	r3 := NewTestRouter("03:00:00:03:00:00")
	routers := []*Router{r1, r2, r3}
	AddTestGossipConnection(r1, r2)
	AddTestGossipConnection(r3, r2)
	flushAndCheckTopology(t, routers, r1.tp(r2), r2.tp(r1, r3), r3.tp(r2))

	// create a gossiper at either end, but not the middle
	g1 := newTestGossiper()
	g3 := newTestGossiper()
	s1 := r1.NewGossip("Test", g1)
	s3 := r3.NewGossip("Test", g3)

	// broadcast a message from each end, check it reaches the other
	broadcast(s1, 1)
	broadcast(s3, 2)
	sendPendingGossip(r1, r2, r3)
	g1.checkHas(t, 2)
	g3.checkHas(t, 1)

	// check that each end gets their message back through periodic
	// gossip
	r1.SendAllGossip()
	r3.SendAllGossip()
	sendPendingGossip(r1, r2, r3)
	g1.checkHas(t, 1, 2)
	g3.checkHas(t, 1, 2)
}

type testGossiper struct {
	sync.RWMutex
	state map[byte]struct{}
}

func newTestGossiper() *testGossiper {
	return &testGossiper{state: make(map[byte]struct{})}
}

func (g *testGossiper) OnGossipUnicast(sender PeerName, msg []byte) error {
	return nil
}

func (g *testGossiper) OnGossipBroadcast(_ PeerName, update []byte) (GossipData, error) {
	g.Lock()
	defer g.Unlock()
	for _, v := range update {
		g.state[v] = void
	}
	return NewSurrogateGossipData(update), nil
}

func (g *testGossiper) Gossip() GossipData {
	g.RLock()
	defer g.RUnlock()
	state := make([]byte, len(g.state))
	for v := range g.state {
		state = append(state, v)
	}
	return NewSurrogateGossipData(state)
}

func (g *testGossiper) OnGossip(update []byte) (GossipData, error) {
	g.Lock()
	defer g.Unlock()
	var delta []byte
	for _, v := range update {
		if _, found := g.state[v]; !found {
			delta = append(delta, v)
			g.state[v] = void
		}
	}
	if len(delta) == 0 {
		return nil, nil
	}
	return NewSurrogateGossipData(delta), nil
}

func (g *testGossiper) checkHas(t *testing.T, vs ...byte) {
	g.RLock()
	defer g.RUnlock()
	for _, v := range vs {
		if _, found := g.state[v]; !found {
			require.FailNow(t, fmt.Sprintf("%d is missing", v))
		}
	}
}

func broadcast(s Gossip, v byte) {
	s.GossipBroadcast(NewSurrogateGossipData([]byte{v}))
}
