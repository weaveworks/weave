package router

import (
	"math"
	"sync"
)

type unicastRoutes map[PeerName]PeerName
type broadcastRoutes map[PeerName][]PeerName

type Routes struct {
	sync.RWMutex
	ourself      *LocalPeer
	peers        *Peers
	onChange     func()
	unicast      unicastRoutes
	unicastAll   unicastRoutes // [1]
	broadcast    broadcastRoutes
	broadcastAll broadcastRoutes // [1]
	recalculate  chan<- *struct{}
	wait         chan<- chan struct{}
	// [1] based on *all* connections, not just established &
	// symmetric ones
}

func NewRoutes(ourself *LocalPeer, peers *Peers, onChange func()) *Routes {
	recalculate := make(chan *struct{}, 1)
	wait := make(chan chan struct{})
	routes := &Routes{
		ourself:      ourself,
		peers:        peers,
		onChange:     onChange,
		unicast:      make(unicastRoutes),
		unicastAll:   make(unicastRoutes),
		broadcast:    make(broadcastRoutes),
		broadcastAll: make(broadcastRoutes),
		recalculate:  recalculate,
		wait:         wait}
	routes.unicast[ourself.Name] = UnknownPeerName
	routes.unicastAll[ourself.Name] = UnknownPeerName
	routes.broadcast[ourself.Name] = []PeerName{}
	routes.broadcastAll[ourself.Name] = []PeerName{}
	go routes.run(recalculate, wait)
	return routes
}

func (routes *Routes) PeerNames() PeerNameSet {
	return routes.peers.Names()
}

func (routes *Routes) Unicast(name PeerName) (PeerName, bool) {
	routes.RLock()
	defer routes.RUnlock()
	hop, found := routes.unicast[name]
	return hop, found
}

func (routes *Routes) UnicastAll(name PeerName) (PeerName, bool) {
	routes.RLock()
	defer routes.RUnlock()
	hop, found := routes.unicastAll[name]
	return hop, found
}

func (routes *Routes) Broadcast(name PeerName) []PeerName {
	routes.RLock()
	defer routes.RUnlock()
	return routes.broadcast[name]
}

func (routes *Routes) BroadcastAll(name PeerName) []PeerName {
	routes.RLock()
	defer routes.RUnlock()
	return routes.broadcastAll[name]
}

// Choose min(log2(n_peers), n_neighbouring_peers) neighbours, with a
// random distribution that is topology-sensitive, favouring
// neighbours at the end of "bottleneck links". We determine the
// latter based on the unicast routing table. If a neighbour appears
// as the value more frequently than others - meaning that we reach a
// higher proportion of peers via that neighbour than other neighbours
// - then it is chosen with a higher probability.
//
// Note that we choose log2(n_peers) *neighbours*, not
// peers. Consequently, on sparsely connected peers this function
// returns a higher proportion of neighbours than elsewhere. In
// extremis, on peers with fewer than log2(n_peers) neighbours, all
// neighbours are returned.
func (routes *Routes) RandomNeighbours(except PeerName) PeerNameSet {
	res := make(PeerNameSet)
	routes.RLock()
	defer routes.RUnlock()
	count := int(math.Log2(float64(len(routes.unicastAll))))
	// depends on go's random map iteration
	for _, dst := range routes.unicastAll {
		if dst != UnknownPeerName && dst != except {
			res[dst] = void
			if len(res) >= count {
				break
			}
		}
	}
	return res
}

// Request recalculation of the routing table. This is async but can
// effectively be made synchronous with a subsequent call to
// EnsureRecalculated.
func (routes *Routes) Recalculate() {
	// The use of a 1-capacity channel in combination with the
	// non-blocking send is an optimisation that results in multiple
	// requests being coalesced.
	select {
	case routes.recalculate <- nil:
	default:
	}
}

// Wait for any preceding Recalculate requests to be processed.
func (routes *Routes) EnsureRecalculated() {
	done := make(chan struct{})
	routes.wait <- done
	<-done
}

func (routes *Routes) run(recalculate <-chan *struct{}, wait <-chan chan struct{}) {
	for {
		select {
		case <-recalculate:
			routes.calculate()
		case done := <-wait:
			select {
			case <-recalculate:
				routes.calculate()
			default:
			}
			close(done)
		}
	}
}

func (routes *Routes) calculate() {
	routes.peers.RLock()
	routes.ourself.RLock()
	var (
		oldUnicast   = routes.unicast
		oldBroadcast = routes.broadcast
		unicast      = routes.calculateUnicast(true)
		unicastAll   = routes.calculateUnicast(false)
		broadcast    = routes.calculateBroadcast(true)
		broadcastAll = routes.calculateBroadcast(false)
	)
	routes.ourself.RUnlock()
	routes.peers.RUnlock()

	routes.Lock()
	routes.unicast = unicast
	routes.unicastAll = unicastAll
	routes.broadcast = broadcast
	routes.broadcastAll = broadcastAll
	routes.Unlock()

	if !unicast.equals(oldUnicast) || !broadcast.equals(oldBroadcast) {
		routes.onChange()
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
func (routes *Routes) calculateUnicast(establishedAndSymmetric bool) unicastRoutes {
	_, unicast := routes.ourself.Routes(nil, establishedAndSymmetric)
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
func (routes *Routes) calculateBroadcast(establishedAndSymmetric bool) broadcastRoutes {
	broadcast := make(broadcastRoutes)
	for _, peer := range routes.peers.table {
		hops := []PeerName{}
		if found, reached := peer.Routes(routes.ourself.Peer, establishedAndSymmetric); found {
			routes.ourself.ForEachConnectedPeer(establishedAndSymmetric, reached,
				func(remotePeer *Peer) { hops = append(hops, remotePeer.Name) })
		}
		broadcast[peer.Name] = hops
	}
	return broadcast
}

func (a unicastRoutes) equals(b unicastRoutes) bool {
	for key, aval := range a {
		if bval, ok := b[key]; !ok || bval != aval {
			return false
		}
	}

	for key := range b {
		if _, ok := a[key]; !ok {
			return false
		}
	}

	return true
}

func (a broadcastRoutes) equals(b broadcastRoutes) bool {
	set := make(map[PeerName]struct{})

	for key, aval := range a {
		bval, ok := b[key]
		if !ok {
			return false
		}

		for _, peer := range aval {
			set[peer] = struct{}{}
		}

		for _, peer := range bval {
			if _, ok := set[peer]; !ok {
				return false
			}

			delete(set, peer)
		}

		if len(set) != 0 {
			return false
		}

	}

	for key := range b {
		if _, ok := a[key]; !ok {
			return false
		}
	}

	return true
}
