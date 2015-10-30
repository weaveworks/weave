package nameserver

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

var now = func() int64 { return time.Now().Unix() }

type Entry struct {
	ContainerID string
	Origin      router.PeerName
	Addr        address.Address
	Hostname    string
	Version     int
	Tombstone   int64 // timestamp of when it was deleted
}

type Entries []Entry
type CaseSensitive Entries
type CaseInsensitive Entries
type SortableEntries interface {
	sort.Interface
	Get(i int) Entry
}

// Gossip messages are sorted in a case sensitive order...
func (es CaseSensitive) Len() int           { return len(es) }
func (es CaseSensitive) Swap(i, j int)      { es[i], es[j] = es[j], es[i] }
func (es CaseSensitive) Get(i int) Entry    { return es[i] }
func (es CaseSensitive) Less(i, j int) bool { return es[i].less(&es[j]) }

// ... but we store entries in a case insensitive order.
func (es CaseInsensitive) Len() int           { return len(es) }
func (es CaseInsensitive) Swap(i, j int)      { es[i], es[j] = es[j], es[i] }
func (es CaseInsensitive) Get(i int) Entry    { return es[i] }
func (es CaseInsensitive) Less(i, j int) bool { return es[i].insensitiveLess(&es[j]) }

func (e1 Entry) equal(e2 Entry) bool {
	return e1.ContainerID == e2.ContainerID &&
		e1.Origin == e2.Origin &&
		e1.Addr == e2.Addr &&
		e1.Hostname == e2.Hostname
}

func (e1 *Entry) less(e2 *Entry) bool {
	// Entries are kept sorted by Hostname, Origin, ContainerID then address
	switch {
	case e1.Hostname != e2.Hostname:
		return e1.Hostname < e2.Hostname

	case e1.Origin != e2.Origin:
		return e1.Origin < e2.Origin

	case e1.ContainerID != e2.ContainerID:
		return e1.ContainerID < e2.ContainerID

	default:
		return e1.Addr < e2.Addr
	}
}

func (e1 *Entry) insensitiveLess(e2 *Entry) bool {
	// Entries are kept sorted by Hostname, Origin, ContainerID then address
	e1Hostname, e2Hostname := strings.ToLower(e1.Hostname), strings.ToLower(e2.Hostname)
	switch {
	case e1Hostname != e2Hostname:
		return e1Hostname < e2Hostname

	case e1.Origin != e2.Origin:
		return e1.Origin < e2.Origin

	case e1.ContainerID != e2.ContainerID:
		return e1.ContainerID < e2.ContainerID

	default:
		return e1.Addr < e2.Addr
	}
}

// returns true to indicate a change
func (e1 *Entry) merge(e2 *Entry) bool {
	// we know container id, origin, add and hostname are equal
	if e2.Version > e1.Version {
		e1.Version = e2.Version
		e1.Tombstone = e2.Tombstone
		return true
	}
	return false
}

func (e1 *Entry) String() string {
	return fmt.Sprintf("%s -> %s", e1.Hostname, e1.Addr.String())
}

func check(es SortableEntries) error {
	if !sort.IsSorted(es) {
		return fmt.Errorf("Not sorted!")
	}
	for i := 1; i < es.Len(); i++ {
		if es.Get(i).equal(es.Get(i - 1)) {
			return fmt.Errorf("Duplicate entry: %d:%v and %d:%v", i-1, es.Get(i-1), i, es.Get(i))
		}
	}
	return nil
}

func checkAndPanic(es SortableEntries) {
	if err := check(es); err != nil {
		panic(err)
	}
}

func (es *Entries) checkAndPanic() *Entries {
	checkAndPanic(CaseInsensitive(*es))
	return es
}

func (es *Entries) add(hostname, containerid string, origin router.PeerName, addr address.Address) Entry {
	defer es.checkAndPanic().checkAndPanic()

	entry := Entry{Hostname: hostname, Origin: origin, ContainerID: containerid, Addr: addr}
	i := sort.Search(len(*es), func(i int) bool {
		return !(*es)[i].insensitiveLess(&entry)
	})
	if i < len(*es) && (*es)[i].equal(entry) {
		if (*es)[i].Tombstone > 0 {
			(*es)[i].Tombstone = 0
			(*es)[i].Version++
		}
	} else {
		*es = append(*es, Entry{})
		copy((*es)[i+1:], (*es)[i:])
		(*es)[i] = entry
	}
	return (*es)[i]
}

func (es *Entries) merge(incoming Entries) Entries {
	defer es.checkAndPanic().checkAndPanic()
	incoming.checkAndPanic()

	newEntries := Entries{}
	i := 0

	for _, entry := range incoming {
		for i < len(*es) && (*es)[i].insensitiveLess(&entry) {
			i++
		}
		if i < len(*es) && (*es)[i].equal(entry) {
			if (*es)[i].merge(&entry) {
				newEntries = append(newEntries, entry)
			}
		} else {
			*es = append(*es, Entry{})
			copy((*es)[i+1:], (*es)[i:])
			(*es)[i] = entry
			newEntries = append(newEntries, entry)
		}
	}

	return newEntries
}

// f returning true means keep the entry.
func (es *Entries) tombstone(ourname router.PeerName, f func(*Entry) bool) Entries {
	defer es.checkAndPanic().checkAndPanic()

	tombstoned := Entries{}
	for i, e := range *es {
		if f(&e) && e.Origin == ourname {
			e.Version++
			e.Tombstone = now()
			(*es)[i] = e
			tombstoned = append(tombstoned, e)
		}
	}
	return tombstoned
}

func (es *Entries) filter(f func(*Entry) bool) {
	defer es.checkAndPanic().checkAndPanic()

	i := 0
	for _, e := range *es {
		if !f(&e) {
			continue
		}
		(*es)[i] = e
		i++
	}
	*es = (*es)[:i]
}

func (es Entries) lookup(hostname string) Entries {
	es.checkAndPanic()

	lowerHostname := strings.ToLower(hostname)
	i := sort.Search(len(es), func(i int) bool {
		return strings.ToLower(es[i].Hostname) >= lowerHostname
	})
	if i >= len(es) || strings.ToLower(es[i].Hostname) != lowerHostname {
		return Entries{}
	}

	j := sort.Search(len(es)-i, func(j int) bool {
		return strings.ToLower(es[i+j].Hostname) > lowerHostname
	})

	return es[i : i+j]
}

func (es *Entries) first(f func(*Entry) bool) (*Entry, error) {
	es.checkAndPanic()

	for _, e := range *es {
		if f(&e) {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("Not found")
}

type GossipData struct {
	Timestamp int64
	Entries
}

func (g *GossipData) Merge(o router.GossipData) {
	other := o.(*GossipData)
	g.Entries.merge(other.Entries)
	if g.Timestamp < other.Timestamp {
		g.Timestamp = other.Timestamp
	}
}

func (g *GossipData) Encode() [][]byte {
	// Make a copy so we can sort: all outgoing data is sent in case-sensitive order
	g2 := GossipData{Timestamp: g.Timestamp, Entries: make(Entries, len(g.Entries))}
	copy(g2.Entries, g.Entries)
	sort.Sort(CaseSensitive(g2.Entries))
	buf := &bytes.Buffer{}
	if err := gob.NewEncoder(buf).Encode(g2); err != nil {
		panic(err)
	}
	return [][]byte{buf.Bytes()}
}
