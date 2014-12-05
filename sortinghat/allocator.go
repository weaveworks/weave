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
)

type Allocator struct {
	sync.RWMutex
	ourName     router.PeerName
	gossip      router.GossipCommsProvider
	spacesets   map[router.PeerName]*PeerSpace
	ourSpaceSet *SpaceSet
}

func NewAllocator(ourName router.PeerName, gossip router.GossipCommsProvider, startAddr net.IP, poolSize int) *Allocator {
	spaceSet := NewSpaceSet(ourName)
	if poolSize > 0 {
		spaceSet.AddSpace(NewSpace(startAddr, uint32(poolSize)))
	}

	return &Allocator{
		gossip:      gossip,
		ourName:     ourName,
		spacesets:   make(map[router.PeerName]*PeerSpace),
		ourSpaceSet: spaceSet,
	}
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
	// Rule: if we have no IP space, pick the peer with the most available space and request some
	alloc.RLock()
	defer alloc.RUnlock()
	if alloc.ourSpaceSet.NumFreeAddresses() < MinSafeFreeAddresses {
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
		}
	}
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
func (alloc *Allocator) NotifyMsg(msg []byte) {
	log.Printf("NotifyMsg: %+v\n", msg)
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
