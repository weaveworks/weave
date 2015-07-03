package ipam

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
	"time"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/ipam/paxos"
	"github.com/weaveworks/weave/ipam/ring"
	"github.com/weaveworks/weave/ipam/space"
	"github.com/weaveworks/weave/router"
)

// Kinds of message we can unicast to other peers
const (
	msgSpaceRequest = iota
	msgRingUpdate

	paxosInterval = time.Second * 5
	MinSubnetSize = 4 // first and last addresses are excluded, so 2 would be too small
)

// operation represents something which Allocator wants to do, but
// which may need to wait until some other message arrives.
type operation interface {
	// Try attempts this operations and returns false if needs to be tried again.
	Try(alloc *Allocator) bool

	Cancel()

	String() string

	// Does this operation pertain to the given container id?
	// Used for tidying up pending operations when containers die.
	ForContainer(ident string) bool
}

// Allocator brings together Ring and space.Set, and does the
// necessary plumbing.  Runs as a single-threaded Actor, so no locks
// are used around data structures.
type Allocator struct {
	actionChan       chan<- func()
	ourName          router.PeerName
	universe         address.Range                // superset of all ranges
	ring             *ring.Ring                   // information on ranges owned by all peers
	space            space.Space                  // more detail on ranges owned by us
	owned            map[string][]address.Address // who owns what addresses, indexed by container-ID
	nicknames        map[router.PeerName]string   // so we can map nicknames for rmpeer
	pendingAllocates []operation                  // held until we get some free space
	pendingClaims    []operation                  // held until we know who owns the space
	gossip           router.Gossip                // our link to the outside world for sending messages
	paxos            *paxos.Node
	paxosTicker      *time.Ticker
	shuttingDown     bool // to avoid doing any requests while trying to shut down
	now              func() time.Time
}

// NewAllocator creates and initialises a new Allocator
func NewAllocator(ourName router.PeerName, ourUID router.PeerUID, ourNickname string, universe address.Range, quorum uint) *Allocator {
	return &Allocator{
		ourName:   ourName,
		universe:  universe,
		ring:      ring.New(universe.Start, address.Add(universe.Start, universe.Size()), ourName),
		owned:     make(map[string][]address.Address),
		paxos:     paxos.NewNode(ourName, ourUID, quorum),
		nicknames: map[router.PeerName]string{ourName: ourNickname},
		now:       time.Now,
	}
}

// Start runs the allocator goroutine
func (alloc *Allocator) Start() {
	actionChan := make(chan func(), router.ChannelSize)
	alloc.actionChan = actionChan
	go alloc.actorLoop(actionChan)
}

// Stop makes the actor routine exit, for test purposes ONLY because any
// calls after this is processed will hang. Async.
func (alloc *Allocator) Stop() {
	alloc.actionChan <- nil
}

// Operation life cycle

// Given an operation, try it, and add it to the pending queue if it didn't succeed
func (alloc *Allocator) doOperation(op operation, ops *[]operation) {
	alloc.actionChan <- func() {
		if alloc.shuttingDown {
			op.Cancel()
			return
		}
		alloc.establishRing()
		if !op.Try(alloc) {
			*ops = append(*ops, op)
		}
	}
}

// Given an operation, remove it from the pending queue
//  Note the op may not be on the queue; it may have
//  already succeeded.  If it is on the queue, we call
//  cancel on it, allowing callers waiting for the resultChans
//  to unblock.
func (alloc *Allocator) cancelOp(op operation, ops *[]operation) {
	for i, op := range *ops {
		if op == op {
			*ops = append((*ops)[:i], (*ops)[i+1:]...)
			op.Cancel()
			break
		}
	}
}

// Cancel all operations in a queue
func (alloc *Allocator) cancelOps(ops *[]operation) {
	for _, op := range *ops {
		op.Cancel()
	}
	*ops = []operation{}
}

// Cancel all operations for a given container id, returns true
// if we found any.
func (alloc *Allocator) cancelOpsFor(ops *[]operation, ident string) bool {
	var found bool
	for i, op := range *ops {
		if op.ForContainer(ident) {
			found = true
			op.Cancel()
			*ops = append((*ops)[:i], (*ops)[i+1:]...)
		}
	}
	return found
}

// Try all pending operations
func (alloc *Allocator) tryPendingOps() {
	// The slightly different semantics requires us to operate on 'claims' and
	// 'allocates' separately:
	// Claims must be tried before Allocates
	for i := 0; i < len(alloc.pendingClaims); {
		op := alloc.pendingClaims[i]
		if !op.Try(alloc) {
			i++
			continue
		}
		alloc.pendingClaims = append(alloc.pendingClaims[:i], alloc.pendingClaims[i+1:]...)
	}

	// When the first Allocate fails, bail - no need to
	// send too many begs for space.
	for i := 0; i < len(alloc.pendingAllocates); {
		op := alloc.pendingAllocates[i]
		if !op.Try(alloc) {
			break
		}
		alloc.pendingAllocates = append(alloc.pendingAllocates[:i], alloc.pendingAllocates[i+1:]...)
	}
}

func hasBeenCancelled(cancelChan <-chan bool) func() bool {
	return func() bool {
		select {
		case <-cancelChan:
			return true
		default:
			return false
		}
	}
}

// Actor client API

// Allocate (Sync) - get new IP address for container with given name in range
// if there isn't any space in that range we block indefinitely
func (alloc *Allocator) Allocate(ident string, r address.Range, cancelChan <-chan bool) (address.Address, error) {
	resultChan := make(chan allocateResult)
	op := &allocate{resultChan: resultChan, ident: ident, r: r,
		hasBeenCancelled: hasBeenCancelled(cancelChan)}
	alloc.doOperation(op, &alloc.pendingAllocates)
	result := <-resultChan
	return result.addr, result.err
}

// Lookup (Sync) - get existing IP address for container with given name in range
func (alloc *Allocator) Lookup(ident string, r address.Range) (address.Address, error) {
	resultChan := make(chan allocateResult)
	alloc.actionChan <- func() {
		if addr, found := alloc.lookupOwned(ident, r); found {
			resultChan <- allocateResult{addr: addr}
			return
		}
		resultChan <- allocateResult{err: fmt.Errorf("lookup: no address found for %s in range %s", ident, r)}
	}
	result := <-resultChan
	return result.addr, result.err
}

// Claim an address that we think we should own (Sync)
func (alloc *Allocator) Claim(ident string, addr address.Address, cancelChan <-chan bool) error {
	resultChan := make(chan error)
	op := &claim{resultChan: resultChan, ident: ident, addr: addr,
		hasBeenCancelled: hasBeenCancelled(cancelChan)}
	alloc.doOperation(op, &alloc.pendingClaims)
	return <-resultChan
}

// ContainerDied is provided to satisfy the updater interface; does a 'Delete' underneath.  Sync.
func (alloc *Allocator) ContainerDied(ident string) error {
	err := alloc.Delete(ident)
	if err == nil {
		alloc.debugln("Container", ident, "died; released addresses")
	}
	return err
}

// Delete (Sync) - release all IP addresses for container with given name
func (alloc *Allocator) Delete(ident string) error {
	errChan := make(chan error)
	alloc.actionChan <- func() {
		addrs, found := alloc.owned[ident]
		for _, addr := range addrs {
			alloc.space.Free(addr)
		}
		delete(alloc.owned, ident)

		// Also remove any pending ops
		found = alloc.cancelOpsFor(&alloc.pendingAllocates, ident) || found
		found = alloc.cancelOpsFor(&alloc.pendingClaims, ident) || found

		if !found {
			errChan <- fmt.Errorf("Delete: no addresses for %s", ident)
			return
		}
		errChan <- nil
	}
	return <-errChan
}

// Free (Sync) - release single IP address for container
func (alloc *Allocator) Free(ident string, addrToFree address.Address) error {
	errChan := make(chan error)
	alloc.actionChan <- func() {
		addrs := alloc.owned[ident]
		for i, ownedAddr := range addrs {
			if ownedAddr == addrToFree {
				alloc.debugln("Freed", addrToFree, "for", ident)
				if len(addrs) == 1 {
					delete(alloc.owned, ident)
				} else {
					alloc.owned[ident] = append(addrs[:i], addrs[i+1:]...)
				}
				alloc.space.Free(addrToFree)
				errChan <- nil
				return
			}
		}

		errChan <- fmt.Errorf("Free: address %s not found for %s", addrToFree, ident)
	}
	return <-errChan
}

// Sync.
func (alloc *Allocator) String() string {
	resultChan := make(chan string)
	alloc.actionChan <- func() {
		resultChan <- alloc.string()
	}
	return <-resultChan
}

// Shutdown (Sync)
func (alloc *Allocator) Shutdown() {
	alloc.infof("Shutdown")
	doneChan := make(chan struct{})
	alloc.actionChan <- func() {
		alloc.shuttingDown = true
		alloc.cancelOps(&alloc.pendingClaims)
		alloc.cancelOps(&alloc.pendingAllocates)
		if heir := alloc.ring.PickPeerForTransfer(); heir != router.UnknownPeerName {
			alloc.ring.Transfer(alloc.ourName, heir)
			alloc.space.Clear()
			alloc.gossip.GossipBroadcast(alloc.Gossip())
			time.Sleep(100 * time.Millisecond)
		}
		doneChan <- struct{}{}
	}
	<-doneChan
}

// AdminTakeoverRanges (Sync) - take over the ranges owned by a given peer.
// Only done on adminstrator command.
func (alloc *Allocator) AdminTakeoverRanges(peerNameOrNickname string) error {
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		peername, err := alloc.lookupPeername(peerNameOrNickname)
		if err != nil {
			resultChan <- fmt.Errorf("Cannot find peer '%s'", peerNameOrNickname)
			return
		}

		alloc.debugln("AdminTakeoverRanges:", peername)
		if peername == alloc.ourName {
			resultChan <- fmt.Errorf("Cannot take over ranges from yourself!")
			return
		}

		delete(alloc.nicknames, peername)
		newRanges, err := alloc.ring.Transfer(peername, alloc.ourName)
		alloc.space.AddRanges(newRanges)
		resultChan <- err
	}
	return <-resultChan
}

// Lookup a PeerName by nickname or stringified PeerName.  We can't
// call into the router for this because we are interested in peers
// that have gone away but are still in the ring, which is why we
// maintain our own nicknames map.
func (alloc *Allocator) lookupPeername(name string) (router.PeerName, error) {
	for peername, nickname := range alloc.nicknames {
		if nickname == name {
			return peername, nil
		}
	}

	return router.PeerNameFromString(name)
}

// Restrict the peers in "nicknames" to those in the ring and our own
func (alloc *Allocator) pruneNicknames() {
	ringPeers := alloc.ring.PeerNames()
	for name := range alloc.nicknames {
		if _, ok := ringPeers[name]; !ok && name != alloc.ourName {
			delete(alloc.nicknames, name)
		}
	}
}

// OnGossipUnicast (Sync)
func (alloc *Allocator) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	alloc.debugln("OnGossipUnicast from", sender, ": ", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		switch msg[0] {
		case msgSpaceRequest:
			// some other peer asked us for space
			decoder := gob.NewDecoder(bytes.NewReader(msg[1:]))
			var r address.Range
			if err := decoder.Decode(&r); err != nil {
				resultChan <- err
				return
			}
			alloc.donateSpace(r, sender)
			resultChan <- nil
		case msgRingUpdate:
			resultChan <- alloc.update(msg[1:])
		}
	}
	return <-resultChan
}

// OnGossipBroadcast (Sync)
func (alloc *Allocator) OnGossipBroadcast(msg []byte) (router.GossipData, error) {
	alloc.debugln("OnGossipBroadcast:", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		resultChan <- alloc.update(msg)
	}
	return alloc.Gossip(), <-resultChan
}

type gossipState struct {
	// We send a timstamp along with the information to be
	// gossipped in order to detect skewed clocks
	Now       int64
	Nicknames map[router.PeerName]string

	Paxos paxos.GossipState
	Ring  *ring.Ring
}

func (alloc *Allocator) encode() []byte {
	data := gossipState{
		Now:       alloc.now().Unix(),
		Nicknames: alloc.nicknames,
	}

	// We're only interested in Paxos until we have a Ring.
	if alloc.ring.Empty() {
		data.Paxos = alloc.paxos.GossipState()
	} else {
		data.Ring = alloc.ring
	}
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(data); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Encode (Sync)
func (alloc *Allocator) Encode() []byte {
	resultChan := make(chan []byte)
	alloc.actionChan <- func() {
		resultChan <- alloc.encode()
	}
	return <-resultChan
}

// OnGossip (Sync)
func (alloc *Allocator) OnGossip(msg []byte) (router.GossipData, error) {
	alloc.debugln("Allocator.OnGossip:", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		resultChan <- alloc.update(msg)
	}
	return nil, <-resultChan // for now, we never propagate updates. TBD
}

// GossipData implementation is trivial - we always gossip the latest
// data we have at time of sending
type ipamGossipData struct {
	alloc *Allocator
}

func (d *ipamGossipData) Merge(other router.GossipData) {
	// no-op
}

func (d *ipamGossipData) Encode() [][]byte {
	return [][]byte{d.alloc.Encode()}
}

// Gossip returns a GossipData implementation, which in this case always
// returns the latest ring state (and does nothing on merge)
func (alloc *Allocator) Gossip() router.GossipData {
	return &ipamGossipData{alloc}
}

// SetInterfaces gives the allocator two interfaces for talking to the outside world
func (alloc *Allocator) SetInterfaces(gossip router.Gossip) {
	alloc.gossip = gossip
}

// ACTOR server

func (alloc *Allocator) actorLoop(actionChan <-chan func()) {
	for {
		var tickChan <-chan time.Time
		if alloc.paxosTicker != nil {
			tickChan = alloc.paxosTicker.C
		}

		select {
		case action := <-actionChan:
			if action == nil {
				return
			}
			action()
		case <-tickChan:
			alloc.propose()
		}

		alloc.assertInvariants()
		alloc.reportFreeSpace()
	}
}

// Helper functions

func (alloc *Allocator) string() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Allocator range %s", alloc.universe)

	if alloc.ring.Empty() {
		if alloc.paxosTicker != nil {
			fmt.Fprintf(&buf, " awaiting consensus: %s", alloc.paxos.String())
		}
	} else {
		fmt.Fprint(&buf, "\nOwned Ranges:")
		alloc.ring.FprintWithNicknames(&buf, alloc.nicknames)
	}
	if len(alloc.pendingAllocates)+len(alloc.pendingClaims) > 0 {
		fmt.Fprintf(&buf, "\nPending requests:")
		for _, op := range alloc.pendingAllocates {
			fmt.Fprintf(&buf, "\n  %s", op.String())
		}
		for _, op := range alloc.pendingClaims {
			fmt.Fprintf(&buf, "\n  %s", op.String())
		}
	}
	return buf.String()
}

// Ensure we are making progress towards an established ring
func (alloc *Allocator) establishRing() {
	if !alloc.ring.Empty() || alloc.paxosTicker != nil {
		return
	}

	alloc.propose()
	if ok, cons := alloc.paxos.Consensus(); ok {
		// If the quorum was 1, then proposing immediately
		// leads to consensus
		alloc.createRing(cons.Value)
	} else {
		// re-try until we get consensus
		alloc.paxosTicker = time.NewTicker(paxosInterval)
	}
}

func (alloc *Allocator) createRing(peers []router.PeerName) {
	alloc.debugln("Paxos consensus:", peers)
	alloc.ring.ClaimForPeers(normalizeConsensus(peers))
	alloc.gossip.GossipBroadcast(alloc.Gossip())
	alloc.ringUpdated()
}

func (alloc *Allocator) ringUpdated() {
	// When we have a ring, we don't need paxos any more
	if alloc.paxos != nil {
		alloc.paxos = nil

		if alloc.paxosTicker != nil {
			alloc.paxosTicker.Stop()
			alloc.paxosTicker = nil
		}
	}

	alloc.space.UpdateRanges(alloc.ring.OwnedRanges())
	alloc.tryPendingOps()
}

// For compatibility with sort.Interface
type peerNames []router.PeerName

func (a peerNames) Len() int           { return len(a) }
func (a peerNames) Less(i, j int) bool { return a[i] < a[j] }
func (a peerNames) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// When we get a consensus from Paxos, the peer names are not in a
// defined order and may contain duplicates.  This function sorts them
// and de-dupes.
func normalizeConsensus(consensus []router.PeerName) []router.PeerName {
	if len(consensus) == 0 {
		return nil
	}

	peers := make(peerNames, len(consensus))
	copy(peers, consensus)
	sort.Sort(peers)

	dst := 0
	for src := 1; src < len(peers); src++ {
		if peers[dst] != peers[src] {
			dst++
			peers[dst] = peers[src]
		}
	}

	return peers[:dst+1]
}

func (alloc *Allocator) propose() {
	alloc.debugf("Paxos proposing")
	alloc.paxos.Propose()
	alloc.gossip.GossipBroadcast(alloc.Gossip())
}

func (alloc *Allocator) sendSpaceRequest(dest router.PeerName, r address.Range) error {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(r); err != nil {
		panic(err)
	}
	msg := router.Concat([]byte{msgSpaceRequest}, buf.Bytes())
	return alloc.gossip.GossipUnicast(dest, msg)
}

func (alloc *Allocator) sendRingUpdate(dest router.PeerName) {
	msg := router.Concat([]byte{msgRingUpdate}, alloc.encode())
	alloc.gossip.GossipUnicast(dest, msg)
}

func (alloc *Allocator) update(msg []byte) error {
	reader := bytes.NewReader(msg)
	decoder := gob.NewDecoder(reader)
	var data gossipState
	var err error

	if err := decoder.Decode(&data); err != nil {
		return err
	}

	deltat := time.Unix(data.Now, 0).Sub(alloc.now())
	if deltat > time.Hour || -deltat > time.Hour {
		return fmt.Errorf("clock skew of %v detected, ignoring update", deltat)
	}

	// Merge nicknames
	for peer, nickname := range data.Nicknames {
		alloc.nicknames[peer] = nickname
	}

	// only one of Ring and Paxos should be present.  And we
	// shouldn't get updates for a empty Ring. But tolerate
	// them just in case.
	if data.Ring != nil {
		err = alloc.ring.Merge(*data.Ring)
		if !alloc.ring.Empty() {
			alloc.pruneNicknames()
			alloc.ringUpdated()
		}
		return err
	}

	if data.Paxos != nil && alloc.ring.Empty() {
		if alloc.paxos.Update(data.Paxos) {
			if alloc.paxos.Think() {
				// If something important changed, broadcast
				alloc.gossip.GossipBroadcast(alloc.Gossip())
			}

			if ok, cons := alloc.paxos.Consensus(); ok {
				alloc.createRing(cons.Value)
			}
		}
	}

	return nil
}

func (alloc *Allocator) donateSpace(r address.Range, to router.PeerName) {
	// No matter what we do, we'll send a unicast gossip
	// of our ring back to tha chap who asked for space.
	// This serves to both tell him of any space we might
	// have given him, or tell him where he might find some
	// more.
	defer alloc.sendRingUpdate(to)

	alloc.debugln("Peer", to, "asked me for space")
	chunk, ok := alloc.space.Donate(r)
	if !ok {
		free := alloc.space.NumFreeAddressesInRange(r)
		common.Assert(free == 0)
		alloc.debugln("No space to give to peer", to)
		return
	}
	alloc.debugln("Giving range", chunk, "to", to)
	alloc.ring.GrantRangeToHost(chunk.Start, chunk.End, to)
}

func (alloc *Allocator) assertInvariants() {
	// We need to ensure all ranges the ring thinks we own have
	// a corresponding space in the space set, and vice versa
	checkSpace := space.New()
	checkSpace.AddRanges(alloc.ring.OwnedRanges())
	ranges := checkSpace.OwnedRanges()
	spaces := alloc.space.OwnedRanges()

	common.Assert(len(ranges) == len(spaces))

	for i := 0; i < len(ranges); i++ {
		r := ranges[i]
		s := spaces[i]
		common.Assert(s.Start == r.Start && s.End == r.End)
	}
}

func (alloc *Allocator) reportFreeSpace() {
	ranges := alloc.ring.OwnedRanges()
	if len(ranges) == 0 {
		return
	}

	freespace := make(map[address.Address]address.Offset)
	for _, r := range ranges {
		freespace[r.Start] = alloc.space.NumFreeAddressesInRange(r)
	}
	alloc.ring.ReportFree(freespace)
}

// Owned addresses

// NB: addr must not be owned by ident already
func (alloc *Allocator) addOwned(ident string, addr address.Address) {
	alloc.owned[ident] = append(alloc.owned[ident], addr)
}

func (alloc *Allocator) lookupOwned(ident string, r address.Range) (address.Address, bool) {
	for _, addr := range alloc.owned[ident] {
		if r.Contains(addr) {
			return addr, true
		}
	}
	return 0, false
}

func (alloc *Allocator) findOwner(addr address.Address) string {
	for ident, addrs := range alloc.owned {
		for _, candidate := range addrs {
			if candidate == addr {
				return ident
			}
		}
	}
	return ""
}

// Logging

func (alloc *Allocator) infof(fmt string, args ...interface{}) {
	common.Log.Infof("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
func (alloc *Allocator) debugln(args ...interface{}) {
	common.Log.Debugln(append([]interface{}{fmt.Sprintf("[allocator %s]:", alloc.ourName)}, args...)...)
}
func (alloc *Allocator) debugf(fmt string, args ...interface{}) {
	common.Log.Debugf("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
