package ipam

import (
	"bytes"
	"encoding/gob"
	"fmt"
	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	"net"
	"sort"
	"sync"
	"time"
)

const (
	MinSafeFreeAddresses = 5
	MaxAddressesToGiveUp = 256
)
const (
	gossipSpaceRequest = iota
	gossipSpaceDonate
)
const (
	allocStateNeutral = iota
	allocStateExpectingDonation
	allocStateLeaderless // Need to elect a leader
)

// To allow time itself to be stubbed out for testing
type timeProvider interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func())
}

type Allocator struct {
	sync.RWMutex
	ourName     router.PeerName
	ourUID      uint64
	state       int
	stateExpire time.Time
	universe    MinSpace // all the addresses that could be allocated
	gossip      router.Gossip
	peerInfo    map[uint64]SpaceSet // indexed by peer UID
	ourSpaceSet *MutableSpaceSet
	leaked      map[time.Time]Space
	maxAge      time.Duration
	timeProvider
}

type defaultTime struct {
}

func (defaultTime) Now() time.Time { return time.Now() }

func (defaultTime) AfterFunc(d time.Duration, f func()) {
	time.AfterFunc(d, f)
}

func NewAllocator(ourName router.PeerName, ourUID uint64, startAddr net.IP, universeSize int) *Allocator {
	alloc := &Allocator{
		ourName:      ourName,
		ourUID:       ourUID,
		state:        allocStateLeaderless,
		universe:     MinSpace{Start: startAddr, Size: uint32(universeSize)},
		peerInfo:     make(map[uint64]SpaceSet),
		ourSpaceSet:  NewSpaceSet(ourName, ourUID),
		leaked:       make(map[time.Time]Space),
		maxAge:       10 * time.Second,
		timeProvider: defaultTime{},
	}
	alloc.peerInfo[ourUID] = alloc.ourSpaceSet
	return alloc
}

func (alloc *Allocator) SetGossip(gossip router.Gossip) {
	alloc.gossip = gossip
}

func (alloc *Allocator) Start() {
	alloc.moveToState(allocStateLeaderless, router.GossipWaitForLead)
	go alloc.queryLoop()
}

func (alloc *Allocator) startForTesting() {
	alloc.moveToState(allocStateLeaderless, router.GossipWaitForLead)
}

// NOTE: exposed functions (start with uppercase) take a lock;
// internal functions never take a lock and never call an exposed function.
// Go's locks are not re-entrant

func (alloc *Allocator) manageSpace(startAddr net.IP, poolSize uint32) {
	alloc.ourSpaceSet.AddSpace(NewSpace(startAddr, poolSize))
}

// We shouldn't ever get any errors on *encoding*, but if we do, this will make sure we get to hear about them.
func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func encode(spaceset SpaceSet) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	panicOnError(enc.Encode(1))
	panicOnError(spaceset.Encode(enc))
	return buf.Bytes()
}

// Unpack the supplied buffer which is encoded as per encode() above.
// return a slice of MinSpace containing those PeerSpaces which are newer
// than what we had previously
func (alloc *Allocator) decodeUpdate(update []byte) ([]*PeerSpaceSet, error) {
	reader := bytes.NewReader(update)
	decoder := gob.NewDecoder(reader)
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
				continue
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

func (alloc *Allocator) spaceOwner(space *MinSpace) uint64 {
	for uid, spaceset := range alloc.peerInfo {
		if spaceset.Overlaps(space) {
			return uid
		}
	}
	return 0
}

func (alloc *Allocator) lookForOverlaps() (ret bool) {
	ret = false
	allSpaces := make([]Space, 0)
	for _, peerSpaceSet := range alloc.peerInfo {
		peerSpaceSet.ForEachSpace(func(space Space) {
			allSpaces = append(allSpaces, space)
		})
	}
	sort.Sort(SpaceByStart(allSpaces))
	for i := 0; i < len(allSpaces)-1; i++ {
		if allSpaces[i].Overlaps(allSpaces[i+1]) {
			lg.Error.Printf("Spaces overlap: %s and %s", allSpaces[i], allSpaces[i+1])
			ret = true
		}
	}
	return
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
			lg.Info.Println(allSpace.describe("New leaked spaces:"))
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
	limit := now.Add(-router.GossipDeadTimeout)
	for age, leak := range alloc.leaked {
		if age.Before(limit) {
			for _, space := range alloc.ourSpaceSet.spaces {
				if space.IsHeirTo(leak.GetMinSpace(), alloc.universe.GetMinSpace()) {
					lg.Info.Printf("Reclaiming leak %+v heir %+v", leak, space)
					delete(alloc.leaked, age)
					alloc.manageSpace(leak.GetStart(), leak.GetSize())
					changed = true
					break
				}
			}
		}
	}
	return
}

func (alloc *Allocator) considerOurPosition() {
	if alloc.gossip == nil {
		return // Can't do anything.
	}
	now := alloc.timeProvider.Now()
	switch alloc.state {
	case allocStateNeutral:
		// Should we ask for some space?
		if alloc.ourSpaceSet.NumFreeAddresses() < MinSafeFreeAddresses {
			alloc.requestSpace()
		}
		alloc.discardOldLeaks()
		changed := alloc.reclaimLeaks(now)
		alloc.lookForNewLeaks(now)
		alloc.lookForOverlaps()
		if changed {
			alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
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
		alloc.manageSpace(alloc.universe.Start, alloc.universe.Size)
		alloc.moveToState(allocStateNeutral, 0)
		alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
	} else {
		// We expect the other guy to take control, but if he doesn't, try again.
		alloc.moveToState(allocStateLeaderless, router.GossipWaitForLead)
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
		myState := encode(alloc.ourSpaceSet)
		msg := router.Concat([]byte{gossipSpaceRequest}, myState)
		alloc.gossip.GossipUnicast(best.PeerName(), msg)
		alloc.moveToState(allocStateExpectingDonation, router.GossipReqTimeout)
	} else {
		lg.Debug.Println("Nobody available to ask for space")
	}
}

func (alloc *Allocator) handleSpaceRequest(sender router.PeerName, msg []byte) {
	lg.Debug.Println("Received space request from", sender)
	alloc.decodeUpdate(msg)

	if start, size, ok := alloc.ourSpaceSet.GiveUpSpace(); ok {
		lg.Debug.Println("Decided to give  peer", sender, "space from", start, "size", size)
		myState := encode(alloc.ourSpaceSet)
		size_encoding := intip4(size) // hack!
		msg := router.Concat([]byte{gossipSpaceDonate}, start.To4(), size_encoding, myState)
		alloc.gossip.GossipUnicast(sender, msg)
	}
}

func (alloc *Allocator) handleSpaceDonate(sender router.PeerName, msg []byte) {
	var start net.IP = msg[0:4]
	size := ip4int(msg[4:8])
	lg.Debug.Println("Received space donation: sender", sender, "start", start, "size", size)
	switch alloc.state {
	case allocStateNeutral:
		lg.Error.Println("Not expecting to receive space donation from", sender)
	case allocStateExpectingDonation:
		// Message is concluded by an update of state of the sender
		if _, err := alloc.decodeUpdate(msg[8:]); err != nil {
			lg.Error.Println("Error decoding update", err)
			return
		}
		newSpace := NewSpace(start, size)
		if owner := alloc.spaceOwner(newSpace.GetMinSpace()); owner != 0 {
			lg.Error.Printf("Space donated: %+v is already owned by UID %d\n%+v", newSpace, owner, alloc.peerInfo[owner])
			return
		}
		alloc.ourSpaceSet.AddSpace(newSpace)
		alloc.moveToState(allocStateNeutral, 0)
		alloc.gossip.GossipBroadcast(encode(alloc.ourSpaceSet))
	}
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
	return buf.String()
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
func (alloc *Allocator) OnGossipUnicast(sender router.PeerName, msg []byte) {
	lg.Debug.Printf("OnGossipUnicast from %s: %d bytes\n", sender, len(msg))
	alloc.Lock()
	defer alloc.Unlock()
	switch msg[0] {
	case gossipSpaceRequest:
		alloc.handleSpaceRequest(sender, msg[1:])
	case gossipSpaceDonate:
		alloc.handleSpaceDonate(sender, msg[1:])
	}
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

func (alloc *Allocator) OnGossipBroadcast(buf []byte) {
	lg.Debug.Printf("OnGossipBroadcast: %d bytes\n", len(buf))
	alloc.Lock()
	defer alloc.Unlock()
	_, err := alloc.decodeUpdate(buf)
	if err != nil {
		lg.Error.Println("Error decoding update", err)
		return
	}
	alloc.considerOurPosition()
}

// merge in state and return a buffer encoding those PeerSpaces which are newer
// than what we had previously, or nil if none were newer
func (alloc *Allocator) OnGossip(buf []byte) []byte {
	lg.Debug.Printf("Allocator.OnGossip: %d bytes\n", len(buf))
	alloc.Lock()
	defer alloc.Unlock()
	newerPeerSpaces, err := alloc.decodeUpdate(buf)
	if err != nil {
		lg.Error.Println("Error decoding update", err)
		return nil
	}
	alloc.considerOurPosition()
	if len(newerPeerSpaces) == 0 {
		return nil
	} else {
		buf := new(bytes.Buffer)
		enc := gob.NewEncoder(buf)
		panicOnError(enc.Encode(len(newerPeerSpaces)))
		for _, spaceset := range newerPeerSpaces {
			panicOnError(spaceset.Encode(enc))
		}
		return buf.Bytes()
	}
}

func (alloc *Allocator) OnAlive(uid uint64) {
	// If it's new to us, nothing to do.
	// If we previously believed it to be dead, need to figure that case out.
}

func (alloc *Allocator) OnDead(uid uint64) {
	alloc.Lock()
	defer alloc.Unlock()
	entry, found := alloc.peerInfo[uid]
	if found {
		if peerEntry, ok := entry.(*PeerSpaceSet); ok &&
			!peerEntry.IsTombstone() {
			lg.Info.Printf("Allocator: Marking %s as dead", entry.PeerName())
			peerEntry.MakeTombstone()
			// Can't run this synchronously or we deadlock
			go alloc.gossip.GossipBroadcast(encode(entry))
		}
	}
}
