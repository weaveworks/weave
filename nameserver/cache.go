package nameserver

import (
	"container/heap"
	"errors"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	errInvalidCapacity = errors.New("Invalid cache capacity")
	errCouldNotResolve = errors.New("Could not resolve")
	errTimeout         = errors.New("Timeout while waiting for resolution")
	errNoLocalReplies  = errors.New("No local replies")
)

const (
	defPendingTimeout int = 5  // timeout for a resolution
)

type entryStatus uint8

const (
	stPending  entryStatus = iota // someone is waiting for the resolution
	stResolved entryStatus = iota // resolved
)

const (
	CacheNoLocalReplies uint8 = 1 << iota // not found in local network (stored in the cache so we skip another local lookup or some time)
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
	Status   entryStatus // status of the entry
	Flags    uint8       // some extra flags
	ReplyLen int

	question dns.Question
	protocol dnsProtocol
	reply    dns.Msg

	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	putTime    time.Time

	waitChan chan struct{}

	index int // for fast lookups in the heap
}

func newCacheEntry(question *dns.Question, reply *dns.Msg, status entryStatus, flags uint8, now time.Time) *cacheEntry {
	e := &cacheEntry{
		Status:   status,
		Flags:    flags,
		question: *question,
		index:    -1,
	}

	if e.Status == stPending {
		e.validUntil = now.Add(time.Duration(defPendingTimeout) * time.Second)
		e.waitChan = make(chan struct{})
	} else {
		e.setReply(reply, flags, now)
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
		Debug.Printf("[cache] returning truncated reponse: %d > %d", e.ReplyLen, maxLen)
		return makeTruncatedReply(request), nil
	}

	// create a copy of the reply, with values for this particular query
	reply := e.reply
	reply.SetReply(request)
	reply.Rcode = e.reply.Rcode
	reply.Authoritative = true

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
	reply.Answer = shuffleAnswers(reply.Answer)

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

	e.Status = stResolved
	e.Flags = flags
	e.putTime = now

	if e.Flags&CacheNoLocalReplies != 0 {
		// use a fixed timeout for negative local resolutions
		e.validUntil = now.Add(time.Second * time.Duration(negLocalTTL))
	} else {
		// calculate the validUntil from the reply TTL
		var minTtl uint32 = math.MaxUint32
		for _, rr := range reply.Answer {
			ttl := rr.Header().Ttl
			if ttl < minTtl {
				minTtl = ttl // TODO: improve the minTTL calculation (maybe we should skip some RRs)
			}
		}
		e.validUntil = now.Add(time.Second * time.Duration(minTtl))
	}

	if reply != nil {
		e.reply = *reply
		e.ReplyLen = reply.Len()
	}

	if shouldNotify {
		close(e.waitChan) // notify all the waiters by closing the channel
	}

	return (prevValidUntil != e.validUntil)
}

// wait until a valid reply is set in the cache
func (e *cacheEntry) waitReply(request *dns.Msg, timeout time.Duration, maxLen int, now time.Time) (*dns.Msg, error) {
	if e.Status == stResolved {
		return e.getReply(request, maxLen, now)
	}

	if timeout > 0 {
		select {
		case <-e.waitChan:
			return e.getReply(request, maxLen, now)
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

type cacheKey dns.Question
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
	heap.Init(&c.entriesH)
}

// Purge removes the old elements in the cache
func (c *Cache) Purge(now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()

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
func (c *Cache) Put(request *dns.Msg, reply *dns.Msg, flags uint8, now time.Time) int {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	key := cacheKey(question)
	ent, found := c.entries[key]
	if found {
		Debug.Printf("[cache msgid %d] replacing response in cache", request.MsgHdr.Id)
		updated := ent.setReply(reply, flags, now)
		if updated {
			heap.Fix(&c.entriesH, ent.index)
		}
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entriesH) >= c.Capacity {
			lowestEntry := heap.Pop(&c.entriesH).(*cacheEntry)
			lowestEntry.close()
			delete(c.entries, cacheKey(lowestEntry.question))
		}
		ent = newCacheEntry(&question, reply, stResolved, flags, now)
		heap.Push(&c.entriesH, ent)
		c.entries[key] = ent
	}
	return ent.ReplyLen
}

// Look up for a question's reply from the cache.
// If no reply is stored in the cache, it returns a `nil` reply and no error. The caller can then `Wait()`
// for another goroutine `Put`ing a reply in the cache.
func (c *Cache) Get(request *dns.Msg, maxLen int, now time.Time) (reply *dns.Msg, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	key := cacheKey(question)
	if ent, found := c.entries[key]; found {
		reply, err = ent.getReply(request, maxLen, now)
		if ent.hasExpired(now) {
			Debug.Printf("[cache msgid %d] expired: removing", request.MsgHdr.Id)
			if ent.index > 0 {
				heap.Remove(&c.entriesH, ent.index)
			}
			delete(c.entries, key)
			reply = nil
		}
	} else {
		// we are the first asking for this name: create an entry with no reply... the caller must wait
		Debug.Printf("[cache msgid %d] addind in pending state", request.MsgHdr.Id)
		c.entries[key] = newCacheEntry(&question, nil, stPending, 0, now)
	}
	return
}

// Wait for a reply for a question in the cache
// Notice that the caller could Get() and then Wait() for a question, but the corresponding cache
// entry could have been removed in between. In that case, the caller should retry the query (and
// the user should increase the cache size!)
func (c *Cache) Wait(request *dns.Msg, timeout time.Duration, maxLen int, now time.Time) (reply *dns.Msg, err error) {
	// do not try to lock the cache: otherwise, no one else could `Put()` the reply
	question := request.Question[0]
	if entry, found := c.entries[cacheKey(question)]; found {
		reply, err = entry.waitReply(request, timeout, maxLen, now)
	}
	return
}

// Remove removes the provided question from the cache.
func (c *Cache) Remove(question *dns.Question) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := cacheKey(*question)
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
