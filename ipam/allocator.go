package ipam

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	"net"
	"sync"
	"time"
)

const (
	GossipReqTimeout     = 1 * time.Second
	GossipWaitForLead    = 10 * time.Second
	GossipDeadTimeout    = 10 * time.Second
	MinSafeFreeAddresses = 5
	MaxAddressesToGiveUp = 256
)
const (
	msgSpaceRequest = iota
	msgSpaceDonate
)
const (
	allocStateNeutral = iota
	allocStateExpectingDonation
	allocStateLeaderless
)

// To allow time itself to be stubbed out for testing
type timeProvider interface {
	Now() time.Time
}

type defaultTime struct{}

func (defaultTime) Now() time.Time { return time.Now() }

type Allocator struct {
	sync.RWMutex
	ourName     router.PeerName
	ourUID      uint64
	state       int
	stateExpire time.Time
	universeLen int
	universe    MinSpace // all the addresses that could be allocated
	gossip      router.Gossip
	peerInfo    map[uint64]SpaceSet // indexed by peer UID
	ourSpaceSet *OurSpaceSet
	pastLife    *PeerSpaceSet // Probably allocations from a previous incarnation
	leaked      map[time.Time]Space
	claims      []Allocation
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
	alloc.moveToState(allocStateLeaderless, GossipWaitForLead)
	go alloc.queryLoop()
}

func (alloc *Allocator) startForTesting() {
	alloc.moveToState(allocStateLeaderless, GossipWaitForLead)
}

// NOTE: Go's locks are not re-entrant, so we have some rules to avoid deadlock:
// exposed functions (start with uppercase) take a lock;
// internal functions never take a lock and never call an exposed function.

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
				alloc.moveToState(allocStateNeutral, 0)
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
				// fixme: should merge contiguous spaces
				alloc.leaked[now] = space
				break // can only store one space against each time
			}
		}
	}
}

func (alloc *Allocator) discardOldLeaks() {
	for age, leak := range alloc.leaked {
		for _, peerSpaceSet := range alloc.peerInfo {
			if peerSpaceSet.Overlaps(leak.GetMinSpace()) {
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
				if space.IsHeirTo(leak.GetMinSpace(), alloc.universe.GetMinSpace()) {
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
	lg.Debug.Println("checkClaim", addr, alloc.string())
	testaddr := NewMinSpace(addr, 1)
	if !alloc.universe.Overlaps(testaddr) {
		return 0, errors.New(fmt.Sprintf("Address %s is not within our universe %s", addr, alloc.universe.String()))
	}
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
		panic("reclaiming addresses from another owner not implemented") // TODO
		//return spaceSet.UID(), nil
	}
}

func (alloc *Allocator) checkClaims() {
	lg.Debug.Println("checkClaims:", alloc.string())
	for _, claim := range alloc.claims {
		_, err := alloc.checkClaim(claim.Ident, claim.IP)
		if err != nil {
			lg.Error.Println("checkClaims:", err)
		}
	}
}

func (alloc *Allocator) considerOurPosition() {
	if alloc.gossip == nil {
		return // Can't do anything.
	}
	now := alloc.timeProvider.Now()
	switch alloc.state {
	case allocStateNeutral:
		alloc.discardOldLeaks()
		alloc.lookForDead(now)
		changed := alloc.reclaimLeaks(now)
		alloc.lookForNewLeaks(now)
		alloc.checkClaims()
		if changed {
			alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
		} else if alloc.ourSpaceSet.NumFreeAddresses() < MinSafeFreeAddresses {
			alloc.requestSpace()
		}

	case allocStateExpectingDonation:
		// If nobody came back to us, ask again
		if now.After(alloc.stateExpire) {
			alloc.requestSpace()
		}
	case allocStateLeaderless:
		if now.After(alloc.stateExpire) {
			alloc.electLeader()
		}
	}
}

func (alloc *Allocator) moveToState(newState int, timeout time.Duration) {
	alloc.state = newState
	alloc.stateExpire = alloc.timeProvider.Now().Add(timeout)
}

func (alloc *Allocator) electLeader() {
	lg.Debug.Println("Time to look for a leader")
	// If anyone is already managing some space, then we don't need to elect a leader
	if !alloc.ourSpaceSet.Empty() {
		lg.Debug.Println("I have some space; someone must have given it to me")
		return
	}
	highest := alloc.ourUID
	for uid, spaceset := range alloc.peerInfo {
		if !spaceset.Empty() {
			lg.Debug.Println("Peer", spaceset.PeerName(), "has some space; someone must have given it to her")
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
		// But don't allocate the first and last addresses
		alloc.manageSpace(alloc.universe.Start, alloc.universe.Size)
		alloc.moveToState(allocStateNeutral, 0)
		alloc.checkClaims()
		alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
	} else {
		// We expect the other guy to take control, but if he doesn't, try again.
		alloc.moveToState(allocStateLeaderless, GossipWaitForLead)
	}
}

func (alloc *Allocator) requestSpace() {
	var best SpaceSet = nil
	var bestNumFree uint32 = 0
	for _, spaceset := range alloc.peerInfo {
		if num := spaceset.NumFreeAddresses(); spaceset != alloc.ourSpaceSet && num > bestNumFree {
			bestNumFree = num
			best = spaceset
		}
	}
	if best != nil {
		lg.Debug.Println("Decided to ask peer", best.PeerName(), "for space:", best)
		msg := router.Concat([]byte{msgSpaceRequest}, encode(alloc.ourSpaceSet))
		alloc.gossip.GossipUnicast(best.PeerName(), msg)
		alloc.moveToState(allocStateExpectingDonation, GossipReqTimeout)
	} else {
		lg.Debug.Println("Nobody available to ask for space")
	}
}

func (alloc *Allocator) handleSpaceRequest(sender router.PeerName, msg []byte) error {
	lg.Debug.Println("Received space request from", sender)
	if _, err := alloc.decodeUpdate(msg); err != nil {
		return err
	}

	if space, ok := alloc.ourSpaceSet.GiveUpSpace(); ok {
		lg.Debug.Println("Decided to give  peer", sender, "space", space)
		msg := router.Concat([]byte{msgSpaceDonate}, GobEncode(space, 1, alloc.ourSpaceSet))
		alloc.gossip.GossipUnicast(sender, msg)
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
	lg.Debug.Println("Received space donation: sender", sender, "space", donation)
	switch alloc.state {
	case allocStateNeutral:
		lg.Error.Println("Not expecting to receive space donation from", sender)
	case allocStateExpectingDonation:
		// Message is concluded by an update of state of the sender
		if _, err := alloc.decodeFromDecoder(decoder); err != nil {
			return err
		}
		if owner := alloc.spaceOwner(&donation); owner != 0 {
			return errors.New(fmt.Sprintf("Space donated: %+v is already owned by UID %d\n%+v", donation, owner, alloc.peerInfo[owner]))
		}
		alloc.ourSpaceSet.AddSpace(NewSpace(donation.Start, donation.Size))
		alloc.moveToState(allocStateNeutral, 0)
		alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
	}
	return nil
}

// Claim an address that we think we should own
func (alloc *Allocator) Claim(ident string, addr net.IP) error {
	lg.Info.Printf("Address %s claimed by %s", addr, ident)
	alloc.Lock()
	defer alloc.Unlock()
	if owner, err := alloc.checkClaim(ident, addr); err != nil {
		return err
	} else if owner == 0 {
		// That address is not currently owned; wait until someone claims it
		alloc.claims = append(alloc.claims, Allocation{ident, addr})
	}

	return nil
}

func (alloc *Allocator) AllocateFor(ident string) net.IP {
	alloc.Lock()
	defer alloc.Unlock()
	return alloc.ourSpaceSet.AllocateFor(ident)
}

func (alloc *Allocator) Free(addr net.IP) error {
	alloc.Lock()
	defer alloc.Unlock()
	return alloc.ourSpaceSet.Free(addr)
}

func (alloc *Allocator) String() string {
	alloc.RLock()
	defer alloc.RUnlock()
	return alloc.string()
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

func (alloc *Allocator) DeleteRecordsFor(ident string) error {
	alloc.Lock()
	defer alloc.Unlock()
	alloc.ourSpaceSet.DeleteRecordsFor(ident)
	return nil
}

// Actor (?)

func (alloc *Allocator) queryLoop() {
	gossipTimer := time.Tick(router.GossipInterval)
	for {
		select {
		case <-gossipTimer:
			alloc.Lock()
			alloc.considerOurPosition()
			alloc.Unlock()
		}
	}
}

// GossipDelegate methods
func (alloc *Allocator) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	lg.Debug.Printf("OnGossipUnicast from %s: %d bytes\n", sender, len(msg))
	alloc.Lock()
	defer alloc.Unlock()
	switch msg[0] {
	case msgSpaceRequest:
		alloc.handleSpaceRequest(sender, msg[1:])
	case msgSpaceDonate:
		return alloc.handleSpaceDonate(sender, msg[1:])
	}
	return nil
}

func (alloc *Allocator) Gossip() []byte {
	alloc.Lock()
	defer alloc.Unlock()
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	panicOnError(enc.Encode(len(alloc.peerInfo)))
	for _, spaceset := range alloc.peerInfo {
		panicOnError(spaceset.Encode(enc))
	}
	return buf.Bytes()
}

func (alloc *Allocator) OnGossipBroadcast(buf []byte) error {
	lg.Debug.Printf("OnGossipBroadcast: %d bytes\n", len(buf))
	alloc.Lock()
	defer alloc.Unlock()
	_, err := alloc.decodeUpdate(buf)
	if err != nil {
		return err
	}
	alloc.considerOurPosition()
	return nil
}

// merge in state and return a buffer encoding those PeerSpaces which are newer
// than what we had previously, or nil if none were newer
func (alloc *Allocator) OnGossip(buf []byte) ([]byte, error) {
	lg.Debug.Printf("Allocator.OnGossip: %d bytes\n", len(buf))
	alloc.Lock()
	defer alloc.Unlock()
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

func (alloc *Allocator) OnAlive(name router.PeerName, uid uint64) {
	// If it's new to us, nothing to do.
	// If we previously believed it to be dead, need to figure that case out.
}

func (alloc *Allocator) OnDead(name router.PeerName, uid uint64) {
	alloc.Lock()
	defer alloc.Unlock()
	entry, found := alloc.peerInfo[uid]
	if found {
		if peerEntry, ok := entry.(*PeerSpaceSet); ok &&
			!peerEntry.MaybeDead() {
			lg.Info.Printf("[allocator] Marking %s as maybe dead", entry.PeerName())
			peerEntry.MarkMaybeDead(true, alloc.timeProvider.Now())
		}
	}
}
