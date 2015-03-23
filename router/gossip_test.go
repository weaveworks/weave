package router

import (
	wt "github.com/zettio/weave/testing"
	"testing"
	"time"
)

// TODO test gossip unicast and broadcast; atm we only test topology
// gossip, which does not employ unicast or broadcast.

type mockChannelConnection struct {
	RemoteConnection
	dest *Router
}

// Construct a "passive" Router, i.e. without any goroutines.
//
// We need to create some dummy channels otherwise tests hang on nil
// channels when Router.OnGossip() calls async methods.
func NewTestRouter(name PeerName) *Router {
	router := NewRouter(nil, name, "", nil, 10, 1024, nil)
	router.ConnectionMaker.queryChan = make(chan *ConnectionMakerInteraction, ChannelSize)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

func (conn *mockChannelConnection) SendProtocolMsg(protocolMsg ProtocolMsg) {
	if err := conn.dest.handleGossip(protocolMsg.msg, deliverGossip); err != nil {
		panic(err)
	}
	conn.dest.sendPendingGossip()
}

// FIXME this doesn't actually guarantee everything has been sent
// since a GossipSender may be in the process of sending and there is
// no easy way for us to know when that has completed.
func (router *Router) sendPendingGossip() {
	for _, channel := range router.GossipChannels {
		for _, sender := range channel.senders {
			sender.flush()
		}
	}
}

func (sender *GossipSender) flush() {
	for {
		select {
		case pending := <-sender.cell:
			sender.sendPending(pending)
		default:
			return
		}
	}
}

func (router *Router) AddTestChannelConnection(r *Router) {
	fromName := router.Ourself.Peer.Name
	toName := r.Ourself.Peer.Name

	fromPeer := NewPeer(fromName, "", router.Ourself.Peer.UID, 0)
	toPeer := NewPeer(toName, "", r.Ourself.Peer.UID, 0)

	r.Peers.FetchWithDefault(fromPeer)    // Has side-effect of incrementing refcount
	router.Peers.FetchWithDefault(toPeer) //

	conn := &mockChannelConnection{RemoteConnection{router.Ourself.Peer, toPeer, "", false, true}, r}
	router.Ourself.handleAddConnection(conn)
	router.Ourself.handleConnectionEstablished(conn)
	router.sendPendingGossip()
}

func (router *Router) DeleteTestChannelConnection(r *Router) {
	fromName := router.Ourself.Peer.Name
	toName := r.Ourself.Peer.Name

	fromPeer, _ := r.Peers.Fetch(fromName)
	toPeer, _ := router.Peers.Fetch(toName)

	fromPeer.DecrementLocalRefCount()
	toPeer.DecrementLocalRefCount()

	conn, _ := router.Ourself.ConnectionTo(toName)
	router.Ourself.handleDeleteConnection(conn)
	router.sendPendingGossip()
}

func TestGossipTopology(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() {
		implTestGossipTopology(t)
	})
}

// Create a Peer representing the receiver router, with connections to
// the routers supplied as arguments, carrying across all UID and
// version information.
func (router *Router) tp(routers ...*Router) *Peer {
	peer := NewPeer(router.Ourself.Peer.Name, "", router.Ourself.Peer.UID, 0)
	connections := make(map[PeerName]Connection)
	for _, r := range routers {
		p := NewPeer(r.Ourself.Peer.Name, "", r.Ourself.Peer.UID, r.Ourself.Peer.version)
		connections[r.Ourself.Peer.Name] = newMockConnection(peer, p)
	}
	peer.SetVersionAndConnections(router.Ourself.Peer.version, connections)
	return peer
}

// Check that the topology of router matches the peers and all of their connections
func checkTopology(t *testing.T, router *Router, wantedPeers ...*Peer) {
	checkTopologyPeers(t, true, router.Peers.allPeers(), wantedPeers...)
}

func implTestGossipTopology(t *testing.T) {
	// Create some peers that will talk to each other
	peer1Name, _ := PeerNameFromString("01:00:00:01:00:00")
	peer2Name, _ := PeerNameFromString("02:00:00:02:00:00")
	peer3Name, _ := PeerNameFromString("03:00:00:03:00:00")
	r1 := NewTestRouter(peer1Name)
	r2 := NewTestRouter(peer2Name)
	r3 := NewTestRouter(peer3Name)

	// Check state when they have no connections
	checkTopology(t, r1, r1.tp())
	checkTopology(t, r2, r2.tp())

	// Now try adding some connections
	r1.AddTestChannelConnection(r2)
	checkTopology(t, r1, r1.tp(r2), r2.tp())
	checkTopology(t, r2, r1.tp(r2), r2.tp())
	r2.AddTestChannelConnection(r1)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1))

	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestChannelConnection(r3)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1, r3), r3.tp())
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1, r3), r3.tp())
	// When r2 gossiped to r3, 1 was unreachable from r3 so it got removed from the
	// list of peers, but remains referenced in the connection from 1 to 3.
	checkTopology(t, r3, r2.tp(r1, r3), r3.tp())

	// Add a connection from 3 to 1 and now r1 is reachable.
	r3.AddTestChannelConnection(r1)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1, r3), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1, r3), r3.tp(r1))
	checkTopology(t, r3, r1.tp(), r2.tp(r1, r3), r3.tp(r1))

	r1.AddTestChannelConnection(r3)
	checkTopology(t, r1, r1.tp(r2, r3), r2.tp(r1, r3), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2, r3), r2.tp(r1, r3), r3.tp(r1))
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1, r3), r3.tp(r1))

	// Drop the connection from 2 to 3
	r2.DeleteTestChannelConnection(r3)
	checkTopology(t, r1, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2, r3), r2.tp(r1))
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))

	// Drop the connection from 1 to 3
	r1.DeleteTestChannelConnection(r3)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))

	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))

	// On a timer, r3 will gossip to r1
	r3.SendAllGossip()
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))
}
