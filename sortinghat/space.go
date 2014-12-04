package sortinghat

import (
	"fmt"
	"net"
	"sync"
)

type Record struct {
	Ident string
	IP    net.IP
}

type SpaceInfo interface {
	GetStart() net.IP
	GetSize() uint32
	GetMaxAllocated() uint32
	GetMinSpace() *MinSpace
	NumFreeAddresses() uint32
}

type MinSpace struct {
	Start        net.IP
	Size         uint32
	MaxAllocated uint32
}

type Space struct {
	MinSpace
	recs      []Record
	free_list []net.IP
	sync.RWMutex
}

func (s *MinSpace) GetMinSpace() *MinSpace {
	return s
}

func (s *MinSpace) GetStart() net.IP {
	return s.Start
}

func (s *MinSpace) GetSize() uint32 {
	return s.Size
}

func (s *MinSpace) GetMaxAllocated() uint32 {
	return s.MaxAllocated
}

func (s *MinSpace) NumFreeAddresses() uint32 {
	return s.Size - s.MaxAllocated
}

func NewSpace(start net.IP, size uint32) *Space {
	return &Space{MinSpace: MinSpace{Start: start, Size: size, MaxAllocated: 0}}
}

func (space *Space) AllocateFor(ident string) net.IP {
	space.Lock()
	defer space.Unlock()
	var ret net.IP = nil
	if n := len(space.free_list); n > 0 {
		ret = space.free_list[n-1]
		space.free_list = space.free_list[:n-1]
	} else if space.MaxAllocated < space.Size {
		space.MaxAllocated++
		ret = add(space.Start, space.MaxAllocated-1)
	} else {
		return nil
	}
	space.recs = append(space.recs, Record{ident, ret})
	return ret
}

func (space *Space) Free(addr net.IP) bool {
	diff := subtract(addr, space.Start)
	if diff < 0 || diff >= int64(space.Size) {
		return false
	}
	space.Lock()
	space.free_list = append(space.free_list, addr)
	// TODO: consolidate free space
	space.Unlock()
	return true
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

func (space *Space) DeleteRecordsFor(ident string) error {
	space.Lock()
	defer space.Unlock()
	w := 0 // write index

	for _, r := range space.recs {
		if r.Ident == ident {
			space.Free(r.IP)
		} else {
			space.recs[w] = r
			w++
		}
	}
	space.recs = space.recs[:w]
	return nil
}

func (s *Space) NumFreeAddresses() uint32 {
	s.RLock()
	defer s.RUnlock()
	return s.Size - uint32(len(s.recs)) + uint32(len(s.free_list))
}

func (space *Space) String() string {
	space.RLock()
	defer space.RUnlock()
	return fmt.Sprintf("Space allocator start %s, size %d, allocated %d, free %d", space.Start, space.Size, len(space.recs), len(space.free_list))
}
