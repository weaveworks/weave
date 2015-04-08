package space

import (
	"fmt"
	"github.com/zettio/weave/ipam/utils"
	"net"
	"sort"
)

// Space repsents a range of addresses owned by this peer,
// and contains the state for managing free addresses.
type Space struct {
	Start        net.IP
	Size         uint32
	MaxAllocated uint32 // 0 if nothing allocated, 1 if first address allocated, etc.
	freelist     addressList
}

func (space *Space) assertInvariants() {
	utils.Assert(space.MaxAllocated <= space.Size,
		"MaxAllocated must not be greater than size")
	utils.Assert(sort.IsSorted(space.freelist),
		"Free address list must always be sorted")
	utils.Assert(uint32(len(space.freelist)) <= space.MaxAllocated,
		"Can't have more entries on free list than allocated.")
}

func (space *Space) contains(addr net.IP) bool {
	diff := utils.Subtract(addr, space.Start)
	return diff >= 0 && diff < int64(space.Size)
}

// Mark an address as allocated on behalf of some specific container
func (space *Space) Claim(addr net.IP) (bool, error) {
	offset := utils.Subtract(addr, space.Start)
	if !(offset >= 0 && offset < int64(space.Size)) {
		return false, nil
	}
	// note: MaxAllocated is one more than the offset of the last allocated address
	if uint32(offset) >= space.MaxAllocated {
		// Need to add all the addresses in the gap to the free list
		for i := space.MaxAllocated; i < uint32(offset); i++ {
			addr := utils.Add(space.Start, i)
			space.freelist.add(addr)
		}
		space.MaxAllocated = uint32(offset) + 1
	} else if pos := space.freelist.find(addr); pos >= 0 {
		space.freelist.removeAt(pos)
	}
	return true, nil
}

// Allocate returns the lowest availible IP within this space.
func (space *Space) Allocate() net.IP {
	space.assertInvariants()
	defer space.assertInvariants()

	// First ask the free list; this will get
	// the lowest availible address
	if ret := space.freelist.take(); ret != nil {
		return ret
	}

	// If nothing on the free list, have we given
	// out all the addresses?
	if space.MaxAllocated >= space.Size {
		return nil
	}

	// Otherwise increase the number of address we have given
	// out
	space.MaxAllocated++
	return utils.Add(space.Start, space.MaxAllocated-1)
}

func (space *Space) addrInRange(addr net.IP) bool {
	offset := utils.Subtract(addr, space.Start)
	return offset >= 0 && offset < int64(space.Size)
}

// Free takes an IP in this space and record it as avalible.
func (space *Space) Free(addr net.IP) error {
	space.assertInvariants()
	defer space.assertInvariants()

	if !space.addrInRange(addr) {
		return fmt.Errorf("Free out of range: %s", addr)
	}

	offset := utils.Subtract(addr, space.Start)
	if offset >= int64(space.MaxAllocated) {
		return fmt.Errorf("IP address not allocated: %s", addr)
	}

	if space.freelist.find(addr) >= 0 {
		return fmt.Errorf("Duplicate free: %s", addr)
	}
	space.freelist.add(addr)
	space.drainFreeList()
	return nil
}

// drainFreeList takes any contiguous addresses at the end
// of the allocated address space and removes them from the
// free list, reducing the allocated address space
func (space *Space) drainFreeList() {
	for len(space.freelist) > 0 {
		end := len(space.freelist) - 1
		potential := space.freelist[end]
		offset := utils.Subtract(potential, space.Start)
		utils.Assert(space.addrInRange(potential), "Free list contains address not in range")

		// Is this potential address at the end of the allocated
		// address space?
		if offset != int64(space.MaxAllocated)-1 {
			return
		}

		space.freelist.removeAt(end)
		space.MaxAllocated--
	}
}

// assertFree asserts that the size consequtive IPs from start
// (inclusive) are not allocated
func (space *Space) assertFree(start net.IP, size uint32) {
	utils.Assert(space.contains(start), "Range outside my care")
	utils.Assert(space.contains(utils.Add(start, size-1)), "Range outside my care")

	// Is this range wholly outside the free list?
	// if start == s.Start; offset = 0; MaxAllocated
	// if both next free and count of allocated.
	offset := uint32(utils.Subtract(space.Start, start))
	if offset >= space.MaxAllocated {
		return
	}

	for i := offset; i < offset+size; i++ {
		if i >= space.MaxAllocated {
			return
		}

		utils.Assert(space.freelist.find(utils.Add(start, i)) != -1, "Address in use!")
	}
}

// BiggestFreeChunk scans the freelist and returns the
// start, length of the largest free range of address it
// can find.
func (space *Space) BiggestFreeChunk() (net.IP, uint32) {
	space.assertInvariants()
	defer space.assertInvariants()

	// First, drain the free list
	space.drainFreeList()

	// Keep a track of the current chunk start and size
	// First chunk we've found is the one of unallocated space
	chunkStart := utils.Add(space.Start, space.MaxAllocated)
	chunkSize := space.Size - space.MaxAllocated

	// Now scan the free list of other chunks
	for i := 0; i < len(space.freelist); {
		// We know we have a chunk of at least one
		potentialStart := space.freelist[i]
		potentialSize := uint32(1)

		// Can we grow this chunk one by one
		curr := space.freelist[i]
		i++
		for ; i < len(space.freelist); i++ {
			if utils.Subtract(space.freelist[i], curr) > 1 {
				break
			}

			curr = space.freelist[i]
			potentialSize++
		}

		// Is the chunk we found bigger than the
		// one we already have?
		if potentialSize > chunkSize {
			chunkStart = potentialStart
			chunkSize = potentialSize
		}
	}

	// Now return what we found
	if chunkSize == 0 {
		return nil, 0
	}

	space.assertFree(chunkStart, chunkSize)
	return chunkStart, chunkSize
}

// Grow increases the size of this space to size.
func (space *Space) Grow(size uint32) {
	utils.Assert(space.Size < size, "Cannot shrink a space!")
	space.Size = size
}

// NumFreeAddresses returns the total number of free addressed in
// this space.
func (space *Space) NumFreeAddresses() uint32 {
	return space.Size - space.MaxAllocated + uint32(len(space.freelist))
}

func (space *Space) String() string {
	return fmt.Sprintf("%s+%d (%d/%d)", space.Start, space.Size, space.MaxAllocated, len(space.freelist))
}

// Split divide this space into two new spaces at a given address, copying allocations and frees.
func (space *Space) Split(addr net.IP) (*Space, *Space) {
	utils.Assert(space.contains(addr), "Splitting around a point not in the space!")
	breakpoint := utils.Subtract(addr, space.Start)
	ret1 := &Space{Start: space.Start, Size: uint32(breakpoint)}
	ret2 := &Space{Start: addr, Size: space.Size - uint32(breakpoint)}

	// find the max-allocated point for each sub-space
	if space.MaxAllocated > uint32(breakpoint) {
		ret1.MaxAllocated = ret1.Size
		ret2.MaxAllocated = space.MaxAllocated - ret1.Size
	} else {
		ret1.MaxAllocated = space.MaxAllocated
		ret2.MaxAllocated = 0
	}

	// Now copy the free list, but omit anything above MaxAllocated in each case
	for _, alloc := range space.freelist {
		offset := utils.Subtract(alloc, addr)
		if offset < 0 {
			if uint32(offset+breakpoint) < ret1.MaxAllocated {
				ret1.freelist.add(alloc)
			}
		} else {
			if uint32(offset) < ret2.MaxAllocated {
				ret2.freelist.add(alloc)
			}
		}
	}

	ret1.drainFreeList()
	ret2.drainFreeList()

	return ret1, ret2
}
