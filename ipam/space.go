package ipam

import (
	"fmt"
	"net"
)

type Space interface {
	GetStart() net.IP
	GetSize() uint32
	Overlaps(b Space) bool
	IsHeirTo(b Space, universe Space) bool
}

// This struct is used in Gob-encoding to pass info around, which is why all of its fields are exported.
type MinSpace struct {
	Start net.IP
	Size  uint32
}

func (s *MinSpace) GetStart() net.IP { return s.Start }
func (s *MinSpace) GetSize() uint32  { return s.Size }

func (a *MinSpace) Overlaps(b Space) bool {
	diff := subtract(a.Start, b.GetStart())
	return !(-diff >= int64(a.Size) || diff >= int64(b.GetSize()))
}

func (a *MinSpace) ContainsSpace(b Space) bool {
	diff := subtract(b.GetStart(), a.Start)
	return diff >= 0 && diff+int64(b.GetSize()) <= int64(a.Size)
}

func (a *MinSpace) Contains(addr net.IP) bool {
	diff := subtract(addr, a.Start)
	return diff >= 0 && diff < int64(a.Size)
}

// A space is heir to another space if it is immediately lower than it
// (considering the universe as a ring)
func (a *MinSpace) IsHeirTo(b Space, universe Space) bool {
	startA, startB := subtract(a.Start, universe.GetStart()), subtract(b.GetStart(), universe.GetStart())
	if startA < 0 || startB < 0 { // space outside our universe
		return false
	}
	sizeU, sizeA := int64(universe.GetSize()), int64(a.Size)
	return startA < startB && startA+sizeA == startB ||
		startA > startB && startA+sizeA-sizeU == startB
}

func (s *MinSpace) String() string {
	return fmt.Sprintf("%s+%d", s.Start, s.Size)
}

type addressList []net.IP

// Maintain addresses in increasing order.
func (aa *addressList) add(a net.IP) {
	for i, b := range *aa {
		if subtract(b, a) > 0 {
			(*aa) = append((*aa), nil)   // make space
			copy((*aa)[i+1:], (*aa)[i:]) // move up
			(*aa)[i] = a                 // put in new element
			return
		}
	}
	*aa = append(*aa, a)
}

func (aa *addressList) removeAt(pos int) {
	// Delete, preserving order
	(*aa) = append((*aa)[:pos], (*aa)[pos+1:]...)
}

func (aa *addressList) find(addr net.IP) int {
	for i, a := range *aa {
		if a.Equal(addr) {
			return i
		}
	}
	return -1
}

func (aa *addressList) take() net.IP {
	if n := len(*aa); n > 0 {
		ret := (*aa)[n-1]
		*aa = (*aa)[:n-1]
		return ret
	}
	return nil
}

type MutableSpace struct {
	MinSpace
	MaxAllocated uint32 // 0 if nothing allocated, 1 if first address allocated, etc.
	free_list    addressList
}

func NewSpace(start net.IP, size uint32) *MutableSpace {
	return &MutableSpace{MinSpace: MinSpace{Start: start, Size: size}, MaxAllocated: 0}
}

// Mark an address as allocated on behalf of some specific container
func (space *MutableSpace) Claim(addr net.IP) (bool, error) {
	offset := subtract(addr, space.Start)
	if !(offset >= 0 && offset < int64(space.Size)) {
		return false, nil
	}
	// note: MaxAllocated is one more than the offset of the last allocated address
	if uint32(offset) >= space.MaxAllocated {
		// Need to add all the addresses in the gap to the free list
		for i := space.MaxAllocated; i < uint32(offset); i++ {
			addr := add(space.Start, i)
			space.free_list.add(addr)
		}
		space.MaxAllocated = uint32(offset) + 1
	}
	return true, nil
}

func (space *MutableSpace) Allocate() net.IP {
	ret := space.free_list.take()
	if ret == nil && space.MaxAllocated < space.Size {
		space.MaxAllocated++
		ret = add(space.Start, space.MaxAllocated-1)
	}
	return ret
}

func (space *MutableSpace) Free(addr net.IP) error {
	offset := subtract(addr, space.Start)
	if !(offset >= 0 && offset < int64(space.Size)) {
		return fmt.Errorf("Free out of range: %s", addr)
	} else if offset >= int64(space.MaxAllocated) {
		return fmt.Errorf("IP address not allocated: %s", addr)
	} else if space.free_list.find(addr) >= 0 {
		return fmt.Errorf("Duplicate free: %s", addr)
	}
	space.free_list.add(addr)
	// TODO: consolidate free space
	return nil
}

func (s *MutableSpace) BiggestFreeChunk() *MinSpace {
	// Return some chunk, not necessarily _the_ biggest
	if s.MaxAllocated < s.Size {
		return &MinSpace{add(s.Start, s.MaxAllocated), s.Size - s.MaxAllocated}
	} else if len(s.free_list) > 0 {
		// Find how many contiguous addresses are at the head of the free list
		size := 1
		for ; size < len(s.free_list) && subtract(s.free_list[size], s.free_list[size-1]) == 1; size++ {
		}
		return &MinSpace{s.free_list[0], uint32(size)}
	}
	return nil
}

func (s *MutableSpace) NumFreeAddresses() uint32 {
	return s.Size - s.MaxAllocated + uint32(len(s.free_list))
}

// Enlarge a space by merging in a blank space and return true
// or return false if the space supplied is not contiguous and directly after this one
func (a *MutableSpace) mergeBlank(b Space) bool {
	diff := subtract(b.GetStart(), a.Start)
	if diff != int64(a.Size) {
		return false
	} else {
		a.Size += b.GetSize()
		return true
	}
}

func (space *MutableSpace) String() string {
	return fmt.Sprintf("%s+%d, %d/%d", space.Start, space.Size, space.MaxAllocated, len(space.free_list))
}

// Divide a space into two new spaces at a given address, copying allocations and frees.
func (space *MutableSpace) Split(addr net.IP) (*MutableSpace, *MutableSpace) {
	breakpoint := subtract(addr, space.Start)
	if breakpoint < 0 || breakpoint >= int64(space.Size) {
		return nil, nil // Not contained within this space
	}
	ret1 := NewSpace(space.GetStart(), uint32(breakpoint))
	ret2 := NewSpace(addr, space.Size-uint32(breakpoint))

	// find the max-allocated point for each sub-space
	if space.MaxAllocated > uint32(breakpoint) {
		ret1.MaxAllocated = ret1.Size
		ret2.MaxAllocated = space.MaxAllocated - ret1.Size
	} else {
		ret1.MaxAllocated = space.MaxAllocated
		ret2.MaxAllocated = 0
	}

	// Now copy the free list, but omit anything above MaxAllocated in each case
	for _, alloc := range space.free_list {
		offset := subtract(alloc, addr)
		if offset < 0 {
			if uint32(offset+breakpoint) < ret1.MaxAllocated {
				ret1.free_list.add(alloc)
			}
		} else {
			if uint32(offset) < ret2.MaxAllocated {
				ret2.free_list.add(alloc)
			}
		}
	}

	return ret1, ret2
}
