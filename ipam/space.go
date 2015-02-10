package ipam

import (
	"fmt"
	"net"
	"sync"
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

func NewMinSpace(start net.IP, size uint32) *MinSpace {
	return &MinSpace{Start: start, Size: size}
}

type Allocation struct {
	Ident string
	IP    net.IP
}

func (a *Allocation) String() string {
	return fmt.Sprintf("%s %s", a.Ident, a.IP)
}

type AllocationList []Allocation

func (aa *AllocationList) add(a *Allocation) {
	*aa = append(*aa, *a)
}

func (aa *AllocationList) remove(addr net.IP) *Allocation {
	for i, a := range *aa {
		if a.IP.Equal(addr) {
			// Delete by swapping the last element into this one and truncating
			last := len(*aa) - 1
			(*aa)[i], (*aa) = (*aa)[last], (*aa)[:last]
			return &a
		}
	}
	return nil
}

func (aa *AllocationList) take() *Allocation {
	if n := len(*aa); n > 0 {
		ret := (*aa)[n-1]
		*aa = (*aa)[:n-1]
		return &ret
	}
	return nil
}

type MutableSpace struct {
	MinSpace
	MaxAllocated uint32 // 0 if nothing allocated, 1 if first address allocated, etc.
	allocated    AllocationList
	free_list    AllocationList
	sync.RWMutex
}

func NewSpace(start net.IP, size uint32) *MutableSpace {
	return &MutableSpace{MinSpace: MinSpace{Start: start, Size: size}, MaxAllocated: 0}
}

// Mark an address as allocated on behalf of some specific container
func (space *MutableSpace) Claim(ident string, addr net.IP) bool {
	space.Lock()
	defer space.Unlock()
	offset := subtract(addr, space.Start)
	if !(offset >= 0 && offset < int64(space.Size)) {
		return false
	}
	if uint32(offset) > space.MaxAllocated {
		// Need to add all the addresses in the gap to the free list
		for i := space.MaxAllocated + 1; i < uint32(offset); i++ {
			addr := add(space.Start, i)
			space.free_list.add(&Allocation{"", addr})
		}
		space.MaxAllocated = uint32(offset)
	}
	space.allocated.add(&Allocation{ident, addr})
	return true
}

func (space *MutableSpace) AllocateFor(ident string) net.IP {
	space.Lock()
	defer space.Unlock()
	ret := space.free_list.take()
	if ret != nil {
		ret.Ident = ident
	} else if space.MaxAllocated < space.Size {
		space.MaxAllocated++
		ret = &Allocation{ident, add(space.Start, space.MaxAllocated-1)}
	} else {
		return nil
	}
	space.allocated.add(ret)
	return ret.IP
}

func (space *MutableSpace) Free(addr net.IP) bool {
	if !space.Contains(addr) {
		return false
	}
	space.Lock()
	defer space.Unlock()
	if a := space.allocated.remove(addr); a != nil {
		space.free_list.add(a)
		// TODO: consolidate free space
		return true
	}
	return false
}

func (space *MutableSpace) DeleteRecordsFor(ident string) error {
	space.Lock()
	defer space.Unlock()
	w := 0 // write index

	for _, r := range space.allocated {
		if r.Ident == ident {
			space.free_list.add(&r)
		} else {
			space.allocated[w] = r
			w++
		}
	}
	space.allocated = space.allocated[:w]
	return nil
}

func (s *MutableSpace) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	return s.Size - uint32(len(s.allocated))
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
	space.RLock()
	defer space.RUnlock()
	return space.string()
}

func (space *MutableSpace) string() string {
	return fmt.Sprintf("%s+%d, %d/%d", space.Start, space.Size, len(space.allocated), len(space.free_list))
}

// Divide a space into two new spaces at a given address, copying allocations and frees.
func (space *MutableSpace) Split(addr net.IP) (*MutableSpace, *MutableSpace) {
	space.Lock()
	defer space.Unlock()
	breakpoint := subtract(addr, space.Start)
	if breakpoint < 0 || breakpoint >= int64(space.Size) {
		return nil, nil // Not contained within this space
	}
	ret1 := NewSpace(space.GetStart(), uint32(breakpoint))
	ret2 := NewSpace(addr, space.Size-uint32(breakpoint))

	// Copy all the allocations and find the max-allocated point for each
	for _, alloc := range space.allocated {
		offset := subtract(alloc.IP, addr)
		if offset < 0 {
			ret1.allocated.add(&alloc)
			if uint32(breakpoint+offset)+1 > ret1.MaxAllocated {
				ret1.MaxAllocated = uint32(breakpoint+offset) + 1
			}
		} else {
			ret2.allocated.add(&alloc)
			if uint32(offset)+1 > ret2.MaxAllocated {
				ret2.MaxAllocated = uint32(offset) + 1
			}
		}
	}
	// Now copy the free list, but omit anything above MaxAllocated in each case
	for _, alloc := range space.free_list {
		offset := subtract(alloc.IP, addr)
		if offset < 0 {
			if uint32(offset+breakpoint) < ret1.MaxAllocated {
				ret1.free_list.add(&alloc)
			}
		} else {
			if uint32(offset) < ret2.MaxAllocated {
				ret2.free_list.add(&alloc)
			}
		}
	}

	return ret1, ret2
}
