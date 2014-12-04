package sortinghat

import (
	"bytes"
	"encoding/gob"
	"github.com/zettio/weave/router"
	"log"
	"sync"
)

type PeerSpaceSet struct {
	spacesets map[router.PeerName]*PeerSpace
	sync.RWMutex
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
