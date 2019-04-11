package mesh

import (
	"math"
	"sync"
)

type UnicastRoutes map[PeerName]PeerName
type BroadcastRoutes map[PeerName][]PeerName

type Routes interface {
	OnChange(callback func())
	PeerNames() PeerNameSet
	Unicast(name PeerName) (PeerName, bool)
	UnicastAll(name PeerName) (PeerName, bool)
	Broadcast(name PeerName) []PeerName
	BroadcastAll(name PeerName) []PeerName
	LookupOrCalculate(name PeerName, broadcast *BroadcastRoutes, establishedAndSymmetric bool) []PeerName
	RandomNeighbours(except PeerName) []PeerName
	Recalculate()
	EnsureRecalculated()
	Run(recalculate <-chan *struct{}, wait <-chan chan struct{}, action <-chan func())
	Calculate()
	CalculateUnicast(establishedAndSymmetric bool) UnicastRoutes
	CalculateBroadcast(name PeerName, establishedAndSymmetric bool) []PeerName
	RLock()
	RUnlock()
	UnicastRoutes() UnicastRoutes
	BroadcastRoutes() BroadcastRoutes
}

// routes aggregates unicast and broadcast routes for our peer.
type defaultRoutes struct {
	sync.RWMutex
	ourself      *LocalPeer
	peers        *Peers
	onChange     []func()
	unicast      UnicastRoutes
	unicastAll   UnicastRoutes // [1]
	broadcast    BroadcastRoutes
	broadcastAll BroadcastRoutes // [1]
	recalc       chan<- *struct{}
	wait         chan<- chan struct{}
	action       chan<- func()
	// [1] based on *all* connections, not just established &
	// symmetric ones
}

// newRoutes returns a usable Routes based on the LocalPeer and existing Peers.
func newRoutes(ourself *LocalPeer, peers *Peers) Routes {
	recalculate := make(chan *struct{}, 1)
	wait := make(chan chan struct{})
	action := make(chan func())
	r := &defaultRoutes{
		ourself:      ourself,
		peers:        peers,
		unicast:      UnicastRoutes{ourself.Name: UnknownPeerName},
		unicastAll:   UnicastRoutes{ourself.Name: UnknownPeerName},
		broadcast:    BroadcastRoutes{ourself.Name: []PeerName{}},
		broadcastAll: BroadcastRoutes{ourself.Name: []PeerName{}},
		recalc:       recalculate,
		wait:         wait,
		action:       action,
	}
	go r.Run(recalculate, wait, action)
	return r
}

func (r *defaultRoutes) UnicastRoutes() UnicastRoutes {
	return r.unicast
}

func (r *defaultRoutes) BroadcastRoutes() BroadcastRoutes {
	return r.broadcast
}

// OnChange appends callback to the functions that will be called whenever the
// routes are recalculated.
func (r *defaultRoutes) OnChange(callback func()) {
	r.Lock()
	defer r.Unlock()
	r.onChange = append(r.onChange, callback)
}

// PeerNames returns the peers that are accountd for in the r.
func (r *defaultRoutes) PeerNames() PeerNameSet {
	return r.peers.Names()
}

// Unicast returns the next hop on the unicast route to the named peer,
// based on established and symmetric connections.
func (r *defaultRoutes) Unicast(name PeerName) (PeerName, bool) {
	r.RLock()
	defer r.RUnlock()
	hop, found := r.unicast[name]
	return hop, found
}

// UnicastAll returns the next hop on the unicast route to the named peer,
// based on all connections.
func (r *defaultRoutes) UnicastAll(name PeerName) (PeerName, bool) {
	r.RLock()
	defer r.RUnlock()
	hop, found := r.unicastAll[name]
	return hop, found
}

// Broadcast returns the set of peer names that should be notified
// when we receive a broadcast message originating from the named peer
// based on established and symmetric connections.
func (r *defaultRoutes) Broadcast(name PeerName) []PeerName {
	return r.LookupOrCalculate(name, &r.broadcast, true)
}

// BroadcastAll returns the set of peer names that should be notified
// when we receive a broadcast message originating from the named peer
// based on all connections.
func (r *defaultRoutes) BroadcastAll(name PeerName) []PeerName {
	return r.LookupOrCalculate(name, &r.broadcastAll, false)
}

func (r *defaultRoutes) LookupOrCalculate(name PeerName, broadcast *BroadcastRoutes, establishedAndSymmetric bool) []PeerName {
	r.RLock()
	hops, found := (*broadcast)[name]
	r.RUnlock()
	if found {
		return hops
	}
	res := make(chan []PeerName)
	r.action <- func() {
		r.RLock()
		hops, found := (*broadcast)[name]
		r.RUnlock()
		if found {
			res <- hops
			return
		}
		r.peers.RLock()
		r.ourself.RLock()
		hops = r.CalculateBroadcast(name, establishedAndSymmetric)
		r.ourself.RUnlock()
		r.peers.RUnlock()
		res <- hops
		r.Lock()
		(*broadcast)[name] = hops
		r.Unlock()
	}
	return <-res
}

// RandomNeighbours chooses min(log2(n_peers), n_neighbouring_peers)
// neighbours, with a random distribution that is topology-sensitive,
// favouring neighbours at the end of "bottleneck links". We determine the
// latter based on the unicast routing table. If a neighbour appears as the
// value more frequently than others - meaning that we reach a higher
// proportion of peers via that neighbour than other neighbours - then it is
// chosen with a higher probability.
//
// Note that we choose log2(n_peers) *neighbours*, not peers. Consequently, on
// sparsely connected peers this function returns a higher proportion of
// neighbours than elsewhere. In extremis, on peers with fewer than
// log2(n_peers) neighbours, all neighbours are returned.
func (r *defaultRoutes) RandomNeighbours(except PeerName) []PeerName {
	destinations := make(PeerNameSet)
	r.RLock()
	defer r.RUnlock()
	count := int(math.Log2(float64(len(r.unicastAll))))
	// depends on go's random map iteration
	for _, dst := range r.unicastAll {
		if dst != UnknownPeerName && dst != except {
			destinations[dst] = struct{}{}
			if len(destinations) >= count {
				break
			}
		}
	}
	res := make([]PeerName, 0, len(destinations))
	for dst := range destinations {
		res = append(res, dst)
	}
	return res
}

// Recalculate requests recalculation of the routing table. This is async but
// can effectively be made synchronous with a subsequent call to
// EnsureRecalculated.
func (r *defaultRoutes) Recalculate() {
	// The use of a 1-capacity channel in combination with the
	// non-blocking send is an optimisation that results in multiple
	// requests being coalesced.
	select {
	case r.recalc <- nil:
	default:
	}
}

// EnsureRecalculated waits for any preceding Recalculate requests to finish.
func (r *defaultRoutes) EnsureRecalculated() {
	done := make(chan struct{})
	r.wait <- done
	<-done
}

func (r *defaultRoutes) Run(recalculate <-chan *struct{}, wait <-chan chan struct{}, action <-chan func()) {
	for {
		select {
		case <-recalculate:
			r.Calculate()
		case done := <-wait:
			select {
			case <-recalculate:
				r.Calculate()
			default:
			}
			close(done)
		case f := <-action:
			f()
		}
	}
}

func (r *defaultRoutes) Calculate() {
	r.peers.RLock()
	r.ourself.RLock()
	var (
		unicast      = r.CalculateUnicast(true)
		unicastAll   = r.CalculateUnicast(false)
		broadcast    = make(BroadcastRoutes)
		broadcastAll = make(BroadcastRoutes)
	)
	broadcast[r.ourself.Name] = r.CalculateBroadcast(r.ourself.Name, true)
	broadcastAll[r.ourself.Name] = r.CalculateBroadcast(r.ourself.Name, false)
	r.ourself.RUnlock()
	r.peers.RUnlock()

	r.Lock()
	r.unicast = unicast
	r.unicastAll = unicastAll
	r.broadcast = broadcast
	r.broadcastAll = broadcastAll
	onChange := r.onChange
	r.Unlock()

	for _, callback := range onChange {
		callback()
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
func (r *defaultRoutes) CalculateUnicast(establishedAndSymmetric bool) UnicastRoutes {
	_, unicast := r.ourself.Routes(nil, establishedAndSymmetric)
	return unicast
}

// Calculate the route to answer the question: if we receive a
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
func (r *defaultRoutes) CalculateBroadcast(name PeerName, establishedAndSymmetric bool) []PeerName {
	hops := []PeerName{}
	peer, found := r.peers.ByName[name]
	if !found {
		return hops
	}
	if found, reached := peer.Routes(r.ourself.Peer, establishedAndSymmetric); found {
		r.ourself.ForEachConnectedPeer(establishedAndSymmetric, reached,
			func(remotePeer *Peer) { hops = append(hops, remotePeer.Name) })
	}
	return hops
}
