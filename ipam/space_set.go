package ipam

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	"math"
	"net"
	"sync"
	"time"
)

type SpaceSet interface {
	Encode(enc *gob.Encoder) error
	Decode(decoder *gob.Decoder) error
	Empty() bool
	Version() uint64
	PeerName() router.PeerName
	UID() uint64
	HasFreeAddresses() bool
	Overlaps(space Space) bool
	String() string
	MaybeDead() bool
	ForEachSpace(fun func(Space))
	NumSpacesMergeable(SpaceSet, Space) int
}

// This represents a peer's space allocations, which we only hear about.
type PeerSpaceSet struct {
	peerName  router.PeerName
	uid       uint64
	version   uint64
	spaces    []Space
	lastSeen  time.Time
	hasFree   bool
	maybeDead bool
	sync.RWMutex
}

func NewPeerSpace(pn router.PeerName, uid uint64) *PeerSpaceSet {
	return &PeerSpaceSet{peerName: pn, uid: uid}
}

func (s *PeerSpaceSet) PeerName() router.PeerName { return s.peerName }
func (s *PeerSpaceSet) UID() uint64               { return s.uid }
func (s *PeerSpaceSet) Version() uint64           { return s.version }
func (s *PeerSpaceSet) MaybeDead() bool           { return s.maybeDead }
func (s *PeerSpaceSet) HasFreeAddresses() bool    { return s.hasFree }

type peerSpaceTransport struct {
	PeerName router.PeerName
	UID      uint64
	Version  uint64
	Spaces   []Space
	HasFree  bool
}

func (s *PeerSpaceSet) Encode(enc *gob.Encoder) error {
	s.RLock()
	defer s.RUnlock()
	return s.encode(enc, s.HasFreeAddresses())
}

func (s *PeerSpaceSet) encode(enc *gob.Encoder, hasFree bool) error {
	// Copy as MinSpace to eliminate any MutableSpace info
	spaces := make([]Space, len(s.spaces))
	for i, space := range s.spaces {
		spaces[i] = &MinSpace{space.GetStart(), space.GetSize()}
	}
	return enc.Encode(peerSpaceTransport{s.peerName, s.uid, s.version, spaces, hasFree})
}

func (s *PeerSpaceSet) Decode(decoder *gob.Decoder) error {
	var t peerSpaceTransport
	if err := decoder.Decode(&t); err != nil {
		return err
	}
	s.Lock() // probably unnecessary - why would someone be decoding into an object that is also accessed from another thread?
	defer s.Unlock()
	s.peerName, s.uid, s.version, s.spaces, s.hasFree = t.PeerName, t.UID, t.Version, t.Spaces, t.HasFree
	return nil
}

// Need this for gob decode into an interface pointer to work
func init() {
	gob.Register(&MinSpace{})
}

func (s *PeerSpaceSet) ForEachSpace(fun func(Space)) {
	s.RLock()
	defer s.RUnlock()
	for _, space := range s.spaces {
		fun(space)
	}
}

func (s *PeerSpaceSet) String() string {
	s.RLock()
	defer s.RUnlock()
	ver := " (tombstone)"
	if !s.IsTombstone() {
		ver = fmt.Sprint(" (v", s.version, ")")
	}
	extra := ""
	if s.MaybeDead() {
		extra = " (maybe dead)"
	}
	if s.HasFreeAddresses() {
		extra += " (has free)"
	}
	return s.describe(fmt.Sprint("SpaceSet ", s.peerName, s.uid, ver, extra))
}

func (s *PeerSpaceSet) describe(heading string) string {
	var buf bytes.Buffer
	buf.WriteString(heading)
	for _, space := range s.spaces {
		buf.WriteString(fmt.Sprintf("\n  %s", space))
	}
	return buf.String()
}

func (s *PeerSpaceSet) Empty() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.spaces) == 0
}

// Count the number of times b has a space which is heir to a space in a
// We presume that if b gave up some space, it would be at the end of a reservation
// so if b gives it to a then a can merge it
func (a *PeerSpaceSet) NumSpacesMergeable(b SpaceSet, universe Space) (count int) {
	a.RLock()
	defer a.RUnlock()
	for _, space1 := range a.spaces { // dumb O(n2) implementation
		b.ForEachSpace(func(space2 Space) {
			if space2.IsHeirTo(space1, universe) {
				count++
			}
		})
	}
	return
}

func (s *PeerSpaceSet) Overlaps(space Space) bool {
	s.RLock()
	defer s.RUnlock()
	for _, space2 := range s.spaces {
		if space.Overlaps(space2) {
			return true
		}
	}
	return false
}

func (s *PeerSpaceSet) MarkMaybeDead(f bool, now time.Time) {
	s.Lock()
	s.maybeDead = f
	s.lastSeen = now
	s.Unlock()
}

func (s *PeerSpaceSet) MakeTombstone() {
	s.Lock()
	s.spaces = nil
	s.version = math.MaxUint64
	s.hasFree = false
	s.Unlock()
}

func (s *PeerSpaceSet) IsTombstone() bool {
	return s.version == math.MaxUint64
}

// -------------------------------------------------

// Represents our own space, which we can allocate and free within.
type OurSpaceSet struct {
	PeerSpaceSet
}

func NewSpaceSet(pn router.PeerName, uid uint64) *OurSpaceSet {
	return &OurSpaceSet{PeerSpaceSet{peerName: pn, uid: uid}}
}

func (s *OurSpaceSet) Encode(enc *gob.Encoder) error {
	s.RLock()
	defer s.RUnlock()
	return s.encode(enc, s.HasFreeAddresses())
}

func (s *OurSpaceSet) AddSpace(space *MutableSpace) {
	s.Lock()
	defer s.Unlock()
	s.spaces = append(s.spaces, space)
	s.version++
}

func (s *OurSpaceSet) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	// TODO: Optimize; perhaps maintain the count in allocate and free
	var freeAddresses uint32 = 0
	for _, space := range s.spaces {
		freeAddresses += space.(*MutableSpace).NumFreeAddresses()
	}
	return freeAddresses
}

func (s *OurSpaceSet) HasFreeAddresses() bool {
	s.RLock()
	defer s.RUnlock()
	return s.NumFreeAddresses() > 0
}

// Give up some space because one of our peers has asked for it.
// Pick some large reasonably-sized chunk.
func (s *OurSpaceSet) GiveUpSpace() (ret *MinSpace, ok bool) {
	s.Lock()
	defer s.Unlock()
	totalFreeAddresses := s.NumFreeAddresses()
	if totalFreeAddresses < MinSafeFreeAddresses {
		return nil, false
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
		return NewMinSpace(add(bestSpace.Start, bestSpace.Size), spaceToGiveUp), true
	}
	return nil, false
}

// If we can, give up the space requested and return true.
func (s *OurSpaceSet) GiveUpSpecificSpace(spaceClaimed Space) bool {
	s.Lock()
	defer s.Unlock()
	for i, space := range s.spaces {
		mspace := space.(*MutableSpace)
		if mspace.ContainsSpace(spaceClaimed) {
			split1, split2 := mspace.Split(spaceClaimed.GetStart())
			var split3 *MutableSpace = nil
			if split2.GetSize() != spaceClaimed.GetSize() {
				endAddress := add(spaceClaimed.GetStart(), spaceClaimed.GetSize())
				split2, split3 = split2.Split(endAddress)
			}
			if split2.NumFreeAddresses() == spaceClaimed.GetSize() {
				newspaces := s.spaces[:i]
				if split1.GetSize() > 0 {
					newspaces = append(newspaces, split1)
				}
				if split3 != nil {
					newspaces = append(newspaces, split3)
				}
				s.spaces = append(newspaces, s.spaces[i+1:]...)
				s.version++
				return true
			} else {
				lg.Debug.Println("Unable to give up space", split2)
				return false // space not free
			}
		}
	}
	return false
}

func (s *OurSpaceSet) AllocateFor(ident string) net.IP {
	s.Lock()
	defer s.Unlock()
	// TODO: Optimize; perhaps cache last-used space
	for _, space := range s.spaces {
		if ret := space.(*MutableSpace).AllocateFor(ident); ret != nil {
			s.version++
			return ret
		}
	}
	return nil
}

// Claim an address that we think we should own
func (s *OurSpaceSet) Claim(ident string, addr net.IP) error {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		if done, err := space.(*MutableSpace).Claim(ident, addr); err != nil {
			return err
		} else if done {
			s.version++
			return nil
		}
	}
	return errors.New("Attempt to claim IP address not in range")
}

func (s *OurSpaceSet) Free(addr net.IP) error {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		if space.(*MutableSpace).Free(addr) {
			s.version++
			return nil
		}
	}
	return errors.New("Attempt to free IP address not in range")
}

func (s *OurSpaceSet) DeleteRecordsFor(ident string) {
	s.Lock()
	defer s.Unlock()
	for _, space := range s.spaces {
		space.(*MutableSpace).DeleteRecordsFor(ident)
	}
	s.version++
}

func endOfBlock(a Space) net.IP {
	return add(a.GetStart(), a.GetSize())
}

func (s *OurSpaceSet) Exclude(a Space) bool {
	s.Lock()
	defer s.Unlock()
	ns := make([]Space, 0)
	aSize := int64(a.GetSize())
	for _, b := range s.spaces {
		bSize := int64(b.GetSize())
		diff := subtract(a.GetStart(), b.GetStart())
		if diff > 0 && diff < bSize {
			ns = append(ns, NewMinSpace(b.GetStart(), uint32(diff)))
			if bSize > aSize+diff {
				ns = append(ns, NewMinSpace(endOfBlock(a), uint32(bSize-(aSize+diff))))
			}
		} else if diff <= 0 && -diff < aSize {
			if aSize+diff < bSize {
				ns = append(ns, NewMinSpace(endOfBlock(a), uint32(bSize-(aSize+diff))))
			}
		} else { // Pieces do not overlap; leave the existing one in place
			ns = append(ns, b)
		}
	}
	s.spaces = ns
	return false
}
