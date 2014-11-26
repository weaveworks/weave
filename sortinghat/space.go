package sortinghat

import (
	"fmt"
	"net"
)

type Space interface {
	AllocateFor(ident string) net.IP
	DeleteRecordsFor(ident string) error
	Free(addr net.IP)
}

type Record struct {
	Ident string
	IP    net.IP
}

type simpleSpace struct {
	start         net.IP
	size          uint32
	max_allocated uint32
	recs          []Record
	free_list     []net.IP
}

func NewSpace(start net.IP, size uint32) Space {
	return &simpleSpace{start: start, size: size, max_allocated: 0}
}

func (space *simpleSpace) AllocateFor(ident string) net.IP {
	var ret net.IP = nil
	if n := len(space.free_list); n > 0 {
		ret = space.free_list[n-1]
		space.free_list = space.free_list[:n-1]
	} else if space.max_allocated < space.size {
		space.max_allocated++
		ret = Add(space.start, space.max_allocated-1)
	} else {
		return nil
	}
	space.recs = append(space.recs, Record{ident, ret})
	return ret
}

func (space *simpleSpace) Free(addr net.IP) {
	space.free_list = append(space.free_list, addr)
	// TODO: consolidate free space
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

func (space *simpleSpace) DeleteRecordsFor(ident string) error {
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

func (space *simpleSpace) String() string {
	return fmt.Sprintf("Space allocator start %s, size %d, allocated %d, free %d", space.start, space.size, len(space.recs), len(space.free_list))
}
