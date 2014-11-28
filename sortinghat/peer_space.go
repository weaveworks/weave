package sortinghat

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/zettio/weave/router"
	"sync"
)

type PeerSpace struct {
	router.PeerName
	version uint64
	spaces  []MinSpace
	sync.RWMutex
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
		if err := enc.Encode(space); err != nil {
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
	s.spaces = make([]MinSpace, numSpaces)
	for i := 0; i < numSpaces; i++ {
		if err := decoder.Decode(&s.spaces[i]); err != nil {
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
	fmt.Printf("NotifyMsg: %+v", msg)
}

func (s *PeerSpace) GetBroadcasts(overhead, limit int) [][]byte {
	fmt.Printf("GetBroadcasts: %d %d", overhead, limit)
	return nil
}

func (s *PeerSpace) LocalState(join bool) []byte {
	fmt.Printf("LocalState: %t", join)
	return []byte("something")
}

func (s *PeerSpace) MergeRemoteState(buf []byte, join bool) {
	fmt.Printf("MergeRemoteState: %t %+v", join, buf)
}
