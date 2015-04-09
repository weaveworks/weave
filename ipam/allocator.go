package ipam

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"

	lg "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/ring"
	"github.com/weaveworks/weave/ipam/space"
	"github.com/weaveworks/weave/ipam/utils"
	"github.com/weaveworks/weave/router"
)

const (
	tombstoneTimeout = 14 * 24 * time.Hour
)

// Kinds of message we can unicast to other peers
const (
	msgSpaceRequest = iota
	msgLeaderElected
	msgRingUpdate
)

type pendingAllocation struct {
	resultChan chan<- net.IP
	Ident      string
}

// Allocator brings together Ring and space.Set, and does the nessecary plumbing.
type Allocator struct {
	actionChan    chan<- func()
	ourName       router.PeerName
	universeStart net.IP
	universeSize  uint32
	universeLen   int        // length of network prefix (e.g. 24 for a /24 network)
	ring          *ring.Ring // it's for you!
	spaceSet      space.Set
	owned         map[string][]net.IP // who owns what address, indexed by container-ID
	pending       []pendingAllocation
	claims        claimList
	gossip        router.Gossip
	shuttingDown  bool
}

// NewAllocator creats and initialises a new Allocator
func NewAllocator(ourName router.PeerName, universeCIDR string) (*Allocator, error) {
	_, universeNet, err := net.ParseCIDR(universeCIDR)
	if err != nil {
		return nil, err
	}
	if universeNet.IP.To4() == nil {
		return nil, errors.New("Non-IPv4 address not supported")
	}
	// Get the size of the network from the mask
	ones, bits := universeNet.Mask.Size()
	var universeSize uint32 = 1 << uint(bits-ones)
	if universeSize < 4 {
		return nil, errors.New("Allocation universe too small")
	}
	alloc := &Allocator{
		ourName:       ourName,
		universeStart: universeNet.IP,
		universeSize:  universeSize,
		universeLen:   ones,
		ring:          ring.New(utils.Add(universeNet.IP, 1), utils.Add(universeNet.IP, universeSize-1), ourName),
		owned:         make(map[string][]net.IP),
	}
	return alloc, nil
}

// Start runs the allocator goroutine
func (alloc *Allocator) Start() {
	actionChan := make(chan func(), router.ChannelSize)
	alloc.actionChan = actionChan
	go alloc.actorLoop(actionChan)
}

// Actor client API

// GetFor (Sync) - get IP address for container with given name
func (alloc *Allocator) GetFor(ident string, cancelChan <-chan bool) net.IP {
	resultChan := make(chan net.IP, 1) // len 1 so actor can send while cancel is in progress
	alloc.actionChan <- func() {
		if alloc.shuttingDown {
			resultChan <- nil
			return
		}
		alloc.electLeaderIfNecessary()
		if addr, success := alloc.tryAllocateFor(ident); success {
			resultChan <- addr
		} else {
			alloc.pending = append(alloc.pending, pendingAllocation{resultChan, ident})
		}
	}
	select {
	case result := <-resultChan:
		return result
	case <-cancelChan:
		alloc.actionChan <- func() {
			for i, pending := range alloc.pending {
				if pending.Ident == ident {
					alloc.pending = append(alloc.pending[:i], alloc.pending[i+1:]...)
					break
				}
			}
		}
		return nil
	}
}

// Free (Sync) - release IP address for container with given name
func (alloc *Allocator) Free(ident string, addr net.IP) error {
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		if alloc.removeOwned(ident, addr) {
			resultChan <- alloc.spaceSet.Free(addr)
		} else {
			resultChan <- fmt.Errorf("free: %s not owned by %s", addr, ident)
		}
	}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) String() string {
	resultChan := make(chan string)
	alloc.actionChan <- func() {
		resultChan <- alloc.string()
	}
	return <-resultChan
}

// ContainerDied is provided to satisfy the updater interface; does a free underneath.  Async.
func (alloc *Allocator) ContainerDied(ident string) error {
	alloc.debugln("Container", ident, "died; releasing addresses")
	alloc.actionChan <- func() {
		for _, ip := range alloc.owned[ident] {
			alloc.spaceSet.Free(ip)
		}
		delete(alloc.owned, ident)
	}
	return nil // this is to satisfy the ContainerObserver interface
}

// Shutdown (Sync)
func (alloc *Allocator) Shutdown() {
	alloc.infof("Shutdown")
	doneChan := make(chan struct{})
	alloc.actionChan <- func() {
		alloc.shuttingDown = true
		alloc.ring.TombstonePeer(alloc.ourName, 100)
		alloc.gossip.GossipBroadcast(alloc.Gossip())
		alloc.spaceSet.Clear()
		time.Sleep(100 * time.Millisecond)
		doneChan <- struct{}{}
	}
	<-doneChan
}

// TombstonePeer (Sync) - inserts tombstones for given peer, freeing up the ranges the
// peer owns.  Eventually the peer will go away.
func (alloc *Allocator) TombstonePeer(peer router.PeerName) error {
	alloc.debugln("TombstonePeer:", peer)
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		err := alloc.ring.TombstonePeer(peer, tombstoneTimeout)
		alloc.considerNewSpaces()
		resultChan <- err
	}
	return <-resultChan
}

// ListPeers (Sync) - returns list of peer names known to the ring
func (alloc *Allocator) ListPeers() []router.PeerName {
	resultChan := make(chan []router.PeerName)
	alloc.actionChan <- func() {
		peers := make(map[router.PeerName]bool)
		for _, entry := range alloc.ring.Entries {
			peers[entry.Peer] = true
		}

		result := make([]router.PeerName, 0, len(peers))
		for peer := range peers {
			result = append(result, peer)
		}
		resultChan <- result
	}
	return <-resultChan
}

// Claim an address that we think we should own (Sync)
func (alloc *Allocator) Claim(ident string, addr net.IP, cancelChan <-chan bool) error {
	resultChan := make(chan error, 1)
	alloc.actionChan <- func() {
		if alloc.shuttingDown {
			resultChan <- fmt.Errorf("Claim %s: allocator is shutting down", addr)
			return
		}
		alloc.electLeaderIfNecessary()
		alloc.handleClaim(ident, addr, resultChan)
	}
	select {
	case result := <-resultChan:
		return result
	case <-cancelChan:
		alloc.actionChan <- func() {
			alloc.handleCancelClaim(ident, addr)
		}
		return nil
	}
}

// OnGossipUnicast (Sync)
func (alloc *Allocator) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	alloc.debugln("OnGossipUnicast from", sender, ": ", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		switch msg[0] {
		case msgLeaderElected:
			resultChan <- alloc.handleLeaderElected()
		case msgSpaceRequest:
			// some other peer asked us for space
			alloc.donateSpace(sender)
			resultChan <- nil
		case msgRingUpdate:
			resultChan <- alloc.updateRing(msg[1:])
		}
	}
	return <-resultChan
}

// OnGossipBroadcast (Sync)
func (alloc *Allocator) OnGossipBroadcast(msg []byte) (router.GossipData, error) {
	alloc.debugln("OnGossipBroadcast:", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		resultChan <- alloc.updateRing(msg)
	}
	return alloc.Gossip(), <-resultChan
}

// Encode (Sync)
func (alloc *Allocator) Encode() []byte {
	resultChan := make(chan []byte)
	alloc.actionChan <- func() {
		resultChan <- alloc.ring.GossipState()
	}
	return <-resultChan
}

// OnGossip (Sync)
func (alloc *Allocator) OnGossip(msg []byte) (router.GossipData, error) {
	alloc.debugln("Allocator.OnGossip:", len(msg), "bytes")
	resultChan := make(chan error)
	alloc.actionChan <- func() {
		resultChan <- alloc.updateRing(msg)
	}
	err := <-resultChan
	return nil, err // for now, we never propagate updates. TBD
}

// GossipData implementation is trivial - we always gossip the whole ring
type ipamGossipData struct {
	alloc *Allocator
}

func (d *ipamGossipData) Merge(other router.GossipData) {
	// no-op
}

func (d *ipamGossipData) Encode() []byte {
	return d.alloc.Encode()
}

// Gossip returns a GossipData implementation, which in this case always
// returns the latest ring state (and does nothing on merge)
func (alloc *Allocator) Gossip() router.GossipData {
	return &ipamGossipData{alloc}
}

// SetGossip gives the allocator an interface for talking to the outside world
func (alloc *Allocator) SetGossip(gossip router.Gossip) {
	alloc.gossip = gossip
}

// ACTOR server

func (alloc *Allocator) actorLoop(actionChan <-chan func()) {
	for {
		select {
		case action := <-actionChan:
			if action == nil {
				return
			}
			action()
		}
		alloc.assertInvariants()
		alloc.reportFreeSpace()
		alloc.ring.ExpireTombstones(time.Now().Unix())
	}
}

// Helper functions

func (alloc *Allocator) string() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Allocator universe %s+%d\n", alloc.universeStart, alloc.universeSize)
	fmt.Fprintf(&buf, alloc.ring.String())
	fmt.Fprintf(&buf, alloc.spaceSet.String())
	fmt.Fprintf(&buf, "\nPending requests for ")
	for _, pending := range alloc.pending {
		fmt.Fprintf(&buf, "%s, ", pending.Ident)
	}
	return buf.String()
}

func (alloc *Allocator) checkPendingAllocations() {
	i := 0
	for ; i < len(alloc.pending); i++ {
		if addr, success := alloc.tryAllocateFor(alloc.pending[i].Ident); success {
			alloc.pending[i].resultChan <- addr
		} else {
			break
		}
	}
	alloc.pending = alloc.pending[i:]
}

// Fairly quick check of what's going on; whether requests should now be
// replied to, etc.
func (alloc *Allocator) considerOurPosition() {
	alloc.checkPendingAllocations()
	alloc.checkClaims()
}

func (alloc *Allocator) electLeaderIfNecessary() {
	if !alloc.ring.Empty() {
		return
	}
	leader := alloc.gossip.(router.Leadership).LeaderElect()
	alloc.debugln("Elected leader:", leader)
	if leader == alloc.ourName {
		// I'm the winner; take control of the whole universe
		alloc.ring.ClaimItAll()
		alloc.considerNewSpaces()
		alloc.infof("I was elected leader of the universe\n%s", alloc.string())
		alloc.gossip.GossipBroadcast(alloc.Gossip())
		alloc.considerOurPosition()
	} else {
		alloc.sendRequest(leader, msgLeaderElected)
	}
}

// return true if the request is completed, false if pending
func (alloc *Allocator) tryAllocateFor(ident string) (net.IP, bool) {
	// If we have previously stored an address for this container, return it.
	// FIXME: the data structure allows multiple addresses per (allocator per) ident but the code doesn't
	if addrs, found := alloc.owned[ident]; found && len(addrs) > 0 {
		return addrs[0], true
	}
	if addr := alloc.spaceSet.Allocate(); addr != nil {
		alloc.debugln("Allocated", addr, "for", ident)
		alloc.addOwned(ident, addr)
		return addr, true
	}

	// out of space
	if donor, err := alloc.ring.ChoosePeerToAskForSpace(); err == nil {
		alloc.debugln("Decided to ask peer", donor, "for space")
		alloc.sendRequest(donor, msgSpaceRequest)
	}

	return nil, false
}

func (alloc *Allocator) handleLeaderElected() error {
	// some other peer decided we were the leader:
	// if we already have tokens then they didn't get the memo; repeat
	if !alloc.ring.Empty() {
		alloc.gossip.GossipBroadcast(alloc.Gossip())
	} else {
		// re-run the election here to avoid races
		alloc.electLeaderIfNecessary()
	}
	return nil
}

func (alloc *Allocator) sendRequest(dest router.PeerName, kind byte) {
	msg := router.Concat([]byte{kind}, alloc.ring.GossipState())
	alloc.gossip.GossipUnicast(dest, msg)
}

func (alloc *Allocator) updateRing(msg []byte) error {
	err := alloc.ring.UpdateRing(msg)
	alloc.considerNewSpaces()
	alloc.considerOurPosition()
	return err
}

func (alloc *Allocator) donateSpace(to router.PeerName) {
	// No matter what we do, we'll send a unicast gossip
	// of our ring back to tha chap who asked for space.
	// This serves to both tell him of any space we might
	// have given him, or tell him where he might find some
	// more.
	defer alloc.sendRequest(to, msgRingUpdate)

	alloc.debugln("Peer", to, "asked me for space")
	start, size, ok := alloc.spaceSet.GiveUpSpace()
	if !ok {
		free := alloc.spaceSet.NumFreeAddresses()
		utils.Assert(free == 0,
			fmt.Sprintf("Couldn't give up space but I have %d free addresses", free))
		alloc.debugln("No space to give to peer", to)
		return
	}
	end := utils.IntIP4(utils.IP4int(start) + size)
	alloc.debugln("Giving range", start, end, size, "to", to)
	alloc.ring.GrantRangeToHost(start, end, to)
}

// considerNewSpaces iterates through ranges in the ring
// and ensures we have spaces for them.  It only ever adds
// new spaces, as the invariants in the ring ensure we never
// have spaces taken away from us against our will.
func (alloc *Allocator) considerNewSpaces() {
	ownedRanges := alloc.ring.OwnedRanges()
	for _, r := range ownedRanges {
		size := uint32(utils.Subtract(r.End, r.Start))
		s, exists := alloc.spaceSet.Get(r.Start)
		if !exists {
			alloc.debugf("Found new space [%s, %s)", r.Start, r.End)
			alloc.spaceSet.AddSpace(space.Space{Start: r.Start, Size: size})
			continue
		}

		if s.Size < size {
			alloc.debugf("Growing space starting at %s to %d", s.Start, size)
			s.Grow(size)
		}
	}
}

func (alloc *Allocator) assertInvariants() {
	// We need to ensure all ranges the ring thinks we own have
	// a corresponding space in the space set, and vice versa
	ranges := alloc.ring.OwnedRanges()
	spaces := alloc.spaceSet.Spaces()

	utils.Assert(len(ranges) == len(spaces), "Ring and SpaceSet are out of sync!")

	for i := 0; i < len(ranges); i++ {
		r := ranges[i]
		s := spaces[i]

		rSize := uint32(utils.Subtract(r.End, r.Start))
		utils.Assert(s.Start.Equal(r.Start) && s.Size == rSize,
			fmt.Sprintf("Range starting at %s out of sync with space set!", r.Start))
	}
}

func (alloc *Allocator) reportFreeSpace() {
	spaces := alloc.spaceSet.Spaces()

	for _, s := range spaces {
		alloc.ring.ReportFree(s.Start, s.NumFreeAddresses())
	}
}

// Handling Claims

type pendingClaim struct {
	resultChan chan<- error
	Ident      string
	IP         net.IP
}

type claimList []pendingClaim

func (aa *claimList) removeAt(pos int) {
	(*aa) = append((*aa)[:pos], (*aa)[pos+1:]...)
}

func (aa *claimList) find(addr net.IP) int {
	for i, a := range *aa {
		if a.IP.Equal(addr) {
			return i
		}
	}
	return -1
}

// Claim an address that we think we should own
func (alloc *Allocator) handleClaim(ident string, addr net.IP, resultChan chan<- error) {
	if !alloc.ring.Contains(addr) {
		// Address not within our universe; assume user knows what they are doing
		alloc.infof("Ignored address %s claimed by %s - not in our universe", addr, ident)
		resultChan <- nil
		return
	}
	// See if it's already claimed
	if pos := alloc.claims.find(addr); pos >= 0 && alloc.claims[pos].Ident != ident {
		resultChan <- fmt.Errorf("IP address %s already claimed by %s", addr, alloc.claims[pos].Ident)
		return
	}
	alloc.infof("Address %s claimed by %s", addr, ident)
	if owner, err := alloc.checkClaim(ident, addr); err != nil {
		resultChan <- err
	} else if owner != alloc.ourName {
		alloc.infof("Address %s owned by %s", addr, owner)
		alloc.claims = append(alloc.claims, pendingClaim{resultChan, ident, addr})
	} else {
		resultChan <- nil
	}
}

func (alloc *Allocator) handleCancelClaim(ident string, addr net.IP) {
	for i, claim := range alloc.claims {
		if claim.Ident == ident && claim.IP.Equal(addr) {
			alloc.claims = append(alloc.claims[:i], alloc.claims[i+1:]...)
			break
		}
	}
}

func (alloc *Allocator) checkClaim(ident string, addr net.IP) (owner router.PeerName, err error) {
	if owner := alloc.ring.Owner(addr); owner == alloc.ourName {
		// We own the space; see if we already have an owner for that particular address
		if existingIdent := alloc.findOwner(addr); existingIdent == "" {
			alloc.addOwned(ident, addr)
			err := alloc.spaceSet.Claim(addr)
			return alloc.ourName, err
		} else if existingIdent == ident { // same identifier is claiming same address; that's OK
			return alloc.ourName, nil
		} else {
			return alloc.ourName, fmt.Errorf("Claimed address %s is already owned by %s", addr, existingIdent)
		}
	} else {
		return owner, nil
	}
}

func (alloc *Allocator) checkClaims() {
	for i := 0; i < len(alloc.claims); i++ {
		owner, err := alloc.checkClaim(alloc.claims[i].Ident, alloc.claims[i].IP)
		if err != nil || owner == alloc.ourName {
			alloc.claims[i].resultChan <- err
			alloc.claims = append(alloc.claims[:i], alloc.claims[i+1:]...)
			i--
		}
	}
}

// Owned addresses

func (alloc *Allocator) addOwned(ident string, addr net.IP) {
	alloc.owned[ident] = append(alloc.owned[ident], addr)
}

func (alloc *Allocator) findOwner(addr net.IP) string {
	for ident, addrs := range alloc.owned {
		for _, ip := range addrs {
			if ip.Equal(addr) {
				return ident
			}
		}
	}
	return ""
}

func (alloc *Allocator) removeOwned(ident string, addr net.IP) bool {
	if addrs, found := alloc.owned[ident]; found {
		for i, ip := range addrs {
			if ip.Equal(addr) {
				alloc.owned[ident] = append(addrs[:i], addrs[i+1:]...)
				return true
			}
		}
	}
	return false
}

// Logging

func (alloc *Allocator) errorln(args ...interface{}) {
	lg.Error.Println(append([]interface{}{fmt.Sprintf("[allocator %s]:", alloc.ourName)}, args...)...)
}
func (alloc *Allocator) infof(fmt string, args ...interface{}) {
	lg.Info.Printf("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
func (alloc *Allocator) debugln(args ...interface{}) {
	lg.Debug.Println(append([]interface{}{fmt.Sprintf("[allocator %s]:", alloc.ourName)}, args...)...)
}
func (alloc *Allocator) debugf(fmt string, args ...interface{}) {
	lg.Debug.Printf("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
