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

type MinSpace struct {
	Start         net.IP
	Size          uint32
	Max_allocated uint32
}

type Space struct {
	MinSpace
	recs      []Record
	free_list []net.IP
	sync.RWMutex
}

func NewSpace(start net.IP, size uint32) *Space {
	return &Space{MinSpace: MinSpace{Start: start, Size: size, Max_allocated: 0}}
}

func (space *Space) AllocateFor(ident string) net.IP {
	space.Lock()
	defer space.Unlock()
	var ret net.IP = nil
	if n := len(space.free_list); n > 0 {
		ret = space.free_list[n-1]
		space.free_list = space.free_list[:n-1]
	} else if space.Max_allocated < space.Size {
		space.Max_allocated++
		ret = Add(space.Start, space.Max_allocated-1)
	} else {
		return nil
	}
	space.recs = append(space.recs, Record{ident, ret})
	return ret
}

func (space *Space) Free(addr net.IP) {
	space.Lock()
	space.free_list = append(space.free_list, addr)
	// TODO: consolidate free space
	space.Unlock()
}

// IPv4 Address Arithmetic - convert to 32-bit unsigned integer, add, and convert back
func Add(addr net.IP, i uint32) net.IP {
	ip := addr.To4()
	if ip == nil {
		return nil
	}
	sum := (uint32(ip[0]) << 24) + (uint32(ip[1]) << 16) + (uint32(ip[2]) << 8) + uint32(ip[3]) + i
	p := make(net.IP, net.IPv4len)
	p[0] = byte(sum >> 24)
	p[1] = byte((sum & 0xffffff) >> 16)
	p[2] = byte((sum & 0xffff) >> 8)
	p[3] = byte(sum & 0xff)
	return p
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

func (space *Space) String() string {
	space.RLock()
	defer space.RUnlock()
	return fmt.Sprintf("Space allocator start %s, size %d, allocated %d, free %d", space.Start, space.Size, len(space.recs), len(space.free_list))
}
