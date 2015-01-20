package router

import (
	"bytes"
	"fmt"
	"log"
	"sync"
)

type Routes struct {
	sync.RWMutex
	ourself   *Peer
	peers     *Peers
	unicast   map[PeerName]PeerName
	broadcast map[PeerName][]PeerName
	queryChan chan<- *Interaction
}

func NewRoutes(ourself *Peer, peers *Peers) *Routes {
	routes := &Routes{
		ourself:   ourself,
		peers:     peers,
		unicast:   make(map[PeerName]PeerName),
		broadcast: make(map[PeerName][]PeerName)}
	routes.unicast[ourself.Name] = UnknownPeerName
	routes.broadcast[ourself.Name] = []PeerName{}
	return routes
}

func (routes *Routes) Start() {
	queryChan := make(chan *Interaction, ChannelSize)
	routes.queryChan = queryChan
	go routes.queryLoop(queryChan)
}

func (routes *Routes) Unicast(name PeerName) (PeerName, bool) {
	routes.RLock()
	defer routes.RUnlock()
	hop, found := routes.unicast[name]
	return hop, found
}

func (routes *Routes) Broadcast(name PeerName) []PeerName {
	routes.RLock()
	defer routes.RUnlock()
	hops, found := routes.broadcast[name]
	if !found {
		return []PeerName{}
	}
	return hops
}

func (routes *Routes) String() string {
	var buf bytes.Buffer
	routes.RLock()
	defer routes.RUnlock()
	buf.WriteString(fmt.Sprintln("unicast:"))
	for name, hop := range routes.unicast {
		buf.WriteString(fmt.Sprintf("%s -> %s\n", name, hop))
	}
	buf.WriteString(fmt.Sprintln("broadcast:"))
	for name, hops := range routes.broadcast {
		buf.WriteString(fmt.Sprintf("%s -> %v\n", name, hops))
	}
	return buf.String()
}

// ACTOR client API

const (
	RRecalculate = iota
)

// Async.
func (routes *Routes) Recalculate() {
	routes.queryChan <- &Interaction{code: RRecalculate}
}

// ACTOR server

func (routes *Routes) queryLoop(queryChan <-chan *Interaction) {
	for {
		query, ok := <-queryChan
		if !ok {
			return
		}
		switch query.code {
		case RRecalculate:
			unicast := routes.calculateUnicast()
			broadcast := routes.calculateBroadcast()
			routes.Lock()
			routes.unicast = unicast
			routes.broadcast = broadcast
			routes.Unlock()
		default:
			log.Fatal("Unexpected routes query:", query)
		}
	}
}

// Calculate all the routes for the question: if *we* want to send a
// packet to Peer X, what is the next hop?
//
// When we sniff a packet, we determine the destination peer
// ourself. Consequently, we can relay the packet via any
// arbitrary peers - the intermediate peers do not have to have
// any knowledge of the MAC address at all. Thus there's no need
// to exchange knowledge of MAC addresses, nor any constraints on
// the routes that we construct.
func (routes *Routes) calculateUnicast() map[PeerName]PeerName {
	_, unicast := routes.ourself.Routes(nil, true)
	return unicast
}

// Calculate all the routes for the question: if we receive a
// broadcast originally from Peer X, which peers should we pass the
// frames on to?
//
// When the topology is stable, and thus all peers perform route
// calculations based on the same data, the algorithm ensures that
// broadcasts reach every peer exactly once.
//
// This is largely due to properties of the Peer.Routes algorithm. In
// particular:
//
// ForAll X,Y,Z in Peers.
//     X.Routes(Y) <= X.Routes(Z) \/
//     X.Routes(Z) <= X.Routes(Y)
// ForAll X,Y,Z in Peers.
//     Y =/= Z /\ X.Routes(Y) <= X.Routes(Z) =>
//     X.Routes(Y) u [P | Y.HasSymmetricConnectionTo(P)] <= X.Routes(Z)
// where <= is the subset relationship on keys of the returned map.
func (routes *Routes) calculateBroadcast() map[PeerName][]PeerName {
	broadcast := make(map[PeerName][]PeerName)
	ourself := routes.ourself

	routes.peers.ForEach(func(name PeerName, peer *Peer) {
		hops := []PeerName{}
		if found, reached := peer.Routes(ourself, true); found {
			ourself.ForEachConnection(func(remoteName PeerName, conn Connection) {
				if _, found := reached[remoteName]; found {
					return
				}
				if _, found := conn.Remote().ConnectionTo(ourself.Name); found {
					hops = append(hops, remoteName)
				}
			})
		}
		broadcast[name] = hops
	})
	return broadcast
}
