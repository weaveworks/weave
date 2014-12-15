package sortinghat

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/zettio/weave/router"
	"net"
	"sync"
)

// Represents our own space allocations.  See also PeerSpace.
type SpaceSet struct {
	router.PeerName
	version uint64
	spaces  []*Space
	sync.RWMutex
}

func NewSpaceSet(pn router.PeerName) *SpaceSet {
	return &SpaceSet{PeerName: pn}
}

func (s *SpaceSet) AddSpace(space *Space) {
	s.Lock()
	defer s.Unlock()
	s.spaces = append(s.spaces, space)
	s.version++
}

// We've found (via gossip) some information about what we should be managing
func (s *SpaceSet) MergeFrom(peerSpace *PeerSpace) {
	s.Lock()
	defer s.Unlock()
	s.spaces = make([]*Space, len(peerSpace.spaces))
	// Simple implementation for now: throw away what we know
	for i, space := range peerSpace.spaces {
		s.spaces[i] = NewSpace(space.Start, space.Size)
	}
	s.version = peerSpace.version
}

func (s *SpaceSet) Encode(enc *gob.Encoder) error {
	s.RLock()
	defer s.RUnlock()
	if err := enc.Encode(s.PeerName); err != nil {
		return err
	}
	if err := enc.Encode(s.version); err != nil {
		return err
	}
	if err := enc.Encode(len(s.spaces)); err != nil {
		return err
	}
	for _, space := range s.spaces {
		if err := enc.Encode(space.GetMinSpace()); err != nil {
			return err
		}
	}
	return nil
}

func (s *SpaceSet) String() string {
	var buf bytes.Buffer
	s.RLock()
	defer s.RUnlock()
	buf.WriteString(fmt.Sprint("SpaceSet ", s.PeerName, " (v", s.version, ")\n"))
	for _, space := range s.spaces {
		buf.WriteString(fmt.Sprintf("  %s\n", space.String()))
	}
	return buf.String()
}

func (s *SpaceSet) Empty() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.spaces) == 0
}

func (s *SpaceSet) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	// TODO: Optimize; perhaps maintain the count in allocate and free
	var freeAddresses uint32 = 0
	for _, space := range s.spaces {
		freeAddresses += space.NumFreeAddresses()
	}
	return freeAddresses
}

// Give up some space because one of our peers has asked for it.
// Pick some large reasonably-sized chunk.
func (s *SpaceSet) GiveUpSpace() (start net.IP, size uint32, ok bool) {
	totalFreeAddresses := s.NumFreeAddresses()
	if totalFreeAddresses < MinSafeFreeAddresses {
		return nil, 0, false
	}
	var bestFree uint32 = 0
	var bestSpace *Space = nil
	for _, space := range s.spaces {
		numFree := space.NumFreeAddresses()
		if numFree > bestFree {
			bestFree = numFree
			bestSpace = space
			if numFree >= MaxAddressesToGiveUp {
				break
			}
		}
	}
	if bestSpace != nil {
		var spaceToGiveUp uint32 = MaxAddressesToGiveUp
		if spaceToGiveUp > bestFree {
			spaceToGiveUp = bestFree
		}
		// Don't give away more than half of our available addresses
		if spaceToGiveUp > totalFreeAddresses/2 {
			spaceToGiveUp = totalFreeAddresses / 2
		}
		bestSpace.Size -= spaceToGiveUp
		s.version++
		return add(bestSpace.Start, bestSpace.Size), spaceToGiveUp, true
	}
	return nil, 0, false
}

func (s *SpaceSet) AllocateFor(ident string) net.IP {
	s.Lock()
	defer s.Unlock()
	// TODO: Optimize; perhaps cache last-used space
	for _, space := range s.spaces {
		if ret := space.AllocateFor(ident); ret != nil {
			return ret
		}
	}
	return nil
}

func (s *SpaceSet) Free(addr net.IP) error {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		if space.Free(addr) {
			return nil
		}
	}
	return errors.New("Attempt to free IP address not in range")
}

func (s *SpaceSet) Overlaps(space *MinSpace) bool {
	s.RLock()
	defer s.RUnlock()
	for _, space2 := range s.spaces {
		if space.Overlaps(&space2.MinSpace) {
			return true
		}
	}
	return false
}
