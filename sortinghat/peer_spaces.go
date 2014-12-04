package sortinghat

import (
	"bytes"
	"encoding/gob"
	"github.com/zettio/weave/router"
	"log"
	"sync"
)

type PeerSpaceSet struct {
	ourName   router.PeerName
	spacesets map[router.PeerName]*PeerSpace
	sync.RWMutex
}

func NewPeerSpaceSet(pn router.PeerName, space *PeerSpace) *PeerSpaceSet {
	return &PeerSpaceSet{
		ourName:   pn,
		spacesets: map[router.PeerName]*PeerSpace{pn: space},
	}
}

func (s *PeerSpaceSet) Encode() ([]byte, error) {
	s.RLock()
	defer s.RUnlock()
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(len(s.spacesets)); err != nil {
		return nil, err
	}
	for _, spaceset := range s.spacesets {
		if err := spaceset.Encode(enc); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (s *PeerSpaceSet) DecodeUpdate(update []byte) error {
	s.Lock()
	defer s.Unlock()
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
		oldSpaceset, found := s.spacesets[newSpaceset.PeerName]
		if !found || newSpaceset.version > oldSpaceset.version {
			log.Println("Replacing", newSpaceset.PeerName, "data with newer version")
			s.spacesets[newSpaceset.PeerName] = newSpaceset
		}
	}
	return nil
}

func (s *PeerSpaceSet) ConsiderOurPosition() {
	// Rule: if we have no IP space, pick the peer with the most available space and request some
	if s.spacesets[s.ourName].NumFreeAddresses() == 0 {
		var best *PeerSpace = nil
		var bestNumFree uint32 = 0
		for _, spaceset := range s.spacesets {
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

// GossipDelegate methods
func (s *PeerSpaceSet) NotifyMsg(msg []byte) {
	log.Printf("NotifyMsg: %+v\n", msg)
}

func (s *PeerSpaceSet) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts: %d %d\n", overhead, limit)
	return nil
}

func (s *PeerSpaceSet) LocalState(join bool) []byte {
	log.Printf("LocalState: %t\n", join)
	if buf, err := s.Encode(); err == nil {
		return buf
	} else {
		log.Println("Error", err)
	}
	return nil
}

func (s *PeerSpaceSet) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState: %t %d bytes\n", join, len(buf))
	s.DecodeUpdate(buf)
	s.ConsiderOurPosition()
}
