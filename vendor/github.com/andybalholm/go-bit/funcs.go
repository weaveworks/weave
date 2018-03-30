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

package bit

// Note the use of << to create an untyped constant.
const bitsPerWord = 32 << uint(^uint(0)>>63)

// Implementation-specific size of int and uint in bits.
const BitsPerWord = bitsPerWord // either 32 or 64

// Implementation-specific integer limit values.
const (
	MaxInt  = 1<<(BitsPerWord-1) - 1 // either 1<<31 - 1 or 1<<63 - 1
	MinInt  = -MaxInt - 1            // either -1 << 31 or -1 << 63
	MaxUint = 1<<BitsPerWord - 1     // either 1<<32 - 1 or 1<<64 - 1
)

// MinPos returns the position of the minimum nonzero bit in w, w ≠ 0.
// It panics for w = 0.
func MinPos(w uint64) int {
	// “Using de Bruijn Sequences to Index a 1 in a Computer Word”,
	// Leiserson, Prokop, and Randall, MIT, 1998.
	if w == 0 {
		panic("bit: MinPos(0) undefined")
	}

	// w & -w clears all bits except the one at minimum position p.
	// Hence, the multiplication below is equivalent to b26<<p.
	// A table lookup translates the 64 possible outcomes into
	// the exptected answer.
	return bitPos[((w&-w)*b26)>>58]
}

// A sequence, starting with 6 zeros, that contains all possible
// 6-bit patterns as subseqences, a.k.a. De Bruijn B(2, 6).
const b26 uint64 = 0x022fdd63cc95386d

var bitPos [64]int

func init() {
	for p := uint(0); p < 64; p++ {
		bitPos[b26<<p>>58] = int(p)
	}
}

// MaxPos returns the position of the maximum nonzero bit in w, w ≠ 0.
// It panics for w = 0.
func MaxPos(w uint64) int {
	if w == 0 {
		panic("bit: MaxPos(0) undefined")
	}

	// Fill word with ones on the right, e.g. 0x0000f308 -> 0x0000ffff.
	w |= w >> 1
	w |= w >> 2
	w |= w >> 4
	w |= w >> 8
	w |= w >> 16
	w |= w >> 32
	return Count(w) - 1
}

// Count returns the number of nonzero bits in w.
func Count(w uint64) int {
	// “Software Optimization Guide for AMD64 Processors”, Section 8.6.
	const maxw = 1<<64 - 1
	const bpw = 64

	// Compute the count for each 2-bit group.
	// Example using 16-bit word w = 00,01,10,11,00,01,10,11
	// w - (w>>1) & 01,01,01,01,01,01,01,01 = 00,01,01,10,00,01,01,10
	w -= (w >> 1) & (maxw / 3)

	// Add the count of adjacent 2-bit groups and store in 4-bit groups:
	// w & 0011,0011,0011,0011 + w>>2 & 0011,0011,0011,0011 = 0001,0011,0001,0011
	w = w&(maxw/15*3) + (w>>2)&(maxw/15*3)

	// Add the count of adjacent 4-bit groups and store in 8-bit groups:
	// (w + w>>4) & 00001111,00001111 = 00000100,00000100
	w += w >> 4
	w &= maxw / 255 * 15

	// Add all 8-bit counts with a multiplication and a shift:
	// (w * 00000001,00000001) >> 8 = 00001000
	w *= maxw / 255
	w >>= (bpw/8 - 1) * 8
	return int(w)
}
