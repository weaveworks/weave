package router

import (
	"bytes"
	"fmt"
	"log"
)

func (topo *Topology) Unicast(name PeerName) (PeerName, bool) {
	topo.RLock()
	defer topo.RUnlock()
	hop, found := topo.unicast[name]
	return hop, found
}

func (topo *Topology) Broadcast(name PeerName) []PeerName {
	topo.RLock()
	defer topo.RUnlock()
	hops, found := topo.broadcast[name]
	if !found {
		return []PeerName{}
	}
	return hops
}

func (topo *Topology) String() string {
	var buf bytes.Buffer
	topo.RLock()
	defer topo.RUnlock()
	buf.WriteString(fmt.Sprintln("unicast:"))
	for name, hop := range topo.unicast {
		buf.WriteString(fmt.Sprintf("%s -> %s\n", name, hop))
	}
	buf.WriteString(fmt.Sprintln("broadcast:"))
	for name, hops := range topo.broadcast {
		buf.WriteString(fmt.Sprintf("%s -> %v\n", name, hops))
	}
	return buf.String()
}

func StartTopology(router *Router) *Topology {
	queryChan := make(chan *Interaction, ChannelSize)
	state := &Topology{
		router:    router,
		queryChan: queryChan,
		unicast:   make(map[PeerName]PeerName),
		broadcast: make(map[PeerName][]PeerName)}
	state.unicast[router.Ourself.Name] = UnknownPeerName
	state.broadcast[router.Ourself.Name] = []PeerName{}
	go state.queryLoop(queryChan)
	return state
}

// ACTOR client API

const (
	TFetchAll      = iota
	TRebuildRoutes = iota
)

func (topo *Topology) FetchAll() []byte {
	resultChan := make(chan interface{}, 0)
	topo.queryChan <- &Interaction{
		code:       TFetchAll,
		resultChan: resultChan}
	result := <-resultChan
	return result.([]byte)
}

// Async.
func (topo *Topology) RebuildRoutes() {
	topo.queryChan <- &Interaction{code: TRebuildRoutes}
}

// ACTOR server

func (topo *Topology) queryLoop(queryChan <-chan *Interaction) {
	for {
		query, ok := <-queryChan
		if !ok {
			return
		}
		switch query.code {
		case TRebuildRoutes:
			unicast := topo.buildUnicastRoutes()
			broadcast := topo.buildBroadcastRoutes()
			topo.Lock()
			topo.unicast = unicast
			topo.broadcast = broadcast
			topo.Unlock()
		case TFetchAll:
			query.resultChan <- topo.router.Peers.EncodeAllPeers()
		default:
			log.Fatal("Unexpected topology query:", query)
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
func (topo *Topology) buildUnicastRoutes() map[PeerName]PeerName {
	_, routes := topo.router.Ourself.Routes(nil, true)
	return routes
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
func (topo *Topology) buildBroadcastRoutes() map[PeerName][]PeerName {
	broadcast := make(map[PeerName][]PeerName)
	ourself := topo.router.Ourself

	topo.router.Peers.ForEach(func(name PeerName, peer *Peer) {
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
