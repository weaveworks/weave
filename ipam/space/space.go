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

// Walk down the free list calling f() on the in-range portions, until
// f() returns true or we run out of free space.  Return true iff f() returned true
func (s *Space) walkFree(rangeStart, rangeEnd address.Address, f func(start, end address.Address) bool) bool {
	if rangeStart >= rangeEnd { // degenerate case
		return false
	}
	for i := 0; i < len(s.free); i += 2 {
		start, end := s.free[i], s.free[i+1]
		if end <= rangeStart { // this chunk comes before the range
			continue
		}
		if start >= rangeEnd {
			// all remaining free space is completely after range
			break
		}
		// at this point we know end>start && end>rangeStart && rangeEnd>start && rangeEnd>rangeStart
		// therefore max(start, rangeStart) < min(end, rangeEnd)
		// Restrict this block of free space to be in range
		if start < rangeStart {
			start = rangeStart
		}
		if end > rangeEnd {
			end = rangeEnd
		}
		// at this point we know start<end
		if f(start, end) {
			return true
		}
	}
	return false
}

func (s *Space) Allocate(rangeStart, rangeEnd address.Address) (bool, address.Address) {
	var result address.Address
	return s.walkFree(rangeStart, rangeEnd, func(start, end address.Address) bool {
		result = start
		s.ours = add(s.ours, result, result+1)
		s.free = subtract(s.free, result, result+1)
		return true
	}), result
}

func (s *Space) Claim(addr address.Address) error {
	if !contains(s.free, addr) {
		return fmt.Errorf("Address %v is not free to claim", addr)
	}

	s.ours = add(s.ours, addr, addr+1)
	s.free = subtract(s.free, addr, addr+1)
	return nil
}

func (s *Space) NumFreeAddressesInRange(rangeStart, rangeEnd address.Address) address.Offset {
	res := address.Offset(0)
	s.walkFree(rangeStart, rangeEnd, func(start, end address.Address) bool {
		res += address.Subtract(end, start)
		return false
	})
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

func (s *Space) biggestFreeRange(rangeStart, rangeEnd address.Address) (biggestStart, biggestEnd address.Address) {
	var biggestSize address.Offset
	s.walkFree(rangeStart, rangeEnd, func(start, end address.Address) bool {
		size := address.Subtract(end, start)
		if size >= biggestSize {
			biggestStart, biggestEnd = start, end
			biggestSize = size
		}
		return false
	})
	return
}

func (s *Space) Donate(rangeStart, rangeEnd address.Address) (address.Address, address.Address, bool) {
	start, end := s.biggestFreeRange(rangeStart, rangeEnd)
	size := address.Subtract(end, start)

	if size == 0 {
		return 0, 0, false
	}

	// Donate half of that biggest free range. Note size/2 rounds down, so
	// the resulting donation size rounds up, and in particular can't be empty.
	start = address.Add(start, size/2)

	s.ours = subtract(s.ours, start, end)
	s.free = subtract(s.free, start, end)
	return start, end, true
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
	startPos := firstGreaterOrEq(addrs, start)
	endPos := firstGreater(addrs[startPos:], end) + startPos

	// Boundaries up to startPos are unaffected
	res := make([]address.Address, startPos, len(addrs)+2)
	copy(res, addrs)

	// Include start and end as new boundaries if they lie
	// outside/inside existing ranges (according to sense).
	if startPos&1 == sense {
		res = append(res, start)
	}

	if endPos&1 == sense {
		res = append(res, end)
	}

	// Boundaries after endPos are unaffected
	return append(res, addrs[endPos:]...)
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
