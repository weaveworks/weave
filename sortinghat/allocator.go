package sortinghat

import (
	"bytes"
	"encoding/gob"
	"fmt"
	lg "github.com/zettio/weave/logging"
	"github.com/zettio/weave/router"
	"net"
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
	gossip      router.GossipCommsProvider
	peerInfo    map[uint64]SpaceSet // indexed by peer UID
	ourSpaceSet *MutableSpaceSet
	maxAge      time.Duration
	timeProvider
}

type defaultTime struct {
}

func (defaultTime) Now() time.Time { return time.Now() }

func (defaultTime) AfterFunc(d time.Duration, f func()) {
	time.AfterFunc(d, f)
}

func NewAllocator(ourName router.PeerName, ourUID uint64, gossip router.GossipCommsProvider, startAddr net.IP, universeSize int) *Allocator {
	alloc := &Allocator{
		gossip:       gossip,
		ourName:      ourName,
		ourUID:       ourUID,
		state:        allocStateLeaderless,
		universe:     MinSpace{Start: startAddr, Size: uint32(universeSize)},
		peerInfo:     make(map[uint64]SpaceSet),
		ourSpaceSet:  NewSpaceSet(ourName, ourUID),
		maxAge:       10 * time.Second,
		timeProvider: defaultTime{},
	}
	alloc.peerInfo[ourUID] = alloc.ourSpaceSet
	return alloc
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

// Only called when testing or if we are elected leader
func (alloc *Allocator) manageSpace(startAddr net.IP, poolSize uint32) {
	alloc.ourSpaceSet.AddSpace(NewSpace(startAddr, poolSize))
	if alloc.state == allocStateLeaderless {
		alloc.state = allocStateNeutral
	}
}

func (alloc *Allocator) encode(spaceset SpaceSet) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(1)
	spaceset.Encode(enc)
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
	now := alloc.timeProvider.Now()
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
			}
			lg.Debug.Println("Replacing", newSpaceset.PeerName, "data with newer version", newSpaceset.version)
			alloc.peerInfo[newSpaceset.UID()] = newSpaceset
			if alloc.state == allocStateLeaderless && !newSpaceset.Empty() {
				alloc.moveToState(allocStateNeutral, 0)
			}
			ret = append(ret, newSpaceset)
		}
		alloc.peerInfo[newSpaceset.UID()].SetLastSeen(now)
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
		// Look for any peers we haven't heard from in a long time
		now := alloc.timeProvider.Now()
		alloc.ourSpaceSet.lastSeen = now
		for _, entry := range alloc.peerInfo {
			if now.After(entry.LastSeen().Add(alloc.maxAge)) {
				lg.Debug.Printf("Gossip Peer %s timed out; last seen %v", entry.PeerName(), entry.LastSeen())
				entry.(*PeerSpaceSet).MakeTombstone()
				alloc.gossip.GossipBroadcast(alloc.encode(entry))
			}
		}
		// Look for holes in the address space
		allSpace := NewSpaceSet(router.UnknownPeerName, 0)
		allSpace.AddSpace(NewSpace(alloc.universe.Start, alloc.universe.Size))
		for _, peerSpaceSet := range alloc.peerInfo {
			peerSpaceSet.ForEachSpace(func(space Space) {
				allSpace.Exclude(space)
			})
		}
		if !allSpace.Empty() {
			lg.Info.Printf("Leaked spaces: %s", allSpace)
		}
		// Look for leaked reservations that we are heir to
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
		alloc.gossip.GossipBroadcast(alloc.localState())
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
		lg.Debug.Println("Decided to ask peer", best.PeerName, "for space:", best)
		myState := alloc.encode(alloc.ourSpaceSet)
		msg := router.Concat([]byte{gossipSpaceRequest}, myState)
		alloc.gossip.GossipSendTo(best.PeerName(), msg)
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
		myState := alloc.encode(alloc.ourSpaceSet)
		size_encoding := intip4(size) // hack!
		msg := router.Concat([]byte{gossipSpaceDonate}, start.To4(), size_encoding, myState)
		alloc.gossip.GossipSendTo(sender, msg)
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
		alloc.gossip.GossipBroadcast(alloc.localState())
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
	buf.WriteString(fmt.Sprintf("Allocator state %d universe %+v\n", alloc.state, alloc.universe))
	for _, spaceset := range alloc.peerInfo {
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
func (alloc *Allocator) NotifyMsg(sender router.PeerName, msg []byte) {
	lg.Debug.Printf("NotifyMsg from %s: %d bytes\n", sender, len(msg))
	alloc.Lock()
	defer alloc.Unlock()
	switch msg[0] {
	case gossipSpaceRequest:
		alloc.handleSpaceRequest(sender, msg[1:])
	case gossipSpaceDonate:
		alloc.handleSpaceDonate(sender, msg[1:])
	}
	alloc.considerOurPosition()
}

func (alloc *Allocator) LocalState() []byte {
	alloc.Lock()
	defer alloc.Unlock()
	return alloc.localState()
}

func (alloc *Allocator) localState() []byte {
	lg.Debug.Println("localState")
	if buf := alloc.encode(alloc.ourSpaceSet); buf != nil {
		return buf
	} else {
		lg.Error.Println("Error encoding state")
	}
	return nil
}

func (alloc *Allocator) GlobalState() []byte {
	alloc.Lock()
	defer alloc.Unlock()
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(len(alloc.peerInfo))
	for _, spaceset := range alloc.peerInfo {
		spaceset.Encode(enc)
	}
	return buf.Bytes()
}

// merge in state and return a buffer encoding those PeerSpaces which are newer
// than what we had previously, or nil if none were newer
func (alloc *Allocator) MergeRemoteState(buf []byte) []byte {
	lg.Debug.Printf("MergeRemoteState: %d bytes\n", len(buf))
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
		if err := enc.Encode(len(newerPeerSpaces)); err != nil {
			lg.Error.Println("Error encoding update", err)
			return nil
		}
		for _, spaceset := range newerPeerSpaces {
			spaceset.Encode(enc)
		}
		return buf.Bytes()
	}
}
