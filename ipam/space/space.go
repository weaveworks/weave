package space

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
)

type Addr address.Address

type Space struct {
	// ours and free represent a set of addresses as a sorted
	// sequences of ranges.  Even elements give the inclusive
	// starting points of ranges, and odd elements give the
	// exclusive ending points.  Ranges in an array do not
	// overlap, and neighbouring ranges are always coalesced if
	// possible, so the arrays consist of sorted Addrs without
	// repetition.
	ours []address.Address
	free []address.Address
}

func New() *Space {
	return &Space{}
}

func (s *Space) Add(start address.Address, size address.Offset) {
	s.free = add(s.free, start, address.Add(start, size))
}

// Clear removes all spaces from this space set.  Used during node shutdown.
func (s *Space) Clear() {
	s.free = s.free[:0]
	s.ours = s.ours[:0]
}

func (s *Space) Allocate() (bool, address.Address) {
	if len(s.free) == 0 {
		return false, 0
	} else {
		res := s.free[0]
		s.ours = add(s.ours, res, res+1)
		s.free = subtract(s.free, res, res+1)
		return true, res
	}
}

func (s *Space) Claim(addr address.Address) error {
	if !contains(s.free, addr) {
		return fmt.Errorf("Address %v is not free to claim", addr)
	}

	s.ours = add(s.ours, addr, addr+1)
	s.free = subtract(s.free, addr, addr+1)
	return nil
}

func (s *Space) NumFreeAddresses() address.Offset {
	res := address.Offset(0)
	for i := 0; i < len(s.free); i += 2 {
		res += address.Subtract(s.free[i+1], s.free[i])
	}
	return res
}

func (s *Space) NumFreeAddressesInRange(start, end address.Address) address.Offset {
	res := address.Offset(0)
	for i := 0; i < len(s.free); i += 2 {
		s, e := s.free[i], s.free[i+1]
		if s < start {
			s = start
		}
		if e > end {
			e = end
		}
		if s >= e {
			continue
		}
		res += address.Subtract(e, s)
	}
	return res
}

func (s *Space) Free(addr address.Address) error {
	if !contains(s.ours, addr) {
		return fmt.Errorf("Address %v is not ours", addr)
	}
	if contains(s.free, addr) {
		return fmt.Errorf("Address %v is already free", addr)
	}

	s.ours = subtract(s.ours, addr, addr+1)
	s.free = add(s.free, addr, addr+1)
	return nil
}

func (s *Space) biggestFreeRange() (int, address.Offset) {
	pos := -1
	biggest := address.Offset(0)

	for i := 0; i < len(s.free); i += 2 {
		size := address.Subtract(s.free[i+1], s.free[i])
		if size >= biggest {
			pos = i
			biggest = size
		}
	}
	return pos, biggest
}

func (s *Space) Donate() (address.Address, address.Offset, bool) {
	if len(s.free) == 0 {
		return 0, 0, false
	}

	pos, biggest := s.biggestFreeRange()

	// Donate half of that biggest free range, rounding up so
	// that the donation can't be empty
	end := s.free[pos+1]
	start := end - address.Address((biggest+1)/2)

	s.ours = subtract(s.ours, start, end)
	s.free = subtract(s.free, start, end)
	return start, address.Subtract(end, start), true
}

func firstGreater(a []address.Address, x address.Address) int {
	return sort.Search(len(a), func(i int) bool { return a[i] > x })
}

func firstGreaterOrEq(a []address.Address, x address.Address) int {
	return sort.Search(len(a), func(i int) bool { return a[i] >= x })
}

// Do the ranges contain the given address?
func contains(addrs []address.Address, addr address.Address) bool {
	return firstGreater(addrs, addr)&1 != 0
}

// Take the union of the range [start, end) with the ranges in the array
func add(addrs []address.Address, start address.Address, end address.Address) []address.Address {
	return addSub(addrs, start, end, 0)
}

// Subtract the range [start, end) from the ranges in the array
func subtract(addrs []address.Address, start address.Address, end address.Address) []address.Address {
	return addSub(addrs, start, end, 1)
}

func addSub(addrs []address.Address, start address.Address, end address.Address, sense int) []address.Address {
	start_pos := firstGreaterOrEq(addrs, start)
	end_pos := firstGreater(addrs[start_pos:], end) + start_pos

	// Boundaries up to start_pos are unaffected
	res := make([]address.Address, start_pos, len(addrs)+2)
	copy(res, addrs)

	// Include start and end as new boundaries if they lie
	// outside/inside existing ranges (according to sense).
	if start_pos&1 == sense {
		res = append(res, start)
	}

	if end_pos&1 == sense {
		res = append(res, end)
	}

	// Boundaries after end_pos are unaffected
	return append(res, addrs[end_pos:]...)
}

func (s *Space) String() string {
	var buf bytes.Buffer
	if len(s.ours) > 0 {
		fmt.Fprint(&buf, "owned:")
		for i := 0; i < len(s.ours); i += 2 {
			fmt.Fprintf(&buf, " %s+%d ", s.ours[i], s.ours[i+1]-s.ours[i])
		}
	}
	if len(s.free) > 0 {
		fmt.Fprintf(&buf, "free:")
		for i := 0; i < len(s.free); i += 2 {
			fmt.Fprintf(&buf, " %s+%d ", s.free[i], s.free[i+1]-s.free[i])
		}
	}
	if len(s.ours) == 0 && len(s.free) == 0 {
		fmt.Fprintf(&buf, "No address ranges owned")
	}
	return buf.String()
}

type addressSlice []address.Address

func (p addressSlice) Len() int           { return len(p) }
func (p addressSlice) Less(i, j int) bool { return p[i] < p[j] }
func (p addressSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (s *Space) assertInvariants() {
	common.Assert(sort.IsSorted(addressSlice(s.ours)))
	common.Assert(sort.IsSorted(addressSlice(s.free)))
}

// Return a slice representing everything we own, whether it is free or not
func (s *Space) everything() []address.Address {
	a := make([]address.Address, len(s.ours))
	copy(a, s.ours)
	for i := 0; i < len(s.free); i += 2 {
		a = add(a, s.free[i], s.free[i+1])
	}
	return a
}

// OwnedRanges returns slice of Ranges, ordered by IP, gluing together
// contiguous sequences of owned and free addresses
func (s *Space) OwnedRanges() []address.Range {
	everything := s.everything()
	result := make([]address.Range, len(everything)/2)
	for i := 0; i < len(everything); i += 2 {
		result[i/2] = address.Range{Start: everything[i], End: everything[i+1]}
	}
	return result
}

// Create a Space that has free space in all the supplied Ranges.
func (s *Space) AddRanges(ranges []address.Range) {
	for _, r := range ranges {
		s.free = add(s.free, r.Start, r.End)
	}
}

// Taking ranges to be a set of all space we should own, add in any excess as free space
func (s *Space) UpdateRanges(ranges []address.Range) {
	new := []address.Address{}
	for _, r := range ranges {
		new = add(new, r.Start, r.End)
	}
	current := s.everything()
	for i := 0; i < len(current); i += 2 {
		new = subtract(new, current[i], current[i+1])
	}
	for i := 0; i < len(new); i += 2 {
		s.free = add(s.free, new[i], new[i+1])
	}
}
