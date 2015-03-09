package nameserver

import (
	"container/heap"
	"errors"
	"github.com/miekg/dns"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	errInvalidCapacity = errors.New("Invalid cache capacity")
	errCouldNotResolve = errors.New("Could not resolve")
	errTimeout         = errors.New("Timeout while waiting for resolution")
)

const (
	defPendingTimeout int = 5 // timeout for a resolution
)

type entryStatus uint8

const (
	stPending  entryStatus = iota // someone is waiting for the resolution
	stResolved entryStatus = iota // resolved
)

const (
	CacheLocalReply uint8 = 1 << iota // the reply was obtained from a local resolution
)

// shuffleAnswers reorders answers for very basic load balancing
func shuffleAnswers(answers []dns.RR) []dns.RR {
	n := len(answers)
	if n > 1 {
		rand.Seed(time.Now().UTC().UnixNano())

		for i := 0; i < n; i++ {
			r := i + rand.Intn(n-i)
			answers[r], answers[i] = answers[i], answers[r]
		}
	}

	return answers
}

// a cache entry
type cacheEntry struct {
	Status entryStatus // status of the entry
	Flags  uint8       // some extra flags

	question dns.Question
	protocol dnsProtocol
	reply    dns.Msg

	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	putTime    time.Time

	waitChan chan struct{}

	index int	// for fast lookups in the heap
}

func newCacheEntry(question *dns.Question, reply *dns.Msg, status entryStatus, now time.Time) *cacheEntry {
	e := &cacheEntry{
		Status:   status,
		Flags:    0,
		question: *question,
		index:    -1,
	}

	if e.Status == stPending {
		e.validUntil = now.Add(time.Duration(defPendingTimeout) * time.Second)
		e.waitChan = make(chan struct{})
	} else {
		e.setReply(reply, 0, now)
	}

	return e
}

// Get a copy of the reply stored in the entry, but with some values adjusted like the TTL
func (e *cacheEntry) getReply(request *dns.Msg, now time.Time) (*dns.Msg, error) {
	if e.Status != stResolved {
		return nil, nil
	}

	// if the reply has expired or is invalid, force the caller to start a new resolution
	if e.hasExpired(now) {
		return nil, nil
	}

	// create a copy of the reply, with values for this particular query
	reply := e.reply
	reply.SetReply(request)
	reply.Rcode = e.reply.Rcode
	reply.Authoritative = (e.Flags&CacheLocalReply != 0) // we are only authoritative for local questions

	// adjust the TTLs
	passedSecs := uint32(now.Sub(e.putTime).Seconds())
	for _, rr := range reply.Answer {
		ttl := rr.Header().Ttl
		if passedSecs < ttl {
			rr.Header().Ttl = ttl - passedSecs
		} else {
			rr.Header().Ttl = 0
		}
	}

	// shuffle the values, etc...
	if e.Flags&CacheLocalReply != 0 {
		reply.Answer = shuffleAnswers(reply.Answer)
	}

	return &reply, nil
}

func (e cacheEntry) hasExpired(now time.Time) bool {
	return e.validUntil.Before(now) || e.validUntil == now
}

// set the reply for the entry
// returns True if the entry has changed the validUntil time
func (e *cacheEntry) setReply(reply *dns.Msg, flags uint8, now time.Time) bool {
	shouldNotify := (e.Status == stPending)

	var prevValidUntil time.Time
	if e.Status == stResolved {
		prevValidUntil = e.validUntil
	}

	// calculate the validUntil from the reply TTL
	var minTtl uint32 = math.MaxUint32
	for _, rr := range reply.Answer {
		ttl := rr.Header().Ttl
		if ttl < minTtl {
			minTtl = ttl // TODO: improve the minTTL calculation (maybe we should skip some RRs)
		}
	}
	e.Status = stResolved
	e.Flags = flags
	e.putTime = now
	e.validUntil = now.Add(time.Second * time.Duration(minTtl))
	e.reply = *reply

	if shouldNotify {
		close(e.waitChan) // notify all the waiters by closing the channel
	}

	return (prevValidUntil != e.validUntil)
}

// wait until a valid reply is set in the cache
func (e *cacheEntry) waitReply(request *dns.Msg, timeout time.Duration, now time.Time) (*dns.Msg, error) {
	if e.Status == stResolved {
		return e.getReply(request, now)
	}

	if timeout > 0 {
		select {
		case <-e.waitChan:
			return e.getReply(request, now)
		case <-time.After(time.Second * timeout):
			return nil, errTimeout
		}
	}

	return nil, errCouldNotResolve
}

func (e *cacheEntry) close() {
	if e.Status == stPending {
		close(e.waitChan)
	}
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

type cacheKey struct {
	question dns.Question
	protocol dnsProtocol
}

type entries map[cacheKey]*cacheEntry

// Cache is a thread-safe fixed capacity LRU cache.
type Cache struct {
	Capacity int

	entries  entries
	entriesH entriesPtrsHeap // len(entriesH) <= len(entries), as pending entries can be in entries but not in entriesH
	lock     sync.RWMutex
}

// NewCache creates a cache of the given capacity
func NewCache(capacity int) (*Cache, error) {
	if capacity <= 0 {
		return nil, errInvalidCapacity
	}
	c := &Cache{
		Capacity: capacity,
		entries:  make(entries, capacity),
	}

	heap.Init(&c.entriesH)
	return c, nil
}

// Clear removes all the entries in the cache
func (c *Cache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.entries = make(entries, c.Capacity)
}

// Purge removes the old elements in the cache
func (c *Cache) Purge(now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for i, entry := range c.entriesH {
		if entry.hasExpired(now) {
			heap.Remove(&c.entriesH, i)
			delete(c.entries, cacheKey{entry.question, entry.protocol})
		} else {
			return // all remaining entries must be still valid...
		}
	}
}

// Add adds a reply to the cache.
func (c *Cache) Put(request *dns.Msg, reply *dns.Msg, proto dnsProtocol, flags uint8, now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	key := cacheKey{question, proto}
	if ent, found := c.entries[key]; found {
		updated := ent.setReply(reply, flags, now)
		if updated {
			heap.Fix(&c.entriesH, ent.index)
		}
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entriesH) >= c.Capacity {
			lowestEntry := heap.Pop(&c.entriesH).(*cacheEntry)
			lowestEntry.close()
			delete(c.entries, cacheKey{lowestEntry.question, lowestEntry.protocol})
		}

		newEntry := newCacheEntry(&question, reply, stResolved, now)
		heap.Push(&c.entriesH, newEntry)
		c.entries[key] = newEntry
	}
}

// Look up for a question's reply from the cache.
// If no reply is stored in the cache, it returns a `nil` reply and no error. The caller can then `Wait()`
// for another goroutine `Put`ing a reply in the cache.
func (c *Cache) Get(request *dns.Msg, proto dnsProtocol, now time.Time) (reply *dns.Msg, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	key := cacheKey{question, proto}
	if ent, found := c.entries[key]; found {
		reply, err = ent.getReply(request, now)
		if ent.hasExpired(now) {
			delete(c.entries, key)
			reply = nil
		}
	} else {
		// we are the first asking for this name: create an entry with no reply... the caller must wait
		c.entries[key] = newCacheEntry(&question, nil, stPending, now)
	}
	return
}

// Wait for a reply for a question in the cache
// Notice that the caller could Get() and then Wait() for a question, but the corresponding cache
// entry could have been removed in between. In that case, the caller should retry the query (and
// the user should increase the cache size!)
func (c *Cache) Wait(request *dns.Msg, proto dnsProtocol, timeout time.Duration, now time.Time) (reply *dns.Msg, err error) {
	// do not try to lock the cache: otherwise, no one else could `Put()` the reply
	question := request.Question[0]
	key := cacheKey{question, proto}
	if entry, found := c.entries[key]; found {
		reply, err = entry.waitReply(request, timeout, now)
	}
	return
}

// Remove removes the provided question from the cache.
func (c *Cache) Remove(question *dns.Question, proto dnsProtocol) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := cacheKey{*question, proto}
	if entry, found := c.entries[key]; found {
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
