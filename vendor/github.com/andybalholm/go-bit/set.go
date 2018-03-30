// Copyright 2012 Stefan Nilsson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bit provides a bitset implementation and utility bit functions for uint64 words.
package bit

import (
	"bytes"
	"strconv"
)

const (
	bpw   = 64         // bits per word
	maxw  = 1<<bpw - 1 // maximum value for a word
	shift = 6
	mask  = 0x3f
)

// Set represents a mutable set of non-negative integers.
// The zero value of Set is an empty set ready to use.
// A set occupies approximately n bits, where n is the maximum
// value that has been stored in the set.
type Set struct {
	// Invariants:
	//	• data[n>>shift] & (1<<(n&mask)) == 1 iff n belongs to set,
	// 	• data[i] == 0 for all i such that len(data) ≤ i < cap(data),
	//	• len(data) == 0 if set is empty,
	//	• data[len(data)-1] != 0 if set is nonempty,
	//	• min is the minimum element of a nonempty set, 0 otherwise.
	data []uint64
	min  int
}

// New creates a new set S with the given elements.
func New(n ...int) (S *Set) {
	if len(n) == 0 {
		return new(Set)
	}

	min, max := n[0], n[0]
	for _, e := range n {
		switch {
		case e > max:
			max = e
		case e < min:
			min = e
		}
	}

	S = &Set{
		data: make([]uint64, max>>shift+1),
		min:  min,
	}

	for _, e := range n {
		S.data[e>>shift] |= 1 << uint(e&mask)
	}
	return
}

// Add adds n to S, setting S to S ∪ {n}, and returns the updated set.
func (S *Set) Add(n int) *Set {
	if n < 0 {
		panic("Set: Add(" + strconv.Itoa(n) + ") index out of range")
	}

	if len(S.data) == 0 || n < S.min {
		S.min = n
	}

	i := n >> shift
	if i >= len(S.data) {
		S.resize(i + 1)
	}
	S.data[i] |= 1 << uint(n&mask)
	return S
}

// AddRange adds all integers between m and n-1, m ≤ n,
// setting S to S ∪ {m..n-1}, and returns the updated set.
func (S *Set) AddRange(m, n int) *Set {
	if m == n && m >= 0 {
		return S
	}

	if m > n || m < 0 {
		panic("Set: AddRange(" + strconv.Itoa(m) + ", " + strconv.Itoa(n) + ") bounds out of range")
	}

	if len(S.data) == 0 || m < S.min {
		S.min = m
	}

	n--
	low, high := m>>shift, n>>shift
	if high >= len(S.data) {
		S.resize(high + 1)
	}

	d := S.data
	if low == high { // Range fits in one word.
		d[low] |= bitMask(m&mask, n&mask)
	} else { // Range spans at least two words.
		d[low] |= bitMask(m&mask, bpw-1)
		for i := low + 1; i < high; i++ {
			d[i] = maxw
		}
		d[high] |= bitMask(0, n&mask)
	}
	return S
}

// Remove removes n from S, setting S to S ∖ {n}, and returns the updated set.
func (S *Set) Remove(n int) *Set {
	if n < 0 {
		panic("Set: Remove(" + strconv.Itoa(n) + ") index out of range")
	}

	if n < S.min {
		return S
	}

	i := n >> shift
	if i >= len(S.data) {
		return S
	}

	S.data[i] &^= 1 << uint(n&mask)
	S.trim()

	if n == S.min {
		S.min = findMinFrom(S.min, S.data)
	}
	return S
}

// RemoveMin removes the minimum element from S, setting S to S ∖ {min},
// and returns min. It panics if S is empty.
func (S *Set) RemoveMin() (min int) {
	if len(S.data) == 0 {
		panic("Set: RemoveMin not defined for empty set")
	}

	min = S.min
	i := min >> shift
	d := S.data

	d[i] &^= 1 << uint(min&mask)

	if d[i] != 0 || i+1 < len(d) { // There are more elements left.
		S.min = findMinFrom(min, d)
	} else {
		S.Clear() // The array contains only zeroes.
	}
	return
}

// RemoveMax removes the maximum element from S, setting S to S ∖ {max},
// and returns max. It panics if S is empty.
func (S *Set) RemoveMax() (max int) {
	if len(S.data) == 0 {
		panic("Set: RemoveMax not defined for empty set")
	}

	d := S.data
	i := len(d) - 1
	w := d[i]

	max = i<<shift + MaxPos(w)
	d[i] &^= 1 << uint(max&mask)
	S.trim()

	return
}

// Min returns the minimum element of the set. It panics if S is empty.
func (S *Set) Min() int {
	if len(S.data) == 0 {
		panic("Set: Min not defined for empty set")
	}

	return S.min
}

// Max returns the maximum element of the set. It panics if S is empty.
func (S *Set) Max() int {
	if len(S.data) == 0 {
		panic("Set: Max not defined for empty set")
	}

	d := S.data
	i := len(d) - 1
	w := d[i]
	return i<<shift + MaxPos(w)
}

// Next returns (n, true), where n is the smallest element of S such that m < n,
// or (0, false) if no such element exists.
func (S *Set) Next(m int) (n int, found bool) {
	d := S.data
	len := len(d)

	if len == 0 {
		return
	}

	if min := S.min; m < min {
		return min, true
	}

	i := len - 1
	if max := i<<shift + MaxPos(d[i]); m >= max {
		return
	}

	i = m >> shift
	s := 1 + uint(m&mask)
	w := d[i] >> s << s // Zero out bits for numbers ≤ m.
	for w == 0 {
		i++
		w = d[i]
	}
	return i<<shift + MinPos(w), true
}

// Previous returns (n, true), where n is the largest element of S such that n < m,
// or (0, false) if no such element exists.
func (S *Set) Previous(m int) (n int, found bool) {
	d := S.data
	len := len(d)

	if len == 0 || m <= S.min {
		return
	}

	i := len - 1
	if max := i<<shift + MaxPos(d[i]); m > max {
		return max, true
	}

	i = m >> shift
	s := bpw - uint(m&mask)
	w := d[i] << s >> s // Zero out bits for numbers ≥ m.
	for w == 0 {
		i--
		w = d[i]
	}
	return i<<shift + MaxPos(w), true
}

// RemoveRange removes all integers between m and n-1, m ≤ n,
// setting S to S ∖ {m..n-1}, and returns the updated set.
func (S *Set) RemoveRange(m, n int) *Set {
	if m == n && m >= 0 {
		return S
	}

	if m > n || m < 0 {
		panic("Set: RemoveRange(" + strconv.Itoa(m) + ", " + strconv.Itoa(n) + ") bounds out of range")
	}

	n--
	d := S.data
	low, high := m>>shift, n>>shift

	// Range does not intersect S.
	if n < S.min || low >= len(d) {
		return S
	}

	// Bottom of range undershoots S.
	if m < S.min {
		low = S.min >> shift // low ≤ high still holds, since S.min ≤ n.
		m = 0                // To assure that m&mask == 0 below.
	}

	// Top of range overshoots S.
	if len(d) <= high {
		high = len(d) - 1 // low ≤ high still holds, since low < len(d).
		n = bpw - 1       // To assure that n&mask == bpw-1 below.
	}

	if low == high { // Range fits in one word
		d[low] &^= bitMask(m&mask, n&mask)
	} else { // Range spans at least two words
		d[low] &^= bitMask(m&mask, bpw-1)
		for i := low + 1; i < high; i++ {
			d[i] = 0
		}
		d[high] &^= bitMask(0, n&mask)
	}
	S.trim()

	if m <= S.min {
		S.min = findMinFrom(S.min, S.data)
	}
	return S
}

// Clear removes all elements and returns the updated empty set.
func (S *Set) Clear() *Set {
	S.realloc(0)
	S.min = 0
	return S
}

// Flip removes n from S if it is present, otherwise it adds n,
// setting S to S ∆ {n}, and returns the updated set.
func (S *Set) Flip(n int) *Set {
	if n < 0 {
		panic("Set: Flip(" + strconv.Itoa(n) + ") index out of range")
	}

	if len(S.data) == 0 { // Set correct minimum when S empty.
		S.min = n
	}

	i := n >> shift
	if i >= len(S.data) {
		S.resize(i + 1)
	}
	S.data[i] ^= 1 << uint(n&mask)
	S.trim()

	if n <= S.min {
		// If flip removed S.min, we must search for a new minimum.
		// If n < S.min, the search will find n right away.
		S.min = findMinFrom(n, S.data)
	}
	return S
}

// FlipRange flips all elements in the range m..n-1, m ≤ n,
// setting S to S ∆ {m..n-1}, and returns the updated set.
func (S *Set) FlipRange(m, n int) *Set {
	if m == n && m >= 0 {
		return S
	}

	if m > n || m < 0 {
		panic("Set: FlipRange(" + strconv.Itoa(m) + ", " + strconv.Itoa(n) + ") bounds out of range")
	}

	d := S.data
	if len(d) == 0 { // Set correct minimum when S empty.
		S.min = m
	}

	n--
	low, high := m>>shift, n>>shift
	if high >= len(d) {
		S.resize(high + 1)
		d = S.data
	}

	if low == high { // Range fits in one word.
		d[low] ^= bitMask(m&mask, n&mask)
	} else { // Range spans at least two words.
		d[low] ^= bitMask(m&mask, bpw-1)
		for i := low + 1; i < high; i++ {
			d[i] ^= maxw
		}
		d[high] ^= bitMask(0, n&mask)
	}
	S.trim()

	// If S.min < m, the minimum does not change.
	if m <= S.min {
		// The new minimum can't be smaller than m.
		S.min = findMinFrom(m, S.data)
	}
	return S
}

// Contains returns true if n, n ≥ 0, is an element of S.
func (S *Set) Contains(n int) bool {
	d := S.data
	i := n >> shift

	if i >= len(d) {
		return false
	}
	return d[i]&(1<<uint(n&mask)) != 0
}

// Size returns |S|, the number of elements in S.
// This method scans the set. To check if a set is empty,
// consider using the more efficient IsEmpty.
func (S *Set) Size() int {
	d := S.data

	n := 0
	for i, l := S.min>>shift, len(d); i < l; i++ {
		if w := d[i]; w != 0 {
			n += Count(w)
		}
	}
	return n
}

// IsEmpty returns true if S = ∅.
func (S *Set) IsEmpty() bool {
	return len(S.data) == 0
}

// Equals returns true if A and B contain the same elements.
func (A *Set) Equals(B *Set) bool {
	if A == B {
		return true
	}

	if A.min != B.min {
		return false
	}

	a, b := A.data, B.data
	if len(a) != len(b) {
		return false
	}

	for i, l := A.min>>shift, len(a); i < l; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SubsetOf returns true if A ⊆ B.
func (A *Set) SubsetOf(B *Set) bool {
	if A == B {
		return true
	}

	a, b := A.data, B.data
	if len(a) > len(b) {
		return false
	}

	if len(a) != 0 && A.min < B.min {
		return false
	}

	for i, l := A.min>>shift, len(a); i < l; i++ {
		if a[i]&^b[i] != 0 {
			return false
		}
	}
	return true
}

// Intersects returns true if A and B overlap, i.e. A ∩ B ≠ ∅.
func (A *Set) Intersects(B *Set) bool {
	a, b := A.data, B.data

	if len(a) == 0 || len(b) == 0 {
		return false
	}

	if A == B { // Notice that both sets are nonempty.
		return true
	}

	m := min(len(a), len(b))
	for i := max(A.min, B.min) >> shift; i < m; i++ {
		if a[i]&b[i] != 0 {
			return true
		}
	}
	return false
}

// And creates a new intersection S = A ∩ B that consists of all
// elements that belong to both A and B.
func (A *Set) And(B *Set) (S *Set) {
	return new(Set).SetAnd(A, B)
}

// Or creates a new union S = A ∪ B that contains all
// elements that belong to either A or B.
func (A *Set) Or(B *Set) (S *Set) {
	return new(Set).SetOr(A, B)
}

// AndNot creates a new set difference S = A ∖ B that
// consists of all elements that belong to A, but not to B.
func (A *Set) AndNot(B *Set) (S *Set) {
	return new(Set).SetAndNot(A, B)
}

// Xor creates a new symmetric difference S = A ∆ B = (A ∪ B) ∖ (A ∩ B)
// that consists of all elements that belong to either A or B, but not to both.
func (A *Set) Xor(B *Set) (S *Set) {
	return new(Set).SetXor(A, B)
}

// SetWord interprets w as a bitset with numbers in the range 64i to 64i + 63,
// where 0 ≤ i ≤ ⌊MaxInt/64⌋, overwrites this range in S with w, and returns S.
func (S *Set) SetWord(i int, w uint64) *Set {
	if i > MaxInt/64 {
		panic("Set: SetWord(" + strconv.Itoa(i) + ", 0x" + strconv.FormatUint(w, 16) +
			") index out of range")
	}

	if w == 0 && i >= len(S.data) {
		return S
	}

	if len := len(S.data); i >= len {
		if len == 0 { // Set tentative minimum when S is empty.
			S.min = 64 * i
		}
		S.resize(i + 1)
	}

	S.data[i] = w

	if w == 0 && i == len(S.data)-1 {
		S.trim()
	}

	if mi := S.min >> shift; i < mi && w != 0 || i == mi {
		S.min = findMinFrom(64*i, S.data)
	}
	return S
}

// Word returns the range 64i to 64i + 63 of S as a bitset represented by w.
func (S *Set) Word(i int) (w uint64) {
	if i < len(S.data) {
		w = S.data[i]
	}
	return
}

// Set sets S to A and returns S.
func (S *Set) Set(A *Set) *Set {
	S.realloc(len(A.data))
	low := min(A.min>>shift, S.min>>shift) // Words below this index are all zero,
	copy(S.data[low:], A.data[low:])       // so there is no need to copy them.
	S.min = A.min
	return S
}

// SetAnd sets S to the intersection A ∩ B and returns S.
func (S *Set) SetAnd(A, B *Set) *Set {
	a, b := A.data, B.data
	la, lb := len(a), len(b)
	ma, mb, ms := A.min, B.min, S.min

	// Check if ranges overlap.
	if la == 0 || lb == 0 || la <= mb>>shift || lb <= ma>>shift {
		return S.Clear()
	}

	// S.data[i], i < low, are words that should be zero and already are.
	low := min(max(ma, mb), ms) >> shift

	// Find last nonzero word in result.
	i := min(la, lb) - 1
	for i >= 0 && a[i]&b[i] == 0 {
		i--
	}
	if S == A || S == B {
		S.resize(i + 1)
	} else {
		S.realloc(i + 1)
	}

	for ; i >= low; i-- {
		S.data[i] = a[i] & b[i]
	}

	S.min = findMinFrom(max(ma, mb), S.data)
	return S
}

// SetAndNot sets S to the set difference A ∖ B and returns S.
func (S *Set) SetAndNot(A, B *Set) *Set {
	a, b := A.data, B.data
	la, lb := len(a), len(b)
	ma, mb, ms := A.min, B.min, S.min

	// Check if ranges overlap.
	if la == 0 || lb == 0 || la <= mb>>shift || lb <= ma>>shift {
		return S.Set(A)
	}

	// S.data[i], i < low, are words that should be zero and already are.
	low := min(ma, ms) >> shift

	// Result requires len(a) words if len(a) > len(b),
	// otherwise find last nonzero word in result.
	i := la - 1
	if la <= lb {
		for i >= 0 && a[i]&^b[i] == 0 {
			i--
		}
	}
	if S == A || S == B {
		S.resize(i + 1)
	} else {
		S.realloc(i + 1)
	}

	d := S.data
	if m := max(lb, low); m <= i {
		copy(d[m:i+1], a[m:i+1])
		i = m - 1
	}

	for ; i >= low; i-- {
		d[i] = a[i] &^ b[i]
	}

	if A.min < B.min { // Minimum can't have changed.
		S.min = A.min
	} else {
		S.min = findMinFrom(ma, d) // can't be smaller than previous A.min.
	}
	return S
}

// SetOr sets S to the union A ∪ B and returns S.
func (S *Set) SetOr(A, B *Set) *Set {
	// Swap, if necessary, to make A shorter than B.
	if len(A.data) > len(B.data) {
		A, B = B, A
	}
	a, b := A.data, B.data
	la, lb := len(a), len(b)
	ma, mb, ms := A.min, B.min, S.min

	i := lb - 1
	if S == A || S == B {
		S.resize(i + 1)
	} else {
		S.realloc(i + 1)
	}

	d := S.data
	copy(d[la:i+1], b[la:i+1])
	i = la - 1

	// S.data[i], i < low, are words that should be zero and already are.
	low := min(min(ma, mb), ms) >> shift
	for ; i >= low; i-- {
		d[i] = a[i] | b[i]
	}

	if la == 0 {
		S.min = mb
	} else if lb == 0 {
		S.min = ma
	} else {
		S.min = min(ma, mb)
	}
	return S
}

// SetXor sets S to the symmetric difference A ∆ B = (A ∪ B) ∖ (A ∩ B)
// and returns S.
func (S *Set) SetXor(A, B *Set) *Set {
	// Swap, if necessary, to make A shorter than B.
	if len(A.data) > len(B.data) {
		A, B = B, A
	}
	a, b := A.data, B.data
	la, lb := len(a), len(b)
	ma, mb, ms := A.min, B.min, S.min

	i := lb - 1
	if la == lb { // The only case where result may be shorter than len(b).
		low := min(ma, mb) >> shift
		for i >= low && a[i]^b[i] == 0 {
			i--
		}
		if i == low-1 { // Only zero elements left.
			return S.Clear()
		}
	}

	if S == A || S == B {
		S.resize(i + 1)
	} else {
		S.realloc(i + 1)
	}

	d := S.data
	if la <= i {
		copy(d[la:i+1], b[la:i+1])
		i = la - 1
	}

	// S.data[i], i < low, are words that should be zero and already are.
	low := min(min(ma, mb), ms) >> shift
	for ; i >= low; i-- {
		d[i] = a[i] ^ b[i]
	}

	S.min = findMinFrom(min(ma, mb), d)
	return S
}

// Do calls function f for each element n ∊ S in numerical order.
// It is safe for f to add or remove elements e, e ≤ n, from S.
// The behavior of Do is undefined if f changes the set in any other way.
func (S *Set) Do(f func(n int)) {
	d := S.data

	for i, l := S.min>>shift, len(d); i < l; i++ {
		w := d[i]
		if w == 0 {
			continue
		}
		n := i << shift // element represented by w&1
		for w != 0 {
			b := MinPos(w)
			n += b
			f(n)
			n++
			w >>= uint(b + 1)
			for w&1 != 0 { // common case
				f(n)
				n++
				w >>= 1
			}
		}
	}
}

// String returns a string representation of S.
// The elements are listed in ascending order, enclosed by braces,
// and separated by ", ". Runs of at least three elements a, a+1, ..., b
// are given as "a..b".
//
// For example, the set {1, 2, 6, 5, 3} is represented as "{1..3, 5, 6}".
func (S *Set) String() string {
	sb := new(bytes.Buffer)
	sb.WriteString("{")

	a, b := -1, -2 // keeps track of a range a..b of elements
	S.Do(func(n int) {
		if n == b+1 { // Increase current range from a..b to a..b+1.
			b = n
			return
		}
		writeRange(sb, a, b)
		a, b = n, n // Start new range.
	})
	writeRange(sb, a, b)

	if S.Size() > 0 {
		sb.Truncate(sb.Len() - 2) // Remove trailing ", ".
	}
	sb.WriteString("}")
	return sb.String()
}

// writeRange writes either "", "a", "a, b, " or "a..b, " to buffer.
func writeRange(sb *bytes.Buffer, a, b int) {
	switch {
	case a > b:
		// sb.WriteString(sb, "")
	case a == b:
		sb.WriteString(strconv.Itoa(a))
		sb.WriteString(", ")
	case a+1 == b:
		sb.WriteString(strconv.Itoa(a))
		sb.WriteString(", ")
		sb.WriteString(strconv.Itoa(b))
		sb.WriteString(", ")
	default:
		sb.WriteString(strconv.Itoa(a))
		sb.WriteString("..")
		sb.WriteString(strconv.Itoa(b))
		sb.WriteString(", ")
	}
}

// resize changes the length of S.data to n, keeping old values.
// It preserves the invariant S.data[i] = 0, n ≤ i < cap(data).
func (S *Set) resize(n int) {
	d := S.data

	if S.realloc(n) {
		low := min(S.min>>shift, len(d)) // Words below S.min>>shift are zero;
		copy(S.data[low:], d[low:])      // there is no need to copy them.
	}
}

// realloc creates a slice S.data of length n, possibly zeroing out old values.
// It preserves the invariant S.data[i] = 0, n ≤ i < cap(data).
// It returns true if new memory has been allocated.
func (S *Set) realloc(n int) (didAlloc bool) {
	if c := cap(S.data); c < n {
		S.data = make([]uint64, n, newCap(n, c))
		return true
	}

	// Add zeroes if shrinking.
	d := S.data
	low := max(n, S.min>>shift) // The words d[i], i < low, are already zero.
	for i := len(d) - 1; i >= low; i-- {
		d[i] = 0
	}
	S.data = d[:n]
	return false
}

// newCap suggests a new increased capacity, favoring powers of two,
// when growing a slice to length n. The suggested capacities guarantee
// linear amortized cost for repeated memory allocations.
func newCap(n, prevCap int) int {
	return max(n, nextPow2(prevCap))
}

// nextPow2 returns the smallest p = 1, 2, 4, ..., 2^k such that p > n,
// or MaxInt if p > MaxInt.
func nextPow2(n int) (p int) {
	if n <= 0 {
		return 1
	}

	if k := MaxPos(uint64(n)) + 1; k < BitsPerWord-1 {
		return 1 << uint(k)
	}
	return MaxInt
}

// trim slices S.data by removing all trailing words equal to zero.
// It preserves the invariant: S.min = min(S) if S ≠ ∅, 0 otherwise.
func (S *Set) trim() {
	d := S.data
	i := len(d) - 1
	for i >= 0 && d[i] == 0 {
		i--
	}
	S.data = d[:i+1]
	if i == -1 {
		S.min = 0
	}
}

// findMinFrom finds the mininum element starting the search from n.
// It assumes that such an element exists; however, it returns 0 if
// len(data) == 0.
func findMinFrom(n int, data []uint64) int {
	if len(data) == 0 {
		return 0
	}

	i := n >> shift
	w := data[i]
	for w == 0 {
		i++
		w = data[i]
	}
	return i<<shift + MinPos(w)
}

// bitMask returns a bit mask with nonzero bits from m to n, 0 ≤ m ≤ n < bpw.
func bitMask(m, n int) uint64 {
	return maxw >> uint(bpw-1-(n-m)) << uint(m)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
