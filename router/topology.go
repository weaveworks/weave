package router

import (
	"bytes"
	"fmt"
	"log"
	"sort"
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

func (topo *Topology) buildUnicastRoutes() map[PeerName]PeerName {
	// He we are calculating all the routes for the question: if *we*
	// want to send a packet to Peer X, what is the next hop?
	//
	// When we sniff a packet, we determine the destination peer
	// ourself. Consequently, we can relay the packet via any
	// arbitrary peers - the intermediate peers do not have to have
	// any knowledge of the MAC address at all. Thus there's no need
	// to exchange knowledge of MAC addresses, nor any constraints on
	// the routes that we construct.
	//
	// Because currently we do not have weightings on the connections
	// between peers, there is no need to use Dijkstra's
	// algorithm. Instead, we can use the simpler and cheaper
	// breadth-first widening.
	//
	// Currently we make no attempt to perform any balancing of any
	// sort. That could happen in the future.

	unicast := make(map[PeerName]PeerName)
	ourself := topo.router.Ourself
	unicast[ourself.Name] = UnknownPeerName
	nextWorklist := []*Peer{}
	ourself.ForEachConnection(func(remoteName PeerName, conn Connection) {
		unicast[remoteName] = remoteName
		nextWorklist = append(nextWorklist, conn.Remote())
	})
	for len(nextWorklist) > 0 {
		worklist := nextWorklist
		sort.Sort(ListOfPeers(worklist))
		nextWorklist = []*Peer{}
		for _, peer := range worklist {
			localName := peer.Name
			peer.ForEachConnection(func(remoteName PeerName, conn Connection) {
				if _, found := unicast[remoteName]; found {
					return
				}
				// We now know how to get to remoteName: the same
				// way we get to peer. However, we only record this as
				// a valid path if remoteName has peer in its
				// connections. I.e. it must be a symmetric
				// connection.
				if _, found := conn.Remote().ConnectionTo(localName); found {
					unicast[remoteName] = unicast[localName]
					nextWorklist = append(nextWorklist, conn.Remote())
				}
			})
		}
	}
	return unicast
}

func (topo *Topology) buildBroadcastRoutes() map[PeerName][]PeerName {
	// For broadcast, we need to construct which of our connected
	// peers we should pass frames on to should we receive any
	// broadcast originally from peer X
	broadcast := make(map[PeerName][]PeerName)
	ourself := topo.router.Ourself

	topo.router.Peers.ForEach(func(name PeerName, peer *Peer) {
		hops := []PeerName{}
		found, reached := peer.HasPathTo(ourself, true)
		if found {
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
