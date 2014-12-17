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

type Allocator struct {
	sync.RWMutex
	ourName     router.PeerName
	ourUID      uint64
	state       int
	universe    MinSpace // all the addresses that could be allocated
	gossip      router.GossipCommsProvider
	peerInfo    map[uint64]SpaceSet // indexed by peer UID
	ourSpaceSet *MutableSpaceSet
	maxAge      time.Duration
}

func NewAllocator(ourName router.PeerName, ourUID uint64, gossip router.GossipCommsProvider, startAddr net.IP, universeSize int) *Allocator {
	alloc := &Allocator{
		gossip:      gossip,
		ourName:     ourName,
		ourUID:      ourUID,
		state:       allocStateLeaderless,
		universe:    MinSpace{Start: startAddr, Size: uint32(universeSize)},
		peerInfo:    make(map[uint64]SpaceSet),
		ourSpaceSet: NewSpaceSet(ourName, ourUID),
		maxAge:      10 * time.Minute,
	}
	alloc.peerInfo[ourUID] = alloc.ourSpaceSet
	time.AfterFunc(router.GossipWaitForLead, func() { alloc.ElectLeader() })
	return alloc
}

// NOTE: exposed functions (start with uppercase) take a lock;
// internal functions never take a lock and never call an exposed function.
// Go's locks are not re-entrant

// Only called when testing or if we are elected leader
func (alloc *Allocator) manageSpace(startAddr net.IP, poolSize uint32) {
	alloc.ourSpaceSet.AddSpace(NewSpace(startAddr, poolSize))
	alloc.state = allocStateNeutral
}

func (alloc *Allocator) encode(includePeers bool) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	num := 1
	if includePeers {
		num = len(alloc.peerInfo)
	}
	if err := enc.Encode(num); err != nil {
		return nil, err
	}
	if includePeers {
		for _, spaceset := range alloc.peerInfo {
			spaceset.Encode(enc)
		}
	} else {
		alloc.ourSpaceSet.Encode(enc)
	}
	return buf.Bytes(), nil
}

// Unpack the supplied buffer which is encoded as per encode() above.
// return a slice of MinSpace containing those PeerSpaces which are newer
// than what we had previously
func (alloc *Allocator) decodeUpdate(update []byte) ([]*PeerSpace, error) {
	reader := bytes.NewReader(update)
	decoder := gob.NewDecoder(reader)
	var numSpaceSets int
	if err := decoder.Decode(&numSpaceSets); err != nil {
		return nil, err
	}
	ret := make([]*PeerSpace, 0)
	now := time.Now()
	for i := 0; i < numSpaceSets; i++ {
		newSpaceset := new(PeerSpace)
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
			if alloc.state == allocStateLeaderless {
				alloc.state = allocStateNeutral
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
	switch alloc.state {
	case allocStateNeutral:
		// Should we ask for some space?
		if alloc.ourSpaceSet.NumFreeAddresses() < MinSafeFreeAddresses {
			alloc.requestSpace()
		}
		// Should we time-out any of our peers?
		now := time.Now()
		for _, entry := range alloc.peerInfo {
			if now.After(entry.LastSeen().Add(alloc.maxAge)) {
				lg.Debug.Printf("Gossip Peer %s timed out; last seen %v", entry.PeerName(), entry.LastSeen())
				// FIXME: do something?
			}
		}
		// Look for leaked reservations
	case allocStateExpectingDonation:
		// What?
	case allocStateLeaderless:
		// Can't do anything in this state - waiting for timeout
	}
}

func (alloc *Allocator) haveLeader() {
}

func (alloc *Allocator) ElectLeader() {
	lg.Debug.Println("Time to look for a leader")
	alloc.Lock()
	defer alloc.Unlock()
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
		alloc.gossip.GossipBroadcast(alloc.localState())
	} else {
		// We expect the other guy to take control, but if he doesn't, try again.
		time.AfterFunc(router.GossipWaitForLead, func() { alloc.ElectLeader() })
	}
}

func (alloc *Allocator) requestSpace() {
	var best SpaceSet = nil
	var bestNumFree uint32 = 0
	for _, spaceset := range alloc.peerInfo {
		if num := spaceset.NumFreeAddresses(); num > bestNumFree {
			bestNumFree = num
			best = spaceset
		}
	}
	if best != nil {
		lg.Debug.Println("Decided to ask peer", best.PeerName, "for space")
		myState, _ := alloc.encode(false)
		msg := router.Concat([]byte{gossipSpaceRequest}, myState)
		alloc.gossip.GossipSendTo(best.PeerName(), msg)
		alloc.state = allocStateExpectingDonation
	}
}

func (alloc *Allocator) handleSpaceRequest(sender router.PeerName, msg []byte) {
	lg.Debug.Println("Received space request from", sender)
	alloc.decodeUpdate(msg)

	if start, size, ok := alloc.ourSpaceSet.GiveUpSpace(); ok {
		lg.Debug.Println("Decided to give  peer", sender, "space from", start, "size", size)
		myState, _ := alloc.encode(false)
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
		alloc.state = allocStateNeutral
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
	if buf, err := alloc.encode(false); err == nil {
		return buf
	} else {
		lg.Error.Println("Error", err)
	}
	return nil
}

func (alloc *Allocator) GlobalState() []byte {
	alloc.Lock()
	defer alloc.Unlock()
	if buf, err := alloc.encode(true); err == nil {
		return buf
	} else {
		lg.Error.Println("Error", err)
	}
	return nil
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
