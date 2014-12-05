package sortinghat

import (
	"bytes"
	"encoding/gob"
	"github.com/zettio/weave/router"
	"log"
	"net"
	"sync"
)

const (
	MinSafeFreeAddresses = 5
	MaxAddressesToGiveUp = 256

	GossipSpaceRequest = iota
	GossipSpaceDonate

	AllocStateNeutral = iota
	AllocStateExpectingHole
)

type Allocator struct {
	sync.RWMutex
	ourName     router.PeerName
	state       int
	universe    MinSpace
	gossip      router.GossipCommsProvider
	spacesets   map[router.PeerName]*PeerSpace
	ourSpaceSet *SpaceSet
}

func NewAllocator(ourName router.PeerName, gossip router.GossipCommsProvider, startAddr net.IP, universeSize int) *Allocator {
	return &Allocator{
		gossip:      gossip,
		ourName:     ourName,
		state:       AllocStateNeutral,
		universe:    MinSpace{Start: startAddr, Size: uint32(universeSize)},
		spacesets:   make(map[router.PeerName]*PeerSpace),
		ourSpaceSet: NewSpaceSet(ourName),
	}
}

func (alloc *Allocator) ManageSpace(startAddr net.IP, poolSize int) {
	alloc.ourSpaceSet.AddSpace(NewSpace(startAddr, uint32(poolSize)))
}

func (alloc *Allocator) Encode() ([]byte, error) {
	alloc.RLock()
	defer alloc.RUnlock()
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(1); err != nil {
		return nil, err
	}
	if err := alloc.ourSpaceSet.Encode(enc); err != nil {
		return nil, err
	}
	// Question: Do I want to encode the PeerSpaces - other people's space-sets?
	return buf.Bytes(), nil
}

func (alloc *Allocator) DecodeUpdate(update []byte) error {
	alloc.Lock()
	defer alloc.Unlock()
	reader := bytes.NewReader(update)
	decoder := gob.NewDecoder(reader)
	var numSpaceSets int
	if err := decoder.Decode(&numSpaceSets); err != nil {
		return err
	}
	for i := 0; i < numSpaceSets; i++ {
		newSpaceset := new(PeerSpace)
		if err := newSpaceset.Decode(decoder); err != nil {
			return err
		}
		// compare this received spaceset's version against the one we had prev.
		oldSpaceset, found := alloc.spacesets[newSpaceset.PeerName]
		if !found || newSpaceset.version > oldSpaceset.version {
			log.Println("Replacing", newSpaceset.PeerName, "data with newer version")
			alloc.spacesets[newSpaceset.PeerName] = newSpaceset
		}
	}
	return nil
}

func (alloc *Allocator) ConsiderOurPosition() {
	switch alloc.state {
	case AllocStateNeutral:
		if alloc.ourSpaceSet.NumFreeAddresses() < MinSafeFreeAddresses {
			alloc.requestSpace()
		}
	case AllocStateExpectingHole:
		// What?
	}
}

func (alloc *Allocator) requestSpace() {
	var best *PeerSpace = nil
	var bestNumFree uint32 = 0
	for _, spaceset := range alloc.spacesets {
		if num := spaceset.NumFreeAddresses(); num > bestNumFree {
			bestNumFree = num
			best = spaceset
		}
	}
	if best != nil {
		log.Println("Decided to ask peer", best.PeerName, "for space")
		myState, _ := alloc.Encode()
		msg := router.Concat([]byte{GossipSpaceRequest}, myState)
		alloc.gossip.GossipSendTo(best.PeerName, msg)
		alloc.state = AllocStateExpectingHole
	}
}

func (alloc *Allocator) handleSpaceRequest(sender router.PeerName, msg []byte) {
	log.Println("Received space request")
	alloc.DecodeUpdate(msg)

	if start, size, ok := alloc.ourSpaceSet.GiveUpSpace(); ok {
		myState, _ := alloc.Encode()
		size_encoding := intip4(size) // hack!
		msg := router.Concat([]byte{GossipSpaceDonate}, start, size_encoding, myState)
		alloc.gossip.GossipSendTo(sender, msg)
	}
}

func (alloc *Allocator) handleSpaceDonate(msg []byte) {
	log.Println("Received space donation")

	alloc.gossip.Gossip()
}

func (alloc *Allocator) AllocateFor(ident string) net.IP {
	return alloc.ourSpaceSet.AllocateFor(ident)
}

func (alloc *Allocator) Free(addr net.IP) error {
	return alloc.ourSpaceSet.Free(addr)
}

func (alloc *Allocator) String() string {
	return "something"
}

// GossipDelegate methods
func (alloc *Allocator) NotifyMsg(sender router.PeerName, msg []byte) {
	log.Printf("NotifyMsg from %s: %+v\n", sender, msg)
	switch msg[0] {
	case GossipSpaceRequest:
		alloc.handleSpaceRequest(sender, msg[1:])
	case GossipSpaceDonate:
		alloc.handleSpaceDonate(msg[1:])
	}
}

func (alloc *Allocator) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts: %d %d\n", overhead, limit)
	return nil
}

func (alloc *Allocator) LocalState(join bool) []byte {
	log.Printf("LocalState: %t\n", join)
	if buf, err := alloc.Encode(); err == nil {
		return buf
	} else {
		log.Println("Error", err)
	}
	return nil
}

func (alloc *Allocator) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState: %t %d bytes\n", join, len(buf))
	alloc.DecodeUpdate(buf)
	alloc.ConsiderOurPosition()
}
