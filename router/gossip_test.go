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

func (r1 *Router) AddTestChannelConnection(r2 *Router) {
	toName := r2.Ourself.Peer.Name
	toPeer := NewPeer(toName, r2.Ourself.Peer.UID, 0)
	r1.Peers.FetchWithDefault(toPeer) // Has side-effect of incrementing refcount
	conn := &mockChannelConnection{RemoteConnection{r1.Ourself.Peer, toPeer, ""}, r2}
	r1.Ourself.addConnection(conn)
	r1.Ourself.handleConnectionEstablished(conn)
}

func TestGossipTopology(t *testing.T) {
	wt.RunWithTimeout(t, 1*time.Second, func() {
		implTestGossipTopology(t)
	})
}

// Create a remote Peer object plus all of its connections so we can check topology,
// based on the name and UIDs of existing routers
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
	checkTopology(t, r1, tp(r1))
	checkTopology(t, r2, tp(r2))

	// Now try adding some connections
	r1.AddTestChannelConnection(r2)
	checkTopology(t, r1, tp(r1, r2), tp(r2))
	checkTopology(t, r2, tp(r2))
	r2.AddTestChannelConnection(r1)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkTopology(t, r2, tp(r2, r1), tp(r1, r2))

	// Currently, the connection from 2 to 3 is one-way only
	r2.AddTestChannelConnection(r3)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3))
	// When r2 gossiped to r3, 1 and 2 were unreachable from r3 so they got removed from the update
	checkTopology(t, r3, tp(r3))

	// Add a connection from 3 to 1 and now r1 is reachable.
	r3.AddTestChannelConnection(r1)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r3, tp(r1), tp(r3, r1))

	r1.AddTestChannelConnection(r3)
	checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1, r3), tp(r3, r1))

	// Drop the connection from 2 to 3
	r2.Peers.DeleteTestConnection(r2.Ourself.Peer, r3.Ourself.Peer)
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	// Now r2 tells its connections
	r2.Ourself.broadcastPeerUpdate()
	checkTopology(t, r1, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	checkTopology(t, r2, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	// Drop the connection from 1 to 3, and it will get removed by garbage-collection
	r1.Peers.DeleteTestConnection(r1.Ourself.Peer, r3.Ourself.Peer)
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1), tp(r3, r1))
	r1.Peers.GarbageCollect()
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))

	// Now r1 tells its remaining connection, which also garbage-collects 3
	r1.Ourself.broadcastPeerUpdate()

	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
	checkTopology(t, r2, tp(r1, r2), tp(r2, r1))
	// r3 still thinks r1 has a connection to it
	checkTopology(t, r3, tp(r1, r2, r3), tp(r2, r1), tp(r3, r1))

	// On a timer, r3 will gossip to r1
	r3.SendAllGossip()
	// r1 receives info about 3, but eliminates it through garbage collection
	checkTopology(t, r1, tp(r1, r2), tp(r2, r1))
}
