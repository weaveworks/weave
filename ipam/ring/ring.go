/*
Package ring implements a simple ring CRDT.
*/
package ring

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"time"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/utils"
	"github.com/weaveworks/weave/router"
)

// Ring represents the ring itself
type Ring struct {
	Start, End uint32          // [min, max) tokens in this ring.  Due to wrapping, min == max (effectively)
	Peername   router.PeerName // name of peer owning this ring instance
	Entries    entries         // list of entries sorted by token
}

func (r *Ring) assertInvariants() {
	err := r.checkInvariants()
	if err != nil {
		panic(err.Error())
	}
}

// Errors returned by merge
var (
	ErrNotSorted        = errors.New("Ring not sorted")
	ErrTokenRepeated    = errors.New("Token appears twice in ring")
	ErrTokenOutOfRange  = errors.New("Token is out of range")
	ErrDifferentSubnets = errors.New("Cannot merge gossip for different subnet!")
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
	// NB for this check, we ignore tombstones
	entries := r.Entries.filteredEntries()
	for i, entry := range entries {
		next := entries.entry(i + 1)
		distance := r.distance(entry.Token, next.Token)

		if entry.Free > distance {
			return ErrTooMuchFreeSpace
		}
	}

	return nil
}

// New creates an empty ring belonging to peer.
func New(startIP, endIP net.IP, peer router.PeerName) *Ring {
	start, end := utils.IP4int(startIP), utils.IP4int(endIP)
	utils.Assert(start < end, "Start needs to be less than end!")

	ring := &Ring{start, end, peer, make([]*entry, 0)}
	ring.updateExportedVariables()
	return ring
}

// Returns the distance between two tokens on this ring, dealing
// with ranges which cross the origin
func (r *Ring) distance(start, end uint32) uint32 {
	if end > start {
		return end - start
	}

	return (r.End - start) + (end - r.Start)
}

// GrantRangeToHost modifies the ring such that range [start, end)
// is assigned to peer.  This may insert up to two new tokens.
// Note, due to wrapping, end can be less than start
// Preconditions:
// - startIP < endIP
// - [start, end) must be owned by the calling peer
// - there must not be any live tokens in the range
func (r *Ring) GrantRangeToHost(startIP, endIP net.IP, peer router.PeerName) {
	//fmt.Printf("%s GrantRangeToHost [%v,%v) -> %s\n", r.Peername, startIP, endIP, peer)

	var (
		start, end = utils.IP4int(startIP), utils.IP4int(endIP)
		length     = r.distance(start, end)
	)

	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	// ----------------- Start of Checks -----------------

	utils.Assert(r.Start <= start && start < r.End, "Trying to grant range outside of subnet")
	utils.Assert(r.Start < end && end <= r.End, "Trying to grant range outside of subnet")
	utils.Assert(len(r.Entries) > 0, "Cannot grant if ring is empty!")
	utils.Assert(length > 0, "Cannot create zero-sized ranges")

	// A note re:TOMBSTONES - this is tricky, as we need to do our checks on a
	// view of the ring minus tombstones (new ranges can span tombstoned entries),
	// but we need to modify the real ring.

	// Look for the left-most entry greater than start, then go one previous
	// to get the right-most entry less than or equal to start
	filteredEntries := r.Entries.filteredEntries()
	preceedingEntry := sort.Search(len(filteredEntries), func(j int) bool {
		return filteredEntries[j].Token > start
	})
	preceedingEntry--

	utils.Assert(len(filteredEntries) > 0, "Cannot grant into an empty ring, as by definition you don't own anything.")
	previousLiveEntry := filteredEntries.entry(preceedingEntry)
	utils.Assert(previousLiveEntry.Peer == r.Peername, "Trying to grant in a range I don't own")

	// At the end, the check is a little trickier.  There is never an entry with
	// a token of r.End, as the end of the ring is exclusive.  If we've asked to end == r.End,
	// we actually want an entry with a token of r.Start
	if end == r.End {
		end = r.Start
	}

	// Either the next non-tombstone token is the end token, or the end token is between
	// the current and the next.
	nextLiveEntry := filteredEntries.entry(preceedingEntry + 1)
	utils.Assert(filteredEntries.between(end, preceedingEntry, preceedingEntry+1) ||
		nextLiveEntry.Token == end,
		"Trying to grant spanning a token")

	// ----------------- End of Checks -----------------

	// Is there already a token at start? in which case we need
	// to change the owner and update version
	// Note we don't need to check ownership; we did that above.
	if startEntry, found := r.Entries.get(start); found {
		startEntry.update(peer, length)
	} else {
		// Otherwise, these isn't a token here, insert a new one.
		// Checks have already ensured we own this range.
		r.Entries.insert(entry{Token: start, Peer: peer, Free: length})
	}

	// Reset free space on previous (non-tombstone) entry, which we own.
	if previousLiveEntry.Token != start {
		previousLiveEntry.Free = r.distance(previousLiveEntry.Token, start)
		previousLiveEntry.Version++
	}

	r.assertInvariants()

	// look for the the entry with the end token
	endEntry, found := r.Entries.get(end)

	// Compute free space: nextLiveEntry might not be the next token
	// after, it might be the same as end, but that will just under-estimate free
	// space, which will get corrected by calls to ReportFree.
	endFree := r.distance(end, nextLiveEntry.Token)

	// Now we need to deal with the end token.  There are 4 possible cases.
	switch {
	//  Case i(a).  there is a token equal to the end of the range, and it's not a tombstone
	//        => we don't need to do anything
	case found && endEntry.Tombstone == 0:
		return

	//  Case i(b).  there is a token equal to the end of the range, but it is a tombstone
	//        => resurrect it for this host, increment version number.
	case found && endEntry.Tombstone > 0:
		endEntry.update(r.Peername, endFree)
		return

	//   ii.  the end is between this token and the next,
	//        => we need to insert a token such that we claim this bit on the end.
	//   iii. the end is not between this token and next, but the intervening tokens
	//        are all tombstones.
	//        => this is fine, insert away - no need to check, we did that already
	default:
		utils.Assert(!found, "WTF")
		r.Entries.insert(entry{Token: end, Peer: r.Peername, Free: endFree})
	}
}

// Merge the given ring into this ring.
func (r *Ring) merge(gossip Ring) error {
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

	// If thy receive a ring in which thy has been tombstoned, kill thyself
	for _, entry := range gossip.Entries {
		if entry.Peer == r.Peername && entry.Tombstone > 0 {
			panic("Ah! I've been shot")
		}
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
			if mine.Tombstone == 0 {
				previousOwner = &mine.Peer
			}
			i++
		case mine.Token > theirs.Token:
			// insert, checking that a range owned by us hasn't been split
			if previousOwner != nil && *previousOwner == r.Peername && theirs.Peer != r.Peername {
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
					common.Debug.Printf("Error merging entries at %s - %v != %v\n", utils.IntIP4(mine.Token), mine, theirs)
					return ErrInvalidEntry
				}
				addToResult(*mine)
				if mine.Tombstone == 0 {
					previousOwner = &mine.Peer
				}
			case mine.Version < theirs.Version:
				if mine.Peer == r.Peername { // We shouldn't receive updates to our own tokens
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
		if previousOwner != nil && *previousOwner == r.Peername && theirs.Peer != r.Peername {
			return ErrEntryInMyRange
		}
		addToResult(*theirs)
		previousOwner = nil
	}

	r.Entries = result
	return nil
}

// UpdateRing updates the ring with the state in msg
func (r *Ring) UpdateRing(msg []byte) error {
	reader := bytes.NewReader(msg)
	decoder := gob.NewDecoder(reader)
	gossipedRing := Ring{}

	if err := decoder.Decode(&gossipedRing); err != nil {
		return err
	}

	if err := r.merge(gossipedRing); err != nil {
		return err
	}
	return nil
}

// GossipState returns the encoded state of the ring
func (r *Ring) GossipState() []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(r); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Empty returns true if the ring has no live entries (may contain tombstones)
func (r *Ring) Empty() bool {
	return len(r.Entries.filteredEntries()) == 0
}

// Range is the return type for OwnedRanges.
// NB will never have End < Start
type Range struct {
	Start, End net.IP // [Start, End) of range I own
}

// OwnedRanges returns slice of Ranges, ordered by IP, indicating which
// ranges are owned by this peer.  Will split ranges which
// span 0 in the ring.
func (r *Ring) OwnedRanges() []Range {
	var (
		result  []Range
		entries = r.Entries.filteredEntries() // We can ignore tombstones in this method
	)
	r.assertInvariants()

	for i, entry := range entries {
		if entry.Peer != r.Peername {
			continue
		}

		// next logical token in ring; be careful of
		// wrapping the index
		nextEntry := entries.entry(i + 1)

		switch {
		case nextEntry.Token == r.Start:
			// be careful here; if end token == start (ie last)
			// entry on ring, we want to actually use r.End
			result = append(result, Range{Start: utils.IntIP4(entry.Token),
				End: utils.IntIP4(r.End)})

		case nextEntry.Token <= entry.Token:
			// We wrapped; want to split around 0
			// First shuffle everything up as we want results to be sorted
			result = append(result, Range{})
			copy(result[1:], result[:len(result)-1])
			result[0] = Range{Start: utils.IntIP4(r.Start),
				End: utils.IntIP4(nextEntry.Token)}
			result = append(result, Range{Start: utils.IntIP4(entry.Token),
				End: utils.IntIP4(r.End)})

		default:
			result = append(result, Range{Start: utils.IntIP4(entry.Token),
				End: utils.IntIP4(nextEntry.Token)})
		}
	}

	return result
}

// ClaimItAll claims the entire ring for this peer.  Only works for empty rings.
func (r *Ring) ClaimItAll() {
	utils.Assert(r.Empty(), "Cannot bootstrap ring with entries in it!")
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	if e, found := r.Entries.get(r.Start); found {
		e.update(r.Peername, r.distance(r.Start, r.End))
	} else {
		r.Entries.insert(entry{Token: r.Start, Peer: r.Peername,
			Free: r.distance(r.Start, r.End)})
	}
}

func (r *Ring) String() string {
	var buffer bytes.Buffer
	for _, entry := range r.Entries {
		fmt.Fprintf(&buffer, "%s -> %s (%d, %d, %d)\n", utils.IntIP4(entry.Token),
			entry.Peer, entry.Tombstone, entry.Version, entry.Free)
	}
	return buffer.String()
}

// ReportFree is used by the allocator to tell the ring
// how many free ips are in a given range, so that ChoosePeerToAskForSpace
// can make more intelligent decisions.
func (r *Ring) ReportFree(startIP net.IP, free uint32) {
	start := utils.IP4int(startIP)
	entries := r.Entries.filteredEntries() // We don't want to report free on tombstones

	// Look for entry
	i := sort.Search(len(entries), func(j int) bool {
		return entries[j].Token >= start
	})

	utils.Assert(i < len(entries) && entries[i].Token == start &&
		entries[i].Peer == r.Peername, "Trying to report free on space I don't own")

	// Check we're not reporting more space than the range
	entry, next := entries.entry(i), entries.entry(i+1)
	maxSize := r.distance(entry.Token, next.Token)
	utils.Assert(free <= maxSize, "Trying to report more free space than possible")

	if entries[i].Free == free {
		return
	}

	entries[i].Free = free
	entries[i].Version++
}

// ChoosePeerToAskForSpace chooses a weighted-random peer to ask
// for space.
func (r *Ring) ChoosePeerToAskForSpace() (result router.PeerName, err error) {
	var (
		sum               uint32
		totalSpacePerPeer = make(map[router.PeerName]uint32) // Compute total free space per peer
	)

	// iterate through tokens IGNORING tombstones
	for _, entry := range r.Entries.filteredEntries() {
		utils.Assert(entry.Tombstone == 0, "List shouldn't contain tombstoned entries")

		// Ignore ranges with no free space
		if entry.Free <= 0 {
			continue
		}

		// Don't talk to yourself
		if entry.Peer == r.Peername {
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

// TombstonePeer will mark all entries associated with this peer as tombstones
func (r *Ring) TombstonePeer(peer router.PeerName, dt time.Duration) error {
	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	if dt <= 0 {
		return ErrInvalidTimeout
	}

	found := false
	absTimeout := time.Now().Unix() + int64(dt)

	for _, entry := range r.Entries {
		if entry.Peer == peer {
			found = true
			entry.Tombstone = absTimeout
			entry.Version++
		}
	}

	if !found {
		return ErrNotFound
	}

	return nil
}

// ExpireTombstones removes tombstone entries with timeouts greater than now
func (r *Ring) ExpireTombstones(now int64) {
	r.assertInvariants()
	defer r.assertInvariants()
	defer r.updateExportedVariables()

	i := 0
	for i < len(r.Entries) {
		entry := r.Entries.entry(i)
		if entry.Tombstone == 0 || entry.Tombstone > now {
			i++
			continue
		}

		r.Entries.remove(i)
	}
}

// Contains returns true if addr is in this ring
func (r *Ring) Contains(addr net.IP) bool {
	pos := utils.IP4int(addr)
	return pos >= r.Start && pos < r.End
}

// Owner returns the peername which owns the range containing addr
func (r *Ring) Owner(addr net.IP) router.PeerName {
	token := utils.IP4int(addr)
	utils.Assert(r.Start <= token && token < r.End, "Token out of range")

	r.assertInvariants()
	// There can be no owners on an empty ring
	filteredEntries := r.Entries.filteredEntries()
	if len(filteredEntries) == 0 {
		return router.UnknownPeerName
	}

	// Look for the right-most entry, less than or equal to token
	preceedingEntry := sort.Search(len(filteredEntries), func(j int) bool {
		return filteredEntries[j].Token > token
	})
	preceedingEntry--
	entry := filteredEntries.entry(preceedingEntry)
	return entry.Peer
}
