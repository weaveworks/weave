package nameserver

import (
	"errors"
	"github.com/miekg/dns"
	"math"
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

const (
	stPending  uint8 = iota // someone is waiting for the resolution
	stResolved uint8 = iota // resolved
	stInvalid  uint8 = iota // invalid
)

const (
	CacheLocalReply uint8 = 1 << iota // the reply was obtained from a local resolution
)

// a cache entry
type entry struct {
	Status uint8 // status of the entry
	Flags  uint8 // some extra flags

	question dns.Question
	reply    dns.Msg

	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	putTime    time.Time

	waitChan chan struct{}
}

func newEntry(question *dns.Question, reply *dns.Msg, status uint8, now time.Time) *entry {
	e := &entry{
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
// (in the future, some other transformation could be done, like a round-robin of the responses...)
func (e *entry) getReply(request *dns.Msg, now time.Time) (*dns.Msg, error) {
	// if the reply has expired or is invalid, force the caller to start a new resolution
	if e.hasExpired(now) {
		return nil, nil
	}

	// create a copy of the reply, with values for this particular query
	reply := e.reply
	reply.SetReply(request)
	reply.Rcode = e.reply.Rcode
	reply.Authoritative = (e.Flags & CacheLocalReply != 0)		// we are only authoritative for local questions

	// adjust the TTLs
	passedSecs := now.Sub(e.putTime).Seconds()
	for _, rr := range reply.Answer {
		newTtl := rr.Header().Ttl
		newTtl -= uint32(passedSecs)
		if newTtl < 0 {
			newTtl = 0
		}
		rr.Header().Ttl = newTtl
	}

	// TODO: shuffle the values, etc...

	return &reply, nil
}

func (e entry) hasExpired(now time.Time) bool {
	return e.validUntil.Before(now)
}

// set the reply for
func (e *entry) setReply(reply *dns.Msg, flags uint8, now time.Time) {
	shouldNotify := (e.Status == stPending || e.Status == stInvalid)

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
}

// wait until a valid reply is set in the cache
func (e *entry) waitReply(request *dns.Msg, timeout time.Duration, now time.Time) (reply *dns.Msg, err error) {
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

// Invalidate the entry, so the cached value cannot be considered useful
func (e *entry) invalidate(now time.Time) {
	if e.Status != stPending {
		e.validUntil = now.Add(time.Duration(defPendingTimeout) * time.Second)
		e.waitChan = make(chan struct{})
	}
	e.Status = stInvalid
}

// entriesSlice is used for sorting entries
type entriesSlice []*entry

func (p entriesSlice) Len() int           { return len(p) }
func (p entriesSlice) Less(i, j int) bool { return p[i].validUntil.Before(p[j].validUntil) }
func (p entriesSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

//////////////////////////////////////////////////////////////////////////////////////

type entries map[dns.Question]*entry

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
func (c *Cache) Put(request *dns.Msg, reply *dns.Msg, flags uint8, now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	if ent, found := c.entries[question]; found {
		ent.setReply(reply, flags, now)
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entries) >= c.Capacity {
			c.removeOldest(1, now)
		}
		c.entries[question] = newEntry(&question, reply, stResolved, now)
	}
}

// Look up for a question's reply from the cache.
// If no reply is stored in the cache, it returns a `nil` reply and error. The caller can then `Wait()`
// for another goroutine `Put`ing a reply in the cache.
func (c *Cache) Get(request *dns.Msg, now time.Time) (reply *dns.Msg, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	ent, found := c.entries[question]
	if found {
		return ent.getReply(request, now)
	} else {
		// we are the first asking for this name: create an entry with no reply... the caller must wait
		c.entries[question] = newEntry(&question, nil, stPending, now)
		return nil, nil
	}
}

// Invalidate a cache entry, so it is not returned to clients
func (c *Cache) Invalidate(request *dns.Msg, now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()

	question := request.Question[0]
	if ent, found := c.entries[question]; found {
		ent.invalidate(now)
	}
}

// Wait for a reply for a question in the cache
// Notice that the caller could Get() and then Wait() for a question, but the corresponding cache
// entry could have been removed in between. In that case, the caller should retry the query (and
// the user should increase the cache size!)
func (c *Cache) Wait(request *dns.Msg, timeout time.Duration, now time.Time) (*dns.Msg, error) {
	// do not try to lock the cache: otherwise, no one else could `Put()` the reply
	question := request.Question[0]
	entry, found := c.entries[question]
	if !found {
		return nil, nil // client will trigger another query
	}
	return entry.waitReply(request, timeout, now)
}

// Remove removes the provided question from the cache.
func (c *Cache) Remove(question *dns.Question) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.entries, *question)
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
	for question, entry := range c.entries {
		if entry.hasExpired(now) {
			delete(c.entries, question)

			removed += 1
			if removed >= atLeast {
				return
			}
		}
	}

	// our last resort: sort the entries (by validUntil) and remove the first `atLeast` entries
	var es entriesSlice
	for _, e := range c.entries {
		es = append(es, e)
	}
	sort.Sort(es)
	for i := 0; i < atLeast; i++ {
		question := es[i].question
		delete(c.entries, question)
	}
}
