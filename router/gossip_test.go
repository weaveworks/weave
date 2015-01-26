package router

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	wt "github.com/zettio/weave/testing"
	"testing"
	"time"
)

type mockChannelConnection struct {
	RemoteConnection
	dest *Router
}

// Construct a Router object with dummy channels otherwise tests hang on nil channel
// when Router.OnGossip() calls async methods
func NewTestRouter(name PeerName) *Router {
	router := NewRouter(nil, name, nil, 10, 1024, nil)
	router.ConnectionMaker.queryChan = make(chan *ConnectionMakerInteraction, ChannelSize)
	router.Routes.queryChan = make(chan *Interaction, ChannelSize)
	return router
}

// This is basically the same as LocalConnection.handleGossip()
func (conn *mockChannelConnection) SendProtocolMsg(msg ProtocolMsg) {
	decoder := gob.NewDecoder(bytes.NewReader(msg.msg))
	var channelHash uint32
	if err := decoder.Decode(&channelHash); err != nil {
		panic(errors.New(fmt.Sprintf("error when decoding: %s", err)))
	} else if channel, found := conn.dest.GossipChannels[channelHash]; !found {
		panic(errors.New(fmt.Sprintf("unknown channel: %d", channelHash)))
	} else {
		var srcName PeerName
		if err := decoder.Decode(&srcName); err != nil {
			panic(err)
		}
		deliverGossip(channel, srcName, msg.msg, decoder)
	}
}

func (router *Router) AddTestChannelConnection(r *Router) {
	toName := r.Ourself.Peer.Name
	toPeer := NewPeer(toName, r.Ourself.Peer.UID, 0)
	router.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	fromPeer := NewPeer(router.Ourself.Peer.Name, router.Ourself.Peer.UID, 0)
	r.Peers.FetchWithDefault(fromPeer)
	conn := &mockChannelConnection{RemoteConnection{router.Ourself.Peer, toPeer, ""}, r}
	router.Ourself.addConnection(conn)
	router.Ourself.handleConnectionEstablished(conn)
}

func (router *Router) DeleteTestChannelConnection(peers2 *Peers) {
	toName := peers2.ourself.Name
	toPeer, _ := router.Peers.Fetch(toName)
	toPeer.DecrementLocalRefCount()
	conn, _ := router.Ourself.ConnectionTo(toName)
	router.Ourself.handleDeleteConnection(conn)
	fromPeer, _ := peers2.Fetch(router.Ourself.Name)
	fromPeer.DecrementLocalRefCount()
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
	peer := NewPeer(router.Ourself.Peer.Name, router.Ourself.Peer.UID, 0)
	connections := make(map[PeerName]Connection)
	for _, r := range routers {
		p := NewPeer(r.Ourself.Peer.Name, r.Ourself.Peer.UID, r.Ourself.Peer.version)
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
	r1 := NewTestRouter(peer1Name)
	r2 := NewTestRouter(peer2Name)
	r3 := NewTestRouter(peer3Name)
	r1.NewGossip(TopologyGossipCh, r1)
	r2.NewGossip(TopologyGossipCh, r2)
	r3.NewGossip(TopologyGossipCh, r3)

	// Check state when they have no connections
	checkTopology(t, r1, r1.tp())
	checkTopology(t, r2, r2.tp())

	// Now try adding some connections
	r1.AddTestChannelConnection(r2)
	checkTopology(t, r1, r1.tp(r2), r2.tp())
	checkTopology(t, r2, r1.tp(r2), r2.tp())
	r2.AddTestChannelConnection(r1)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1))
	checkTopology(t, r2, r2.tp(r1), r1.tp(r2))

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
	r2.DeleteTestChannelConnection(r3.Peers)
	checkTopology(t, r1, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2, r3), r2.tp(r1))
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))

	// Drop the connection from 1 to 3
	r1.DeleteTestChannelConnection(r3.Peers)
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))

	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))
	checkTopology(t, r2, r1.tp(r2), r2.tp(r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, r1.tp(r2, r3), r2.tp(r1), r3.tp(r1))

	// On a timer, r3 will gossip to r1
	r3.SendAllGossip()
	checkTopology(t, r1, r1.tp(r2), r2.tp(r1), r3.tp(r1))
}
