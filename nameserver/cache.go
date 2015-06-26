package nameserver

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

var (
	errInvalidCapacity = errors.New("Invalid cache capacity")
	errCouldNotResolve = errors.New("Could not resolve")
	errTimeout         = errors.New("Timeout while waiting for resolution")
	errNoLocalReplies  = errors.New("No local replies")
)

const (
	defPendingTimeout int = 5 // timeout for a resolution
)

const nullTTL = 0 // a null TTL

type entryStatus uint8

const (
	stPending  entryStatus = iota // someone is waiting for the resolution
	stResolved entryStatus = iota // resolved
)

var statusToString = map[entryStatus]string{
	stPending:  "pending",
	stResolved: "resolved",
}

const (
	CacheNoLocalReplies uint8 = 1 << iota // not found in local network (stored in the cache so we skip another local lookup or some time)
)

// a cache entry
type cacheEntry struct {
	Status   entryStatus // status of the entry
	Flags    uint8       // some extra flags
	ReplyLen int

	question dns.Question
	protocol dnsProtocol
	reply    dns.Msg

	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	putTime    time.Time

	index int // for fast lookups in the heap
}

func newCacheEntry(question *dns.Question, now time.Time) *cacheEntry {
	e := &cacheEntry{
		Status:     stPending,
		validUntil: now.Add(time.Second * time.Duration(defPendingTimeout)),
		question:   *question,
		index:      -1,
	}

	return e
}

// Get a copy of the reply stored in the entry, but with some values adjusted like the TTL
func (e *cacheEntry) getReply(request *dns.Msg, maxLen int, now time.Time) (*dns.Msg, error) {
	if e.Status != stResolved {
		return nil, nil
	}

	// if the reply has expired or is invalid, force the caller to start a new resolution
	if e.hasExpired(now) {
		return nil, nil
	}

	if e.Flags&CacheNoLocalReplies != 0 {
		return nil, errNoLocalReplies
	}

	if e.ReplyLen >= maxLen {
		Log.Debugf("[cache msgid %d] returning truncated reponse: %d > %d", request.MsgHdr.Id, e.ReplyLen, maxLen)
		return makeTruncatedReply(request), nil
	}

	// create a copy of the reply, with values for this particular query
	reply := e.reply.Copy()
	reply.SetReply(request)

	// adjust the TTLs
	passedSecs := uint32(now.Sub(e.putTime).Seconds())
	for _, rr := range reply.Answer {
		hdr := rr.Header()
		ttl := hdr.Ttl
		if passedSecs < ttl {
			hdr.Ttl = ttl - passedSecs
		} else {
			return nil, nil // it is expired: do not spend more time and return nil...
		}
	}

	reply.Rcode = e.reply.Rcode
	reply.Authoritative = true

	return reply, nil
}

func (e cacheEntry) hasExpired(now time.Time) bool {
	return e.validUntil.Before(now) || e.validUntil == now
}

// set the reply for the entry
// returns True if the entry has changed the validUntil time
func (e *cacheEntry) setReply(reply *dns.Msg, ttl int, flags uint8, now time.Time) bool {
	var prevValidUntil time.Time
	if e.Status == stResolved {
		if reply != nil {
			Log.Debugf("[cache msgid %d] replacing response in cache", reply.MsgHdr.Id)
		}
		prevValidUntil = e.validUntil
	}

	// make sure we do not overwrite noLocalReplies entries
	if flags&CacheNoLocalReplies != 0 {
		if e.Flags&CacheNoLocalReplies != 0 {
			return false
		}
	}

	if ttl != nullTTL {
		e.validUntil = now.Add(time.Second * time.Duration(ttl))
	} else if reply != nil {
		// calculate the validUntil from the reply TTL
		var minTTL uint32 = math.MaxUint32
		for _, rr := range reply.Answer {
			ttl := rr.Header().Ttl
			if ttl < minTTL {
				minTTL = ttl // TODO: improve the minTTL calculation (maybe we should skip some RRs)
			}
		}
		e.validUntil = now.Add(time.Second * time.Duration(minTTL))
	} else {
		Log.Warningf("[cache] no valid TTL could be calculated")
	}

	e.Status = stResolved
	e.Flags = flags
	e.putTime = now

	if reply != nil {
		e.reply = *reply.Copy()
		e.ReplyLen = reply.Len()
	}

	return (prevValidUntil != e.validUntil)
}

func (e *cacheEntry) String() string {
	var buf bytes.Buffer
	q := e.question
	fmt.Fprintf(&buf, "'%s'[%s]: ", q.Name, dns.TypeToString[q.Qtype])
	if e.Flags&CacheNoLocalReplies != 0 {
		fmt.Fprintf(&buf, "neg-local")
	} else {
		fmt.Fprintf(&buf, "%s", statusToString[e.Status])
	}
	fmt.Fprintf(&buf, "(%d bytes)", e.ReplyLen)
	return buf.String()
}

//////////////////////////////////////////////////////////////////////////////////////

// An entriesPtrHeap is a min-heap of cache entries.
type entriesPtrsHeap []*cacheEntry

func (h entriesPtrsHeap) Len() int           { return len(h) }
func (h entriesPtrsHeap) Less(i, j int) bool { return h[i].validUntil.Before(h[j].validUntil) }
func (h entriesPtrsHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *entriesPtrsHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	n := len(*h)
	entry := x.(*cacheEntry)
	entry.index = n
	*h = append(*h, entry)
}

func (h *entriesPtrsHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*h = old[0 : n-1]
	return item
}

//////////////////////////////////////////////////////////////////////////////////////

// The cache interface
type ZoneCache interface {
	// Get from the cache
	Get(request *dns.Msg, maxLen int) (reply *dns.Msg, err error)
	// Put in the cache
	Put(request *dns.Msg, reply *dns.Msg, ttl int, flags uint8) int
	// Remove from the cache
	Remove(question *dns.Question)
	// Purge all expired entries from the cache
	Purge()
	// Remove all entries
	Clear()
	// Return the cache length
	Len() int
	// Return the max capacity
	Capacity() int
}

type cacheKey dns.Question
type entries map[cacheKey]*cacheEntry

// Cache is a thread-safe fixed capacity LRU cache.
type Cache struct {
	capacity int
	entries  entries
	entriesH entriesPtrsHeap // len(entriesH) <= len(entries), as pending entries can be in entries but not in entriesH
	clock    clock.Clock
	lock     sync.RWMutex
}

// NewCache creates a cache of the given capacity
func NewCache(capacity int, clk clock.Clock) (*Cache, error) {
	if capacity <= 0 {
		return nil, errInvalidCapacity
	}
	c := &Cache{
		capacity: capacity,
		entries:  make(entries, capacity),
		clock:    clk,
	}

	if c.clock == nil {
		c.clock = clock.New()
	}

	heap.Init(&c.entriesH)
	return c, nil
}

// Clear removes all the entries in the cache
func (c *Cache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.entries = make(entries, c.capacity)
	heap.Init(&c.entriesH)
}

// Purge removes the old elements in the cache
func (c *Cache) Purge() {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := c.clock.Now()
	for i, entry := range c.entriesH {
		if entry.hasExpired(now) {
			heap.Remove(&c.entriesH, i)
			delete(c.entries, cacheKey(entry.question))
		} else {
			return // all remaining entries must be still valid...
		}
	}
}

// Add adds a reply to the cache.
// When `ttl` is equal to `nullTTL`, the cache entry will be valid until the closest TTL in the `reply`
func (c *Cache) Put(request *dns.Msg, reply *dns.Msg, ttl int, flags uint8) int {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := c.clock.Now()
	question := request.Question[0]
	key := cacheKey(question)
	ent, found := c.entries[key]
	if found {
		updated := ent.setReply(reply, ttl, flags, now)
		if updated {
			heap.Fix(&c.entriesH, ent.index)
		}
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entriesH) >= c.capacity {
			lowestEntry := heap.Pop(&c.entriesH).(*cacheEntry)
			delete(c.entries, cacheKey(lowestEntry.question))
		}
		ent = newCacheEntry(&question, now)
		ent.setReply(reply, ttl, flags, now)
		heap.Push(&c.entriesH, ent)
		c.entries[key] = ent
	}
	return ent.ReplyLen
}

// Look up for a question's reply from the cache.
// If no reply is stored in the cache, it returns a `nil` reply and no error.
func (c *Cache) Get(request *dns.Msg, maxLen int) (reply *dns.Msg, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := c.clock.Now()
	question := request.Question[0]
	key := cacheKey(question)
	if ent, found := c.entries[key]; found {
		reply, err = ent.getReply(request, maxLen, now)
		if ent.hasExpired(now) {
			Log.Debugf("[cache msgid %d] expired: removing", request.MsgHdr.Id)
			if ent.index > 0 {
				heap.Remove(&c.entriesH, ent.index)
			}
			delete(c.entries, key)
			reply = nil
		}
	} else {
		// we are the first asking for this name: create an entry with no reply... the caller must wait
		Log.Debugf("[cache msgid %d] addind in pending state", request.MsgHdr.Id)
		c.entries[key] = newCacheEntry(&question, now)
	}
	return
}

// Remove removes the provided question from the cache.
func (c *Cache) Remove(question *dns.Question) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := cacheKey(*question)
	if entry, found := c.entries[key]; found {
		Log.Debugf("[cache] removing %s-response for '%s'", dns.TypeToString[question.Qtype], question.Name)
		if entry.index > 0 {
			heap.Remove(&c.entriesH, entry.index)
		}
		delete(c.entries, key)
	}
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.entries)
}

// Return the max capacity
func (c *Cache) Capacity() int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.capacity
}

// Return the max capacity
func (c *Cache) String() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	var buf bytes.Buffer
	for _, entry := range c.entries {
		fmt.Fprintf(&buf, "%s\n", entry)
	}
	return buf.String()
}
