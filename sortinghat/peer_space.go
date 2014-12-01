package sortinghat

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/zettio/weave/router"
	"log"
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

func (s *PeerSpace) Encode() ([]byte, error) {
	s.RLock()
	defer s.RUnlock()
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(s.PeerName); err != nil {
		return nil, err
	}
	if err := enc.Encode(s.version); err != nil {
		return nil, err
	}
	if err := enc.Encode(len(s.spaces)); err != nil {
		return nil, err
	}
	for _, space := range s.spaces {
		if err := enc.Encode(space.GetMinSpace()); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (s *PeerSpace) DecodeUpdate(update []byte) error {
	s.Lock()
	defer s.Unlock()
	reader := bytes.NewReader(update)
	decoder := gob.NewDecoder(reader)
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

// GossipDelegate methods
func (s *PeerSpace) NotifyMsg(msg []byte) {
	log.Printf("NotifyMsg: %+v\n", msg)
}

func (s *PeerSpace) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts: %d %d\n", overhead, limit)
	return nil
}

func (s *PeerSpace) LocalState(join bool) []byte {
	log.Printf("LocalState: %t\n", join)
	if buf, err := s.Encode(); err == nil {
		return buf
	} else {
		log.Println("Error", err)
	}
	return nil
}

func (s *PeerSpace) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState: %t %+v\n", join, buf)
}
