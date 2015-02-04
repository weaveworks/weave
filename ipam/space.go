package ipam

import (
	"fmt"
	"net"
	"sync"
)

type Space interface {
	GetMinSpace() *MinSpace
	GetStart() net.IP
	GetSize() uint32
	GetMaxAllocated() uint32
	LargestFreeBlock() uint32
	Overlaps(b Space) bool
	IsHeirTo(b *MinSpace, universe *MinSpace) bool
	String() string
}

// This struct is used in Gob-encoding to pass info around, which is why all of its fields are exported.
type MinSpace struct {
	Start        net.IP
	Size         uint32
	MaxAllocated uint32
}

func (s *MinSpace) GetMinSpace() *MinSpace   { return s }
func (s *MinSpace) GetStart() net.IP         { return s.Start }
func (s *MinSpace) GetSize() uint32          { return s.Size }
func (s *MinSpace) GetMaxAllocated() uint32  { return s.MaxAllocated }
func (s *MinSpace) LargestFreeBlock() uint32 { return s.Size - s.MaxAllocated }

func (a *MinSpace) Overlaps(b Space) bool {
	diff := subtract(a.Start, b.GetStart())
	return !(-diff >= int64(a.Size) || diff >= int64(b.GetSize()))
}

func (a *MinSpace) Contains(addr net.IP) bool {
	diff := subtract(addr, a.Start)
	return diff >= 0 && diff < int64(a.Size)
}

// A space is heir to another space if it is immediately lower than it
// (considering the universe as a ring)
func (a *MinSpace) IsHeirTo(b *MinSpace, universe *MinSpace) bool {
	startA, startB := subtract(a.Start, universe.Start), subtract(b.Start, universe.Start)
	if startA < 0 || startB < 0 { // space outside our universe
		return false
	}
	sizeU, sizeA := int64(universe.Size), int64(a.Size)
	return startA < startB && startA+sizeA == startB ||
		startA > startB && startA+sizeA-sizeU == startB
}

func (s *MinSpace) String() string {
	if s.MaxAllocated > 0 {
		return fmt.Sprintf("%s+%d, %d", s.Start, s.Size, s.MaxAllocated)
	} else {
		return fmt.Sprintf("%s+%d", s.Start, s.Size)
	}
}

func NewMinSpace(start net.IP, size uint32) *MinSpace {
	return &MinSpace{Start: start, Size: size, MaxAllocated: 0}
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
	allocated AllocationList
	free_list AllocationList
	sync.RWMutex
}

func NewSpace(start net.IP, size uint32) *MutableSpace {
	return &MutableSpace{MinSpace: MinSpace{Start: start, Size: size, MaxAllocated: 0}}
}

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

func ip4int(ip4 net.IP) (r uint32) {
	for _, b := range ip4.To4() {
		r <<= 8
		r |= uint32(b)
	}
	return
}

func intip4(key uint32) (r net.IP) {
	r = make([]byte, net.IPv4len)
	for i := 3; i >= 0; i-- {
		r[i] = byte(key)
		key >>= 8
	}
	return
}

// IPv4 Address Arithmetic - convert to 32-bit unsigned integer, add, and convert back
func add(addr net.IP, i uint32) net.IP {
	sum := ip4int(addr) + i
	return intip4(sum)
}

func subtract(a, b net.IP) int64 {
	return int64(ip4int(a)) - int64(ip4int(b))
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

func (s *MutableSpace) LargestFreeBlock() uint32 {
	return s.Size - s.MaxAllocated
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
