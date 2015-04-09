package ring

import (
	"sort"

	"github.com/weaveworks/weave/ipam/utils"
	"github.com/weaveworks/weave/router"
)

// Entry represents entries around the ring
type entry struct {
	Token     uint32          // The start of this range
	Peer      router.PeerName // Who owns this range
	Tombstone int64           // Timestamp when this entry was tombstone; 0 means live
	Version   uint32          // Version of this range
	Free      uint32          // Number of free IPs in this range
}

func (e *entry) Equal(e2 *entry) bool {
	return e.Token == e2.Token && e.Peer == e2.Peer &&
		e.Tombstone == e2.Tombstone && e.Version == e2.Version
}

func (e *entry) update(peername router.PeerName, free uint32) {
	e.Peer = peername
	e.Tombstone = 0
	e.Version++
	e.Free = free
}

// For compatibility with sort.Interface
type entries []*entry

func (es entries) Len() int           { return len(es) }
func (es entries) Less(i, j int) bool { return es[i].Token < es[j].Token }
func (es entries) Swap(i, j int)      { panic("Should never be swapping entries!") }

func (es entries) entry(i int) *entry {
	i = i % len(es)
	if i < 0 {
		i += len(es)
	}
	return es[i]
}

func (es *entries) insert(e entry) {
	i := sort.Search(len(*es), func(j int) bool {
		return (*es)[j].Token >= e.Token
	})

	if i < len(*es) && (*es)[i].Token == e.Token {
		panic("Trying to insert an existing token!")
	}

	*es = append(*es, &entry{})
	copy((*es)[i+1:], (*es)[i:])
	(*es)[i] = &e
}

func (es entries) get(token uint32) (*entry, bool) {
	i := sort.Search(len(es), func(j int) bool {
		return es[j].Token >= token
	})

	if i < len(es) && es[i].Token == token {
		return es[i], true
	}

	return nil, false
}

func (es *entries) remove(i int) {
	*es = (*es)[:i+copy((*es)[i:], (*es)[i+1:])]
}

// Is token between entries at i and j?
// NB i and j can overflow and will wrap
// NBB this function does not work very well if there is only one
//     token on the ring; luckily an accurate answer is not needed
//     by the call sites in this case.
func (es entries) between(token uint32, i, j int) bool {
	utils.Assert(i < j, "Start and end must be in order")

	first := es.entry(i)
	second := es.entry(j)

	switch {
	case first.Token == second.Token:
		// This implies there is only one token
		// on the ring (i < j and i.token == j.token)
		// In which case everything is between, expect
		// this one token
		return token != first.Token

	case first.Token < second.Token:
		return first.Token <= token && token < second.Token

	case second.Token < first.Token:
		return first.Token <= token || token < second.Token
	}

	panic("Should never get here - switch covers all possibilities.")
}

// filteredEntries returns the entires minus tombstones
func (es entries) filteredEntries() entries {
	var result = make([]*entry, 0, len(es))
	for _, entry := range es {
		if entry.Tombstone == 0 {
			result = append(result, entry)
		}
	}
	return result
}
