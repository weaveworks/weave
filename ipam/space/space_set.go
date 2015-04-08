package space

import (
	"bytes"
	"fmt"
	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/ipam/utils"
	"net"
	"sort"
)

// Set is a set of spaces...
type Set struct {
	spaces []*Space
}

// For compatibility with sort
func (s Set) Len() int { return len(s.spaces) }
func (s Set) Less(i, j int) bool {
	return utils.IP4int(s.spaces[i].Start) < utils.IP4int(s.spaces[j].Start)
}
func (s Set) Swap(i, j int) { panic("Should never be swapping spaces!") }

// Spaces returns the list of spaces in this space set.
func (s *Set) Spaces() []*Space {
	return s.spaces
}

func (s *Set) String() string {
	var buf bytes.Buffer
	buf.WriteString("Set")
	for _, space := range s.spaces {
		buf.WriteString(fmt.Sprintf("\n  %s", space))
	}
	return buf.String()
}

// -------------------------------------------------

func (s *Set) assertInvariants() {
	utils.Assert(sort.IsSorted(s), "space set must always be sorted")
	// TODO invariant around not overlapping
}

// AddSpace adds a new space to this set.
func (s *Set) AddSpace(newspace Space) {
	s.assertInvariants()
	defer s.assertInvariants()

	i := s.find(newspace.Start)
	utils.Assert(i >= len(s.spaces) || !s.spaces[i].Start.Equal(newspace.Start), "inserting space into list already exists!")

	s.spaces = append(s.spaces, &Space{}) // make space
	copy(s.spaces[i+1:], s.spaces[i:])    // move up
	s.spaces[i] = &newspace               // put in new element
}

// Clear removes all spaces from this space set.  Used during node shutdown.
func (s *Set) Clear() {
	s.spaces = s.spaces[:0]
}

// Return the position of the space at or above start
func (s *Set) find(start net.IP) int {
	return sort.Search(len(s.spaces), func(j int) bool {
		return utils.IP4int(s.spaces[j].Start) >= utils.IP4int(start)
	})
}

// Get returns the space found at start.
func (s *Set) Get(start net.IP) (*Space, bool) {
	i := s.find(start)
	if i < len(s.spaces) && s.spaces[i].Start.Equal(start) {
		return s.spaces[i], true
	}
	return nil, false
}

// NumFreeAddresses returns the total free address across
// all Spaces in this set.
func (s *Set) NumFreeAddresses() uint32 {
	// TODO: Optimize; perhaps maintain the count in allocate and free
	var freeAddresses uint32
	for _, space := range s.spaces {
		freeAddresses += space.NumFreeAddresses()
	}
	return freeAddresses
}

// GiveUpSpace returns some large reasonably-sized chunk of free space.
// Normally because one of our peers has asked for it.
func (s *Set) GiveUpSpace() (net.IP, uint32, bool) {
	s.assertInvariants()
	defer s.assertInvariants()

	// Premature optimisation?
	totalFreeAddresses := s.NumFreeAddresses()
	if totalFreeAddresses == 0 {
		return nil, 0, false
	}

	// First find the biggest free chunk amongst all our spaces
	var bestStart net.IP
	var bestSize uint32
	var bestSpace *Space
	var spaceIndex int
	for j, space := range s.spaces {
		chunkStart, chunkSize := space.BiggestFreeChunk()
		if chunkStart == nil || chunkSize < bestSize {
			continue
		}

		bestStart = chunkStart
		bestSize = chunkSize
		bestSpace = space
		spaceIndex = j
	}

	if bestStart == nil {
		utils.Assert(s.NumFreeAddresses() == 0, "Failed to find a range but have free addresses")
		return nil, 0, false
	}

	// Now right-size this space.
	// Never give away more than half a space
	// But don't try ang give away nothing
	utils.Assert(bestSize <= bestSpace.Size, "Space gave me free bigger than themselves!")
	var maxDonation = bestSpace.Size / 2
	if maxDonation < 4 {
		maxDonation = 4
	}

	if bestSize > maxDonation {
		// Try and align the start to the right most
		shift := bestSize - maxDonation
		bestStart = utils.Add(bestStart, shift)
		bestSize = maxDonation
	}

	utils.Assert(bestSize > 0, "Trying to give away nothing!")

	lg.Debug.Println("GiveUpSpace start =", bestStart, "size =", bestSize, "from", bestSpace)

	// Now split and remove the final space
	utils.Assert(bestSpace.contains(bestStart), "WTF?")

	split1, split2 := bestSpace.Split(bestStart)
	lg.Debug.Println("GiveUpSpace splits", split1, split2)
	var split3 *Space
	if split2.Size != bestSize {
		endAddress := utils.Add(bestStart, bestSize)
		split2, split3 = split2.Split(endAddress)
		lg.Debug.Println("GiveUpSpace splits", split1, split2, split3)
	}

	utils.Assert(split2.NumFreeAddresses() == bestSize, "Trying to free a space with stuff in it!")

	// Take out the old space, then add up to two new spaces.
	// Ordering of s.spaces is important.
	s.spaces = append(s.spaces[:spaceIndex], s.spaces[spaceIndex+1:]...)

	if split1.Size > 0 {
		s.AddSpace(*split1)
	}
	if split3 != nil {
		s.AddSpace(*split3)
	}

	return bestStart, bestSize, true
}

// Allocate calls allocate on each Space this set owns, until
// it gets an address.
func (s *Set) Allocate() net.IP {
	// TODO: Optimize; perhaps cache last-used space
	for _, space := range s.spaces {
		if ret := space.Allocate(); ret != nil {
			return ret
		}
	}
	return nil
}

// Free returns the provided address to the
// Space that owns it.
func (s *Set) Free(addr net.IP) error {
	for _, space := range s.spaces {
		if space.contains(addr) {
			return space.Free(addr)
		}
	}
	lg.Debug.Println("Address", addr, "not in range", s)
	return fmt.Errorf("IP %s address not in range", addr)
}

// Claim an address that we think we should own
func (s *Set) Claim(addr net.IP) error {
	for _, space := range s.spaces {
		if done, err := space.Claim(addr); err != nil {
			return err
		} else if done {
			return nil
		}
	}
	return fmt.Errorf("IP %s address not in range", addr)
}
