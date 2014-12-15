package sortinghat

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/zettio/weave/router"
	"sync"
)

// This represents someone else's space allocations. See also SpaceSet.
type PeerSpace struct {
	router.PeerName
	version uint64
	spaces  []*MinSpace
	sync.RWMutex
}

func NewPeerSpace(pn router.PeerName) *PeerSpace {
	return &PeerSpace{PeerName: pn}
}

func (s *PeerSpace) Encode(enc *gob.Encoder) error {
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

func (s *PeerSpace) Decode(decoder *gob.Decoder) error {
	s.Lock()
	defer s.Unlock()
	if err := decoder.Decode(&s.PeerName); err != nil {
		return err
	}
	if err := decoder.Decode(&s.version); err != nil {
		return err
	}
	var numSpaces int
	if err := decoder.Decode(&numSpaces); err != nil {
		return err
	}
	s.spaces = make([]*MinSpace, numSpaces)
	for i := 0; i < numSpaces; i++ {
		s.spaces[i] = new(MinSpace)
		if err := decoder.Decode(s.spaces[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *PeerSpace) String() string {
	var buf bytes.Buffer
	s.RLock()
	defer s.RUnlock()
	buf.WriteString(fmt.Sprint("PeerSpace ", s.PeerName, " (v", s.version, ")\n"))
	for _, space := range s.spaces {
		buf.WriteString(fmt.Sprintf("  %s\n", space.String()))
	}
	return buf.String()
}

func (s *PeerSpace) Empty() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.spaces) == 0
}

func (s *PeerSpace) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	var freeAddresses uint32 = 0
	for _, space := range s.spaces {
		freeAddresses += space.LargestFreeBlock()
	}
	return freeAddresses
}

func (s *PeerSpace) Overlaps(space *MinSpace) bool {
	s.RLock()
	defer s.RUnlock()
	for _, space2 := range s.spaces {
		if space.Overlaps(space2) {
			return true
		}
	}
	return false
}
