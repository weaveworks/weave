package nameserver

import (
	"errors"
	. "github.com/zettio/weave/common"
	"github.com/miekg/dns"
	"math"
	"math/rand"
	"sort"
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
	Status   entryStatus // status of the entry
	Flags    uint8       // some extra flags
	ReplyLen int

	question dns.Question
	protocol dnsProtocol
	reply    dns.Msg

	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	putTime    time.Time

	waitChan chan struct{}
}

func newCacheEntry(question *dns.Question, reply *dns.Msg, status entryStatus, now time.Time) *cacheEntry {
	e := &cacheEntry{
		Status:   status,
		Flags:    0,
		question: *question,
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
func (e *cacheEntry) getReply(request *dns.Msg, maxLen int, now time.Time) (*dns.Msg, int, error) {
	if e.Status != stResolved {
		return nil, 0, nil
	}

	// if the reply has expired or is invalid, force the caller to start a new resolution
	if e.hasExpired(now) {
		return nil, 0, nil
	}

	if e.ReplyLen >= maxLen {
		Debug.Printf("[cache] returning truncated reponse: %d > %d", e.ReplyLen, maxLen)
		return makeTruncatedReply(request), minUdpSize, nil
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

	return &reply, e.ReplyLen, nil
}

func (e cacheEntry) hasExpired(now time.Time) bool {
	return e.validUntil.Before(now) || e.validUntil == now
}

// set the reply for the entry
func (e *cacheEntry) setReply(reply *dns.Msg, flags uint8, now time.Time) {
	shouldNotify := (e.Status == stPending)

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
	e.ReplyLen = reply.Len()

	if shouldNotify {
		close(e.waitChan) // notify all the waiters by closing the channel
	}
}

// wait until a valid reply is set in the cache
func (e *cacheEntry) waitReply(request *dns.Msg, timeout time.Duration, maxLen int, now time.Time) (*dns.Msg, int, error) {
	if e.Status == stResolved {
		return e.getReply(request, maxLen, now)
	}

	if timeout > 0 {
		select {
		case <-e.waitChan:
			return e.getReply(request, maxLen, now)
		case <-time.After(time.Second * timeout):
			return nil, 0, errTimeout
		}
	}

	return nil, 0, errCouldNotResolve
}

func (e *cacheEntry) close() {
	if e.Status == stPending {
		close(e.waitChan)
	}
}

// entriesSlice is used for sorting entries
type cacheEntriesSlice []*cacheEntry

func (p cacheEntriesSlice) Len() int           { return len(p) }
func (p cacheEntriesSlice) Less(i, j int) bool { return p[i].validUntil.Before(p[j].validUntil) }
func (p cacheEntriesSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

//////////////////////////////////////////////////////////////////////////////////////

type cacheKey dns.Question
type entries map[cacheKey]*cacheEntry

// Cache is a thread-safe fixed capacity LRU cache.
type Cache struct {
	Capacity int

	entries entries
	lock    sync.RWMutex
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

	c.removeOldest(c.Capacity, now)
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
		ent.setReply(reply, flags, now)
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entries) >= c.Capacity {
			c.removeOldest(1, now)
		}
		ent = newCacheEntry(&question, reply, stResolved, now)
		c.entries[key] = ent
	}
	return ent.ReplyLen
}

// Look up for a question's reply from the cache.
// If no reply is stored in the cache, it returns a `nil` reply and no error. The caller can then `Wait()`
// for another goroutine `Put`ing a reply in the cache.
func (c *Cache) Get(request *dns.Msg, maxLen int, now time.Time) (reply *dns.Msg, replyLen int, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	key := cacheKey(question)
	if ent, found := c.entries[key]; found {
		if ent.hasExpired(now) {
			Debug.Printf("[cache msgid %d] expired: removing", request.MsgHdr.Id)
			delete(c.entries, key)
			return
		}
		reply, replyLen, err = ent.getReply(request, maxLen, now)
	} else {
		// we are the first asking for this name: create an entry with no reply... the caller must wait
		Debug.Printf("[cache msgid %d] addind in pending state", request.MsgHdr.Id)
		c.entries[key] = newCacheEntry(&question, nil, stPending, now)
	}
	return
}

// Wait for a reply for a question in the cache
// Notice that the caller could Get() and then Wait() for a question, but the corresponding cache
// entry could have been removed in between. In that case, the caller should retry the query (and
// the user should increase the cache size!)
func (c *Cache) Wait(request *dns.Msg, timeout time.Duration, maxLen int, now time.Time) (reply *dns.Msg, replyLen int, err error) {
	// do not try to lock the cache: otherwise, no one else could `Put()` the reply
	question := request.Question[0]
	if entry, found := c.entries[cacheKey(question)]; found {
		reply, replyLen, err = entry.waitReply(request, timeout, maxLen, now)
	}
	return
}

// Remove removes the provided question from the cache.
func (c *Cache) Remove(question *dns.Question) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.entries, cacheKey(*question))
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.entries)
}

// removeOldest removes the oldest item(s) from the cache.
// note: this method is not thread safe (it is a responsability of the caller function...)
func (c *Cache) removeOldest(atLeast int, now time.Time) {
	removed := 0
	// first, remove expired entries
	for key, entry := range c.entries {
		if entry.hasExpired(now) {
			entry.close()
			delete(c.entries, key)

			removed += 1
			if removed >= atLeast {
				return
			}
		}
	}

	// our last resort: sort the entries (by validUntil) and remove the first `atLeast` entries
	var es cacheEntriesSlice
	for _, e := range c.entries {
		es = append(es, e)
	}
	sort.Sort(es)
	for i := 0; i < atLeast; i++ {
		key := cacheKey(es[i].question)
		c.entries[key].close()
		delete(c.entries, key)
	}
}
