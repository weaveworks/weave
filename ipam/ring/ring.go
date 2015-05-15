/*
Package ring implements a simple ring CRDT.
*/
package ring

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sort"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/router"
)

// Ring represents the ring itself
type Ring struct {
	Start, End address.Address // [min, max) tokens in this ring.  Due to wrapping, min == max (effectively)
	Peer       router.PeerName // name of peer owning this ring instance
	Entries    entries         // list of entries sorted by token
}

func (r *Ring) assertInvariants() {
	err := r.checkInvariants()
	if err != nil {
		panic(err.Error())
	}
}

// Errors returned by Merge
var (
	ErrNotSorted        = errors.New("Ring not sorted")
	ErrTokenRepeated    = errors.New("Token appears twice in ring")
	ErrTokenOutOfRange  = errors.New("Token is out of range")
	ErrDifferentSubnets = errors.New("IP Allocator with different subnet detected")
	ErrNewerVersion     = errors.New("Received new version for entry I own!")
	ErrInvalidEntry     = errors.New("Received invalid state update!")
	ErrEntryInMyRange   = errors.New("Received new entry in my range!")
	ErrNoFreeSpace      = errors.New("No free space found!")
	ErrTooMuchFreeSpace = errors.New("Entry reporting too much free space!")
	ErrInvalidTimeout   = errors.New("dt must be greater than 0")
	ErrNotFound         = errors.New("No entries for peer found")
)

func (r *Ring) checkInvariants() error {
	if !sort.IsSorted(r.Entries) {
		return ErrNotSorted
	}

	// Check no token appears twice
	// We know it's sorted...
	for i := 1; i < len(r.Entries); i++ {
		if r.Entries[i-1].Token == r.Entries[i].Token {
			return ErrTokenRepeated
		}
	}

	if len(r.Entries) == 0 {
		return nil
	}

	// Check tokens are in range
	if r.Entries.entry(0).Token < r.Start {
		return ErrTokenOutOfRange
	}
	if r.Entries.entry(-1).Token >= r.End {
		return ErrTokenOutOfRange
	}

	// Check all the freespaces are in range
	for i, entry := range r.Entries {
		next := r.Entries.entry(i + 1)
		distance := r.distance(entry.Token, next.Token)

		if entry.Free > distance {
			return ErrTooMuchFreeSpace
		}
	}

	return nil
}

// New creates an empty ring belonging to peer.
func New(start, end address.Address, peer router.PeerName) *Ring {
	common.Assert(start < end)

	ring := &Ring{Start: start, End: end, Peer: peer, Entries: make([]*entry, 0)}
	ring.updateExportedVariables()
	return ring
}

// TotalRemoteFree returns the approximate number of free IPs
// on other hosts.
func (r *Ring) TotalRemoteFree() address.Offset {
	result := address.Offset(0)
	for _, entry := range r.Entries {
		if entry.Peer != r.Peer {
			result += entry.Free
		}
	}
	return result
}

// Returns the distance between two tokens on this ring, dealing
// with ranges which cross the origin
func (r *Ring) distance(start, end address.Address) address.Offset {
	if end > start {
		return address.Offset(end - start)
	}

	return address.Offset((r.End - start) + (end - r.Start))
}

// GrantRangeToHost modifies the ring such that range [start, end)
// is assigned to peer.  This may insert up to two new tokens.
// Preconditions:
// - start < end
// - [start, end) must be owned by the calling peer
func (r *Ring) GrantRangeToHost(start, end address.Address, peer router.PeerName) {
	//fmt.Printf("%s GrantRangeToHost [%v,%v) -> %s\n", r.Peer, start, end, peer)

	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	// ----------------- Start of Checks -----------------

	common.Assert(start < end)
	common.Assert(r.Start <= start && start < r.End)
	common.Assert(r.Start < end && end <= r.End)
	common.Assert(len(r.Entries) > 0)

	// Look for the left-most entry greater than start, then go one previous
	// to get the right-most entry less than or equal to start
	preceedingPos := sort.Search(len(r.Entries), func(j int) bool {
		return r.Entries[j].Token > start
	})
	preceedingPos--

	// Check all tokens up to end are owned by us
	for pos := preceedingPos; pos < len(r.Entries) && r.Entries.entry(pos).Token < end; pos++ {
		common.Assert(r.Entries.entry(pos).Peer == r.Peer)
	}

	// ----------------- End of Checks -----------------

	// Free space at start is max(length of range, distance to next token)
	startFree := r.distance(start, r.Entries.entry(preceedingPos+1).Token)
	if length := r.distance(start, end); startFree > length {
		startFree = length
	}
	// Is there already a token at start, update it
	if previousEntry := r.Entries.entry(preceedingPos); previousEntry.Token == start {
		previousEntry.update(peer, startFree)
	} else {
		// Otherwise, these isn't a token here, insert a new one.
		r.Entries.insert(entry{Token: start, Peer: peer, Free: startFree})
		preceedingPos++
		// Reset free space on previous entry, which we own.
		previousEntry.update(r.Peer, r.distance(previousEntry.Token, start))
	}

	// Give all intervening tokens to the other peer
	pos := preceedingPos + 1
	for ; pos < len(r.Entries) && r.Entries.entry(pos).Token < end; pos++ {
		entry := r.Entries.entry(pos)
		entry.update(peer, entry.Free)
	}

	// There is never an entry with a token of r.End, as the end of
	// the ring is exclusive.
	if end == r.End {
		end = r.Start
	}

	//  If there is a token equal to the end of the range, we don't need to do anything further
	if _, found := r.Entries.get(end); found {
		return
	}

	// If not, we need to insert a token such that we claim this bit on the end.
	endFree := r.distance(end, r.Entries.entry(pos).Token)
	r.Entries.insert(entry{Token: end, Peer: r.Peer, Free: endFree})
}

// Merge the given ring into this ring and return any new ranges added
func (r *Ring) Merge(gossip Ring) error {
	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	// Don't panic when checking the gossiped in ring.
	// In this case just return any error found.
	if err := gossip.checkInvariants(); err != nil {
		return err
	}

	if r.Start != gossip.Start || r.End != gossip.End {
		return ErrDifferentSubnets
	}

	// Now merge their ring with yours, in a temporary ring.
	var result entries
	addToResult := func(e entry) { result = append(result, &e) }

	var mine, theirs *entry
	var previousOwner *router.PeerName
	// i is index into r.Entries; j is index into gossip.Entries
	var i, j int
	for i < len(r.Entries) && j < len(gossip.Entries) {
		mine, theirs = r.Entries[i], gossip.Entries[j]
		switch {
		case mine.Token < theirs.Token:
			addToResult(*mine)
			previousOwner = &mine.Peer
			i++
		case mine.Token > theirs.Token:
			// insert, checking that a range owned by us hasn't been split
			if previousOwner != nil && *previousOwner == r.Peer && theirs.Peer != r.Peer {
				return ErrEntryInMyRange
			}
			addToResult(*theirs)
			previousOwner = nil
			j++
		case mine.Token == theirs.Token:
			// merge
			switch {
			case mine.Version >= theirs.Version:
				if mine.Version == theirs.Version && !mine.Equal(theirs) {
					common.Debug.Printf("Error merging entries at %s - %v != %v\n", mine.Token, mine, theirs)
					return ErrInvalidEntry
				}
				addToResult(*mine)
				previousOwner = &mine.Peer
			case mine.Version < theirs.Version:
				if mine.Peer == r.Peer { // We shouldn't receive updates to our own tokens
					return ErrNewerVersion
				}
				addToResult(*theirs)
				previousOwner = nil
			}
			i++
			j++
		}
	}

	// At this point, either i is at the end of r or j is at the end
	// of gossip, so copy over the remaining entries.

	for ; i < len(r.Entries); i++ {
		mine = r.Entries[i]
		addToResult(*mine)
	}

	for ; j < len(gossip.Entries); j++ {
		theirs = gossip.Entries[j]
		if previousOwner != nil && *previousOwner == r.Peer && theirs.Peer != r.Peer {
			return ErrEntryInMyRange
		}
		addToResult(*theirs)
		previousOwner = nil
	}

	r.Entries = result
	return nil
}

// Empty returns true if the ring has no entries
func (r *Ring) Empty() bool {
	return len(r.Entries) == 0
}

// Given a slice of ranges which are all in the right order except
// possibly the last one spans zero, fix that up and return the slice
func (r *Ring) splitRangesOverZero(ranges []address.Range) []address.Range {
	if len(ranges) == 0 {
		return ranges
	}
	lastRange := ranges[len(ranges)-1]
	// if end token == start (ie last) entry on ring, we want to actually use r.End
	if lastRange.End == r.Start {
		ranges[len(ranges)-1].End = r.End
	} else if lastRange.End <= lastRange.Start {
		// We wrapped; want to split around 0
		// First shuffle everything up as we want results to be sorted
		ranges = append(ranges, address.Range{})
		copy(ranges[1:], ranges[:len(ranges)-1])
		ranges[0] = address.Range{Start: r.Start, End: lastRange.End}
		ranges[len(ranges)-1].End = r.End
	}
	return ranges
}

// OwnedRanges returns slice of Ranges, ordered by IP, indicating which
// ranges are owned by this peer.  Will split ranges which
// span 0 in the ring.
func (r *Ring) OwnedRanges() (result []address.Range) {
	r.assertInvariants()

	for i, entry := range r.Entries {
		if entry.Peer == r.Peer {
			nextEntry := r.Entries.entry(i + 1)
			result = append(result, address.Range{Start: entry.Token, End: nextEntry.Token})
		}
	}

	return r.splitRangesOverZero(result)
}

// ClaimForPeers claims the entire ring for the array of peers passed
// in.  Only works for empty rings.
func (r *Ring) ClaimForPeers(peers []router.PeerName) {
	common.Assert(r.Empty())
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	totalSize := r.distance(r.Start, r.End)
	share := totalSize/address.Offset(len(peers)) + 1
	remainder := totalSize % address.Offset(len(peers))
	pos := r.Start

	for i, peer := range peers {
		if address.Offset(i) == remainder {
			share--
			if share == 0 {
				break
			}
		}

		if e, found := r.Entries.get(pos); found {
			e.update(peer, share)
		} else {
			r.Entries.insert(entry{Token: pos, Peer: peer, Free: share})
		}

		pos += address.Address(share)
	}

	common.Assert(pos == r.End)
}

func (r *Ring) ClaimItAll() {
	r.ClaimForPeers([]router.PeerName{r.Peer})
}

func (r *Ring) FprintWithNicknames(w io.Writer, m map[router.PeerName]string) {
	fmt.Fprintf(w, "Ring [%s, %s)\n", r.Start, r.End)
	for _, entry := range r.Entries {
		nickname, found := m[entry.Peer]
		if found {
			nickname = fmt.Sprintf(" (%s)", nickname)
		}

		fmt.Fprintf(w, "  %s -> %s%s (version: %d, free: %d)\n", entry.Token,
			entry.Peer, nickname, entry.Version, entry.Free)
	}
}

func (r *Ring) String() string {
	var buffer bytes.Buffer
	r.FprintWithNicknames(&buffer, make(map[router.PeerName]string))
	return buffer.String()
}

// ReportFree is used by the allocator to tell the ring
// how many free ips are in a given range, so that ChoosePeerToAskForSpace
// can make more intelligent decisions.
func (r *Ring) ReportFree(freespace map[address.Address]address.Offset) {
	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	common.Assert(!r.Empty())
	entries := r.Entries

	// As OwnedRanges splits around the origin, we need to
	// detect that here and fix up freespace
	if free, found := freespace[r.Start]; found && entries.entry(0).Token != r.Start {
		lastToken := entries.entry(-1).Token
		prevFree, found := freespace[lastToken]
		common.Assert(found)
		freespace[lastToken] = prevFree + free
		delete(freespace, r.Start)
	}

	for start, free := range freespace {
		// Look for entry
		i := sort.Search(len(entries), func(j int) bool {
			return entries[j].Token >= start
		})

		// Are you trying to report free on space I don't own?
		common.Assert(i < len(entries) && entries[i].Token == start && entries[i].Peer == r.Peer)

		// Check we're not reporting more space than the range
		entry, next := entries.entry(i), entries.entry(i+1)
		maxSize := r.distance(entry.Token, next.Token)
		common.Assert(free <= maxSize)

		if entries[i].Free == free {
			return
		}

		entries[i].Free = free
		entries[i].Version++
	}
}

// ChoosePeerToAskForSpace chooses a weighted-random peer to ask
// for space.
func (r *Ring) ChoosePeerToAskForSpace() (result router.PeerName, err error) {
	var (
		sum               address.Offset
		totalSpacePerPeer = make(map[router.PeerName]address.Offset) // Compute total free space per peer
	)

	// iterate through tokens
	for _, entry := range r.Entries {
		// Ignore ranges with no free space
		if entry.Free <= 0 {
			continue
		}

		// Don't talk to yourself
		if entry.Peer == r.Peer {
			continue
		}

		totalSpacePerPeer[entry.Peer] += entry.Free
		sum += entry.Free
	}

	if sum == 0 {
		err = ErrNoFreeSpace
		return
	}

	// Pick random peer, weighted by total free space
	rn := rand.Int63n(int64(sum))
	for peername, space := range totalSpacePerPeer {
		rn -= int64(space)
		if rn < 0 {
			return peername, nil
		}
	}

	panic("Should never reach this point")
}

func (r *Ring) PickPeerForTransfer() router.PeerName {
	for _, entry := range r.Entries {
		if entry.Peer != r.Peer {
			return entry.Peer
		}
	}
	return router.UnknownPeerName
}

// Transfer will mark all entries associated with 'from' peer as owned by 'to' peer
// and return ranges indicating the new space we picked up
func (r *Ring) Transfer(from, to router.PeerName) (error, []address.Range) {
	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	var newRanges []address.Range
	found := false

	for i, entry := range r.Entries {
		if entry.Peer == from {
			found = true
			entry.Peer = to
			entry.Version++
			newRanges = append(newRanges, address.Range{Start: entry.Token, End: r.Entries.entry(i + 1).Token})
		}
	}

	if !found {
		return ErrNotFound, nil
	}

	return nil, r.splitRangesOverZero(newRanges)
}

// Contains returns true if addr is in this ring
func (r *Ring) Contains(addr address.Address) bool {
	return addr >= r.Start && addr < r.End
}

// Owner returns the peername which owns the range containing addr
func (r *Ring) Owner(token address.Address) router.PeerName {
	common.Assert(r.Start <= token && token < r.End)

	r.assertInvariants()
	// There can be no owners on an empty ring
	if r.Empty() {
		return router.UnknownPeerName
	}

	// Look for the right-most entry, less than or equal to token
	preceedingEntry := sort.Search(len(r.Entries), func(j int) bool {
		return r.Entries[j].Token > token
	})
	preceedingEntry--
	entry := r.Entries.entry(preceedingEntry)
	return entry.Peer
}

// Get the set of PeerNames mentioned in the ring
func (r *Ring) PeerNames() map[router.PeerName]struct{} {
	res := make(map[router.PeerName]struct{})

	for _, entry := range r.Entries {
		res[entry.Peer] = struct{}{}
	}

	return res
}
