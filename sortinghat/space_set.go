package sortinghat

import (
	"encoding/gob"
	"errors"
	"github.com/zettio/weave/router"
	"net"
	"time"
)

type SpaceSet interface {
	Encode(enc *gob.Encoder) error
	Decode(decoder *gob.Decoder) error
	Empty() bool
	Version() uint64
	LastSeen() time.Time
	SetLastSeen(time.Time)
	PeerName() router.PeerName
	UID() uint64
	NumFreeAddresses() uint32
	Overlaps(space *MinSpace) bool
	String() string
}

// Represents our own space allocations.  See also PeerSpace.
type MutableSpaceSet struct {
	PeerSpace
}

func NewSpaceSet(pn router.PeerName, uid uint64) *MutableSpaceSet {
	return &MutableSpaceSet{PeerSpace{peerName: pn, uid: uid, lastSeen: time.Now()}}
}

func (s *MutableSpaceSet) AddSpace(space *MutableSpace) {
	s.Lock()
	defer s.Unlock()
	s.spaces = append(s.spaces, space)
	s.version++
}

func (s *MutableSpaceSet) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	// TODO: Optimize; perhaps maintain the count in allocate and free
	var freeAddresses uint32 = 0
	for _, space := range s.spaces {
		freeAddresses += space.(*MutableSpace).NumFreeAddresses()
	}
	return freeAddresses
}

// Give up some space because one of our peers has asked for it.
// Pick some large reasonably-sized chunk.
func (s *MutableSpaceSet) GiveUpSpace() (start net.IP, size uint32, ok bool) {
	totalFreeAddresses := s.NumFreeAddresses()
	if totalFreeAddresses < MinSafeFreeAddresses {
		return nil, 0, false
	}
	var bestFree uint32 = 0
	var bestSpace *MutableSpace = nil
	for _, space := range s.spaces {
		mSpace := space.(*MutableSpace)
		numFree := mSpace.NumFreeAddresses()
		if numFree > bestFree {
			bestFree = numFree
			bestSpace = mSpace
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

func (s *MutableSpaceSet) AllocateFor(ident string) net.IP {
	s.Lock()
	defer s.Unlock()
	// TODO: Optimize; perhaps cache last-used space
	for _, space := range s.spaces {
		if ret := space.(*MutableSpace).AllocateFor(ident); ret != nil {
			return ret
		}
	}
	return nil
}

func (s *MutableSpaceSet) Free(addr net.IP) error {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		if space.(*MutableSpace).Free(addr) {
			return nil
		}
	}
	return errors.New("Attempt to free IP address not in range")
}
