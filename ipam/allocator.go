package ipam

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	"net"
	"time"
)

const (
	GossipReqTimeout     = 1 * time.Second
	GossipWaitForLead    = 10 * time.Second
	GossipDeadTimeout    = 10 * time.Second
	MaxAddressesToGiveUp = 256
)
const (
	msgSpaceRequest = iota
	msgSpaceDonate
	msgSpaceClaim
	msgSpaceClaimRefused
)

const (
	allocStateNeutral = iota
	allocStateLeaderless
)

// Some in-flight request that we have made to another peer
type request struct {
	dest    router.PeerName
	kind    byte
	details *MinSpace
	expires time.Time
}

type requestList []*request

func (list requestList) find(sender router.PeerName, space Space) int {
	for i, r := range list {
		if r.dest == sender {
			if r.kind == msgSpaceRequest || r.details.Start.Equal(space.GetStart()) {
				return i
			}
		}
	}
	return -1
}

func (list requestList) findKind(kind byte) int {
	for i, r := range list {
		if r.kind == kind {
			return i
		}
	}
	return -1
}

func (list *requestList) removeAt(pos int) {
	(*list) = append((*list)[:pos], (*list)[pos+1:]...)
}

func (list *requestList) remove(sender router.PeerName, space Space) {
	if pos := list.find(sender, space); pos >= 0 {
		list.removeAt(pos)
	}
}

// To allow time itself to be stubbed out for testing
type timeProvider interface {
	Now() time.Time
}

type defaultTime struct{}

func (defaultTime) Now() time.Time { return time.Now() }

type stop struct{}

type makeString struct {
	resultChan chan<- string
}

type claim struct {
	resultChan chan<- error
	Allocation
}

type allocateFor struct {
	resultChan chan<- net.IP
	Ident      string
}

type free struct {
	resultChan chan<- error
	addr       net.IP
}

type deleteRecordsFor struct {
	Ident string
}

type gossipUnicast struct {
	resultChan chan<- error
	sender     router.PeerName
	bytes      []byte
}

type gossipBroadcast struct {
	resultChan chan<- error
	bytes      []byte
}

type gossipCreate struct {
	resultChan chan<- []byte
	bytes      []byte
}

type gossipReceived struct {
	resultChan chan<- gossipReply
	bytes      []byte
}

type gossipReply struct {
	err   error
	bytes []byte
}

type onDead struct {
	name router.PeerName
	uid  uint64
}

type claimList []claim

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

type Allocator struct {
	queryChan   chan<- interface{}
	ourName     router.PeerName
	ourUID      uint64
	state       int
	universeLen int
	universe    MinSpace // all the addresses that could be allocated
	gossip      router.Gossip
	peerInfo    map[uint64]SpaceSet // indexed by peer UID
	ourSpaceSet *OurSpaceSet
	pastLife    *PeerSpaceSet // Probably allocations from a previous incarnation
	leaked      map[time.Time]Space
	claims      claimList
	pending     []allocateFor
	inflight    requestList
	timeProvider
}

func NewAllocator(ourName router.PeerName, ourUID uint64, universeCIDR string) (*Allocator, error) {
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
	// Per RFC1122, don't allocate the first (network) and last (broadcast) addresses
	alloc := &Allocator{
		ourName:      ourName,
		ourUID:       ourUID,
		state:        allocStateLeaderless,
		universeLen:  ones,
		universe:     MinSpace{Start: add(universeNet.IP, 1), Size: universeSize - 2},
		peerInfo:     make(map[uint64]SpaceSet),
		ourSpaceSet:  NewSpaceSet(ourName, ourUID),
		leaked:       make(map[time.Time]Space),
		timeProvider: defaultTime{},
	}
	alloc.peerInfo[ourUID] = alloc.ourSpaceSet
	return alloc, nil
}

func (alloc *Allocator) SetGossip(gossip router.Gossip) {
	alloc.gossip = gossip
}

func (alloc *Allocator) Start() {
	alloc.moveToState(allocStateLeaderless)
	queryChan := make(chan interface{}, router.ChannelSize)
	alloc.queryChan = queryChan
	gossipTimer := time.Tick(router.GossipInterval)
	go alloc.queryLoop(queryChan, gossipTimer)
}

func (alloc *Allocator) startForTesting() {
	alloc.moveToState(allocStateLeaderless)
	queryChan := make(chan interface{}, router.ChannelSize)
	alloc.queryChan = queryChan
	go alloc.queryLoop(queryChan, nil)
}

// Actor client API

// Sync.
func (alloc *Allocator) Stop() {
	alloc.queryChan <- stop{}
}

// Sync.
// Claim an address that we think we should own
func (alloc *Allocator) Claim(ident string, addr net.IP) error {
	lg.Info.Printf("Address %s claimed by %s", addr, ident)
	resultChan := make(chan error)
	alloc.queryChan <- claim{resultChan, Allocation{ident, addr}}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) AllocateFor(ident string) net.IP {
	resultChan := make(chan net.IP)
	alloc.queryChan <- allocateFor{resultChan, ident}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) Free(addr net.IP) error {
	resultChan := make(chan error)
	alloc.queryChan <- free{resultChan, addr}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) String() string {
	resultChan := make(chan string)
	alloc.queryChan <- makeString{resultChan}
	return <-resultChan
}

// Async.
func (alloc *Allocator) DeleteRecordsFor(ident string) error {
	alloc.queryChan <- deleteRecordsFor{ident}
	return nil // this is to satisfy the ContainerObserver interface
}

// Sync.
func (alloc *Allocator) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	lg.Debug.Printf("OnGossipUnicast from %s: %d bytes\n", sender, len(msg))
	resultChan := make(chan error)
	alloc.queryChan <- gossipUnicast{resultChan, sender, msg}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) OnGossipBroadcast(msg []byte) error {
	lg.Debug.Printf("OnGossipBroadcast: %d bytes\n", len(msg))
	resultChan := make(chan error)
	alloc.queryChan <- gossipBroadcast{resultChan, msg}
	return <-resultChan
}

// Sync.
func (alloc *Allocator) OnGossip(msg []byte) ([]byte, error) {
	lg.Debug.Printf("Allocator.OnGossip: %d bytes\n", len(msg))
	resultChan := make(chan gossipReply)
	alloc.queryChan <- gossipReceived{resultChan, msg}
	ret := <-resultChan
	return ret.bytes, ret.err
}

// Sync.
func (alloc *Allocator) Gossip() []byte {
	resultChan := make(chan []byte)
	alloc.queryChan <- gossipCreate{resultChan, nil}
	return <-resultChan
}

// No-op
func (alloc *Allocator) OnAlive(name router.PeerName, uid uint64) {
	// If it's new to us, nothing to do.
	// If we previously believed it to be dead, need to figure that case out.
}

// Async.
func (alloc *Allocator) OnDead(name router.PeerName, uid uint64) {
	alloc.queryChan <- onDead{name, uid}
}

// ACTOR server

func (alloc *Allocator) queryLoop(queryChan <-chan interface{}, gossipTimer <-chan time.Time) {
	for {
		prevState := alloc.ourSpaceSet.HasFreeAddresses()
		changed := false
		select {
		case query, ok := <-queryChan:
			if !ok {
				return
			}
			switch q := query.(type) {
			case stop:
				return
			case makeString:
				q.resultChan <- alloc.string()
			case claim:
				alloc.electLeaderIfNecessary()
				alloc.handleClaim(q.Ident, q.IP, q.resultChan)
			case allocateFor:
				alloc.electLeaderIfNecessary()
				if !alloc.tryAllocateFor(q.Ident, q.resultChan) {
					alloc.pending = append(alloc.pending, allocateFor{q.resultChan, q.Ident})
				}
			case deleteRecordsFor:
				alloc.ourSpaceSet.DeleteRecordsFor(q.Ident)
			case free:
				q.resultChan <- alloc.ourSpaceSet.Free(q.addr)
			case gossipUnicast:
				switch q.bytes[0] {
				case msgSpaceRequest:
					q.resultChan <- alloc.handleSpaceRequest(q.sender, q.bytes[1:])
				case msgSpaceDonate:
					q.resultChan <- alloc.handleSpaceDonate(q.sender, q.bytes[1:])
				case msgSpaceClaim:
					q.resultChan <- alloc.handleSpaceClaim(q.sender, q.bytes[1:])
				case msgSpaceClaimRefused:
					q.resultChan <- alloc.handleSpaceClaimRefused(q.sender, q.bytes[1:])
				default:
					q.resultChan <- errors.New(fmt.Sprint("Unexpected gossip unicast message: ", q.bytes[0]))
				}
			case gossipBroadcast:
				q.resultChan <- alloc.handleGossipBroadcast(q.bytes)
			case gossipReceived:
				buf, err := alloc.handleGossipReceived(q.bytes)
				q.resultChan <- gossipReply{err, buf}
			case gossipCreate:
				q.resultChan <- alloc.handleGossipCreate()
			case onDead:
				alloc.handleDead(q.name, q.uid)
			}
		case <-gossipTimer:
			changed = alloc.considerOurPosition()
		}
		if changed || (prevState != alloc.ourSpaceSet.HasFreeAddresses()) {
			alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
		}
	}
}

func (alloc *Allocator) manageSpace(startAddr net.IP, poolSize uint32) {
	alloc.ourSpaceSet.AddSpace(NewSpace(startAddr, poolSize))
}

func encode(spaceset SpaceSet) []byte {
	return GobEncode(1, spaceset)
}

// Unpack the supplied buffer which is encoded as per encode() above.
// return a slice of MinSpace containing those PeerSpaces which are newer
// than what we had previously
func (alloc *Allocator) decodeFromDecoder(decoder *gob.Decoder) ([]*PeerSpaceSet, error) {
	var numSpaceSets int
	if err := decoder.Decode(&numSpaceSets); err != nil {
		return nil, err
	}
	ret := make([]*PeerSpaceSet, 0)
	for i := 0; i < numSpaceSets; i++ {
		newSpaceset := new(PeerSpaceSet)
		if err := newSpaceset.Decode(decoder); err != nil {
			return nil, err
		}
		// compare this received spaceset's version against the one we had prev.
		oldSpaceset, found := alloc.peerInfo[newSpaceset.UID()]
		if !found || newSpaceset.Version() > oldSpaceset.Version() {
			if newSpaceset.UID() == alloc.ourUID {
				lg.Error.Println("Received update to our own info")
				continue // Shouldn't happen
			} else if newSpaceset.PeerName() == alloc.ourName {
				lg.Debug.Println("Received update with our peerName but different UID")
				if alloc.pastLife == nil || alloc.pastLife.lastSeen.Before(newSpaceset.lastSeen) {
					alloc.pastLife = newSpaceset
				}
				continue
			} else if oldSpaceset != nil && oldSpaceset.MaybeDead() {
				lg.Info.Println("Received update for peer believed dead", newSpaceset)
			}
			lg.Debug.Println("Replacing data with newer version", newSpaceset)
			alloc.peerInfo[newSpaceset.UID()] = newSpaceset
			if alloc.state == allocStateLeaderless && !newSpaceset.Empty() {
				alloc.moveToState(allocStateNeutral)
			}
			ret = append(ret, newSpaceset)
		}
	}
	return ret, nil
}

func (alloc *Allocator) decodeUpdate(update []byte) ([]*PeerSpaceSet, error) {
	reader := bytes.NewReader(update)
	decoder := gob.NewDecoder(reader)
	return alloc.decodeFromDecoder(decoder)
}

func (alloc *Allocator) spaceOwner(space *MinSpace) uint64 {
	for uid, spaceset := range alloc.peerInfo {
		if spaceset.Overlaps(space) {
			return uid
		}
	}
	return 0
}

func (alloc *Allocator) lookForDead(now time.Time) {
	limit := now.Add(-GossipDeadTimeout)
	for _, entry := range alloc.peerInfo {
		if peerEntry, ok := entry.(*PeerSpaceSet); ok &&
			peerEntry.MaybeDead() && !peerEntry.IsTombstone() &&
			peerEntry.lastSeen.Before(limit) {
			peerEntry.MakeTombstone()
			lg.Debug.Println("Tombstoned", peerEntry)
			alloc.gossip.GossipBroadcast(encode(peerEntry))
		}
	}
}

func (alloc *Allocator) lookForNewLeaks(now time.Time) {
	allSpace := NewSpaceSet(router.UnknownPeerName, 0)
	allSpace.AddSpace(NewSpace(alloc.universe.Start, alloc.universe.Size))
	for _, peerSpaceSet := range alloc.peerInfo {
		peerSpaceSet.ForEachSpace(func(space Space) {
			allSpace.Exclude(space)
		})
	}
	if !allSpace.Empty() {
		// Now remove the leaks we already knew about
		for _, leak := range alloc.leaked {
			allSpace.Exclude(leak)
		}
		if !allSpace.Empty() {
			lg.Debug.Println(allSpace.describe("New leaked spaces:"))
			for _, space := range allSpace.spaces {
				alloc.leaked[now] = space
				break // can only store one space against each time
			}
		}
	}
}

func (alloc *Allocator) discardOldLeaks() {
	for age, leak := range alloc.leaked {
		for _, peerSpaceSet := range alloc.peerInfo {
			if peerSpaceSet.Overlaps(leak) {
				lg.Debug.Printf("Discarding non-leak %+v", leak)
				// Really, we should only discard the piece that is overlapped, but
				// this way is simpler and we will recover any real leaks in the end
				delete(alloc.leaked, age)
			}
		}
	}
}

// look for leaks which are aged, and which we are heir to
func (alloc *Allocator) reclaimLeaks(now time.Time) (changed bool) {
	changed = false
	limit := now.Add(-GossipDeadTimeout)
	for age, leak := range alloc.leaked {
		if age.Before(limit) {
			for _, space := range alloc.ourSpaceSet.spaces {
				if space.IsHeirTo(leak, &alloc.universe) {
					lg.Info.Printf("Reclaiming leak %+v heir %+v", leak, space)
					delete(alloc.leaked, age)
					if !space.(*MutableSpace).mergeBlank(leak) { // If we can't merge the two, add another
						alloc.manageSpace(leak.GetStart(), leak.GetSize())
					}
					changed = true
					break
				}
			}
		}
	}
	if changed {
		alloc.ourSpaceSet.version++
	}
	return
}

func (alloc *Allocator) reclaimPastLife() {
	lg.Debug.Println("Reclaiming allocations from past life", alloc.pastLife)
	for _, space := range alloc.pastLife.spaces {
		alloc.manageSpace(space.GetStart(), space.GetSize())
	}
	alloc.pastLife.MakeTombstone()
	alloc.gossip.GossipBroadcast(encode(alloc.pastLife))
	lg.Debug.Println("alloc now", alloc.string())
}

func (alloc *Allocator) checkClaim(ident string, addr net.IP) (owner uint64, err error) {
	testaddr := NewMinSpace(addr, 1)
	if alloc.pastLife != nil && alloc.pastLife.Overlaps(testaddr) {
		// We've been sent a peerInfo that matches our PeerName but not UID
		// We've also been asked to claim an IP that is in the range it owned
		// Conclude that this is an echo of our former self, and reclaim it.
		alloc.reclaimPastLife()
	}
	if owner := alloc.spaceOwner(testaddr); owner == 0 {
		// That address is not currently owned; wait until someone claims it
		return 0, nil
	} else if spaceSet := alloc.peerInfo[owner]; spaceSet == alloc.ourSpaceSet {
		// We own it, perhaps because we claimed it above.
		err := alloc.ourSpaceSet.Claim(ident, addr)
		return alloc.ourUID, err
	} else {
		// That address is owned by someone else
		claimspace := MinSpace{addr, 1}
		if alloc.inflight.find(spaceSet.PeerName(), &claimspace) < 0 { // Have we already requested this one?
			lg.Debug.Println("Claiming address", addr, "from peer:", spaceSet.PeerName())
			alloc.sendRequest(spaceSet.PeerName(), msgSpaceClaim, &claimspace)
		}
		return owner, nil
	}
}

func (alloc *Allocator) checkClaims() {
	for i := 0; i < len(alloc.claims); i++ {
		owner, err := alloc.checkClaim(alloc.claims[i].Ident, alloc.claims[i].IP)
		if err != nil || owner == alloc.ourUID {
			alloc.claims[i].resultChan <- err
			alloc.claims.removeAt(i)
			i--
		}
	}
	alloc.checkPending()
}

// return true if the request is completed, false if pending
func (alloc *Allocator) tryAllocateFor(ident string, resultChan chan<- net.IP) bool {
	if addr := alloc.ourSpaceSet.AllocateFor(ident); addr != nil {
		resultChan <- addr
		return true
	} else { // out of space
		if alloc.inflight.findKind(msgSpaceRequest) < 0 { // is there already a request inflight
			if !alloc.requestSpace() {
				resultChan <- nil // Nobody to ask for more space, so fail now
				return true
			}
		}
	}
	return false
}

func (alloc *Allocator) checkPending() {
	i := 0
	for ; i < len(alloc.pending); i++ {
		if !alloc.tryAllocateFor(alloc.pending[i].Ident, alloc.pending[i].resultChan) {
			break
		}
	}
	alloc.pending = alloc.pending[i:]
}

// If somebody didn't come back to us, drop the record and we will ask again
// because we will still have the underlying need
func (alloc *Allocator) checkInflight(now time.Time) {
	for i := 0; i < len(alloc.inflight); i++ {
		if now.After(alloc.inflight[i].expires) {
			alloc.inflight.removeAt(i)
			i--
		}
	}
}

func (alloc *Allocator) considerOurPosition() (changed bool) {
	now := alloc.timeProvider.Now()
	switch alloc.state {
	case allocStateNeutral:
		alloc.discardOldLeaks()
		alloc.lookForDead(now)
		changed = alloc.reclaimLeaks(now)
		alloc.lookForNewLeaks(now)
		alloc.checkInflight(now)
		alloc.checkClaims()
	}
	return
}

func (alloc *Allocator) moveToState(newState int) {
	alloc.state = newState
}

func (alloc *Allocator) electLeaderIfNecessary() {
	if alloc.state != allocStateLeaderless {
		return
	}
	lg.Debug.Println("Time to look for a leader")
	// If anyone is already managing some space, then we don't need to elect a leader
	highest := alloc.ourUID
	for uid, spaceset := range alloc.peerInfo {
		if !spaceset.Empty() {
			// If anyone is already managing some space, then we don't need to elect a leader
			lg.Debug.Println("Peer", spaceset.PeerName(), "has some space; someone must have given it to her")
			alloc.moveToState(allocStateNeutral)
			return
		}
		if uid > highest {
			highest = uid
		}
	}
	lg.Debug.Println("Elected leader:", highest)
	// The peer with the highest name is the leader
	if highest == alloc.ourUID {
		lg.Info.Printf("I was elected leader of the universe %+v", alloc.universe)
		// I'm the winner; take control of the whole universe
		alloc.manageSpace(alloc.universe.Start, alloc.universe.Size)
		alloc.moveToState(allocStateNeutral)
		alloc.checkClaims()
	}
}

func (alloc *Allocator) sendRequest(dest router.PeerName, kind byte, space *MinSpace) {
	var msg []byte
	if space == nil {
		msg = router.Concat([]byte{kind}, encode(alloc.ourSpaceSet))
	} else {
		msg = router.Concat([]byte{kind}, GobEncode(space, 1, alloc.ourSpaceSet))
	}
	alloc.gossip.GossipUnicast(dest, msg)
	req := &request{dest, kind, space, alloc.timeProvider.Now().Add(GossipReqTimeout)}
	alloc.inflight = append(alloc.inflight, req)
}

func (alloc *Allocator) sendReply(dest router.PeerName, kind byte, space Space) {
	msg := router.Concat([]byte{kind}, GobEncode(space, 1, alloc.ourSpaceSet))
	alloc.gossip.GossipUnicast(dest, msg)
}

func (alloc *Allocator) requestSpace() bool {
	var best SpaceSet = nil
	var bestNum int = 0
	for _, spaceset := range alloc.peerInfo {
		if spaceset != alloc.ourSpaceSet && spaceset.HasFreeAddresses() && !spaceset.MaybeDead() {
			if num := alloc.ourSpaceSet.NumSpacesMergeable(spaceset, &alloc.universe); num > bestNum || best == nil {
				bestNum = num
				best = spaceset
			}
		}
	}
	if best != nil {
		lg.Debug.Println("Decided to ask peer", best.PeerName(), "for space:", best)
		alloc.sendRequest(best.PeerName(), msgSpaceRequest, nil)
		return true
	} else {
		lg.Debug.Println("Nobody available to ask for space")
		return false
	}
}

func (alloc *Allocator) handleSpaceRequest(sender router.PeerName, msg []byte) error {
	lg.Debug.Println("Received space request from", sender)
	if _, err := alloc.decodeUpdate(msg); err != nil {
		return err
	}

	if space, ok := alloc.ourSpaceSet.GiveUpSpace(); ok {
		lg.Debug.Println("Decided to give  peer", sender, "space", space)
		alloc.sendReply(sender, msgSpaceDonate, space)
	}
	return nil
}

func (alloc *Allocator) handleSpaceClaim(sender router.PeerName, msg []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(msg))
	var spaceClaimed MinSpace
	if err := decoder.Decode(&spaceClaimed); err != nil {
		return err
	}
	lg.Debug.Println("Received space claim from", sender, "for ", spaceClaimed)
	if _, err := alloc.decodeFromDecoder(decoder); err != nil {
		return err
	}
	if alloc.ourSpaceSet.GiveUpSpecificSpace(&spaceClaimed) {
		lg.Debug.Println("Giving peer", sender, "space", spaceClaimed)
		alloc.sendReply(sender, msgSpaceDonate, &spaceClaimed)
	} else {
		lg.Debug.Println("Claim refused - space occupied", spaceClaimed)
		alloc.sendReply(sender, msgSpaceClaimRefused, &spaceClaimed)
	}

	return nil
}

func (alloc *Allocator) handleSpaceDonate(sender router.PeerName, msg []byte) error {
	reader := bytes.NewReader(msg)
	decoder := gob.NewDecoder(reader)
	var donation MinSpace
	if err := decoder.Decode(&donation); err != nil {
		return err
	}
	pos := alloc.inflight.find(sender, &donation)
	if pos < 0 {
		lg.Error.Println("Not expecting to receive space donation from", sender, alloc.inflight[0].dest)
		return nil // not a severe enough error to shut down the connection
	}
	lg.Debug.Println("Received space donation: sender", sender, "space", donation)
	// Message is concluded by an update of state of the sender
	if _, err := alloc.decodeFromDecoder(decoder); err != nil {
		return err
	}
	if owner := alloc.spaceOwner(&donation); owner != 0 {
		lg.Error.Printf("Space donated: %+v is already owned by UID %d\n%+v", donation, owner, alloc.peerInfo[owner])
		return nil
	}
	alloc.ourSpaceSet.AddSpace(NewSpace(donation.Start, donation.Size))
	alloc.inflight.removeAt(pos)
	alloc.checkClaims()
	alloc.moveToState(allocStateNeutral)
	return nil
}

func (alloc *Allocator) handleSpaceClaimRefused(sender router.PeerName, msg []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(msg))
	var claim MinSpace
	if err := decoder.Decode(&claim); err != nil {
		return err
	}
	lg.Debug.Println("Received space claim refused: sender", sender, "space", claim)
	// Message is concluded by an update of state of the sender
	if _, err := alloc.decodeFromDecoder(decoder); err != nil {
		return err
	}
	alloc.inflight.remove(sender, &claim)
	for i := 0; i < len(alloc.claims); i++ {
		if claim.Contains(alloc.claims[i].IP) {
			alloc.claims[i].resultChan <- errors.New("IP address owned by" + sender.String())
			alloc.claims.removeAt(i)
			i--
		}
	}
	return nil
}

// Claim an address that we think we should own
func (alloc *Allocator) handleClaim(ident string, addr net.IP, resultChan chan<- error) {
	testaddr := NewMinSpace(addr, 1)
	if !alloc.universe.Overlaps(testaddr) {
		// Address not within our universe; assume user knows what they are doing
		resultChan <- nil
		return
	}
	alloc.electLeaderIfNecessary()
	// See if it's already claimed
	if pos := alloc.claims.find(addr); pos >= 0 {
		if alloc.claims[pos].Ident == ident {
			resultChan <- nil // fixme - this implies the claim has succeeded
			return
		} else {
			resultChan <- errors.New("IP address already claimed by " + alloc.claims[pos].Ident)
			return
		}
	}
	if owner, err := alloc.checkClaim(ident, addr); err != nil {
		resultChan <- err
	} else if owner != alloc.ourUID {
		alloc.claims = append(alloc.claims, claim{resultChan, Allocation{ident, addr}})
	} else {
		resultChan <- nil
	}
}

func (alloc *Allocator) string() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Allocator state %d universe %s+%d", alloc.state, alloc.universe.Start, alloc.universe.Size))
	for _, spaceset := range alloc.peerInfo {
		buf.WriteByte('\n')
		buf.WriteString(spaceset.String())
	}
	for _, claim := range alloc.claims {
		buf.WriteString("\nClaim ")
		buf.WriteString(claim.String())
	}
	return buf.String()
}

func (alloc *Allocator) handleGossipCreate() []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	panicOnError(enc.Encode(len(alloc.peerInfo)))
	for _, spaceset := range alloc.peerInfo {
		panicOnError(spaceset.Encode(enc))
	}
	return buf.Bytes()
}

func (alloc *Allocator) handleGossipBroadcast(buf []byte) error {
	_, err := alloc.decodeUpdate(buf)
	if err != nil {
		return err
	}
	alloc.considerOurPosition()
	return nil
}

// merge in state and return a buffer encoding those PeerSpaces which are newer
// than what we had previously, or nil if none were newer
func (alloc *Allocator) handleGossipReceived(buf []byte) ([]byte, error) {
	newerPeerSpaces, err := alloc.decodeUpdate(buf)
	if err != nil {
		return nil, err
	}
	alloc.considerOurPosition()
	if len(newerPeerSpaces) == 0 {
		return nil, nil
	} else {
		buf := new(bytes.Buffer)
		enc := gob.NewEncoder(buf)
		panicOnError(enc.Encode(len(newerPeerSpaces)))
		for _, spaceset := range newerPeerSpaces {
			panicOnError(spaceset.Encode(enc))
		}
		return buf.Bytes(), nil
	}
}

func (alloc *Allocator) handleDead(name router.PeerName, uid uint64) {
	entry, found := alloc.peerInfo[uid]
	if found {
		if peerEntry, ok := entry.(*PeerSpaceSet); ok &&
			!peerEntry.MaybeDead() {
			lg.Info.Printf("[allocator] Marking %s as maybe dead", entry.PeerName())
			peerEntry.MarkMaybeDead(true, alloc.timeProvider.Now())
		}
	}
}
