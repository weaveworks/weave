package sortinghat

import (
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/zettio/weave/router"
	"net"
	"sync"
)

type PeerSpace struct {
	router.PeerName
	version uint64
	spaces  []SpaceInfo
	sync.RWMutex
}

func NewPeerSpace(pn router.PeerName) *PeerSpace {
	return &PeerSpace{PeerName: pn}
}

func (s *PeerSpace) AddSpace(space SpaceInfo) {
	s.Lock()
	defer s.Unlock()
	s.spaces = append(s.spaces, space)
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
	s.spaces = make([]SpaceInfo, numSpaces)
	for i := 0; i < numSpaces; i++ {
		s.spaces[i] = new(MinSpace)
		if err := decoder.Decode(s.spaces[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *PeerSpace) String() string {
	s.RLock()
	defer s.RUnlock()
	return fmt.Sprint("PeerSpace ", s.PeerName, " (v", s.version, ") (spaces: ", len(s.spaces), ") (1st: ", s.spaces[0], ")")
}

func (s *PeerSpace) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	var freeAddresses uint32 = 0
	for _, space := range s.spaces {
		freeAddresses += space.NumFreeAddresses()
	}
	return freeAddresses
}

func (s *PeerSpace) AllocateFor(ident string) net.IP {
	s.Lock()
	defer s.Unlock()
	// TODO: Optimize; perhaps cache last-used space
	for _, space := range s.spaces {
		if ret := space.(*Space).AllocateFor(ident); ret != nil {
			return ret
		}
	}
	return nil
}

func (s *PeerSpace) Free(addr net.IP) error {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		if space.(*Space).Free(addr) {
			return nil
		}
	}
	return errors.New("Attempt to free IP address not in range")
}
