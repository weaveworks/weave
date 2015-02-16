package nameserver

import (
	"errors"
	"github.com/miekg/dns"
	"sync"
	"time"
)

var (
	errInvalidCapacity = errors.New("Invalid cache capacity")
	errCouldNotResolve = errors.New("Could not resolve")
	errTimeout         = errors.New("Timeout while waiting for resolution")
)

// a cache entry
type entry struct {
	question   *dns.Question
	reply      *dns.Msg
	validUntil time.Time // obtained from the reply and stored here for convenience/speed
	waitChan   chan struct{}
}

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
func (c *Cache) Purge() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.removeOldest(c.Capacity)
}

// Add adds a reply to the cache.
func (c *Cache) Put(question *dns.Question, reply *dns.Msg) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, found := c.entries[*question]; found {
		hadWaiters := (ent.reply == nil)
		ent.reply = reply

		// notify all the waiters by closing the channel
		if hadWaiters {
			close(ent.waitChan)
		}
	} else {
		// If we will add a new item and the capacity has been exceeded, make some room...
		if len(c.entries) > c.Capacity {
			c.removeOldest(1)
		}

		c.entries[*question] = &entry{
			question: question,
			reply:    reply,
		}
	}
}

// Look up a question's reply from the cache.
func (c *Cache) Get(question *dns.Question) (reply *dns.Msg, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.entries[*question]; ok {
		return ent.reply, nil
	}
	return nil, errCouldNotResolve
}

// Look up a question's reply from the cache.
// If a valid reply is not stored in the cache, the current gorutine blocks until a Put()
// for the same question is executed, or the timeout happens.
func (c *Cache) GetBlocking(question *dns.Question, timeout time.Duration) (reply *dns.Msg, err error) {
	c.lock.Lock()

	ent, found := c.entries[*question]
	if found {
		if ent.reply != nil {
			return ent.reply, nil
		}
	} else {
		if len(c.entries) > c.Capacity {
			c.removeOldest(1) // make room for just one entry
		}

		// we are the first asking for this name: create a blocking entry
		c.entries[*question] = &entry{
			question: question,
			waitChan: make(chan struct{}),
			reply:    nil,
		}
	}

	c.lock.Unlock()

	select {
	case _, closed := <-c.entries[*question].waitChan:
		if closed {
			return c.entries[*question].reply, nil
		}
	case <-time.After(time.Second * timeout):
		return nil, errTimeout
	}

	return nil, errCouldNotResolve
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
func (c *Cache) removeOldest(atLeast int) {
	removed := 0
	// first, remove all the elements with an expired time-to-live
	now := time.Now()
	for question, entry := range c.entries {
		if entry.validUntil.Before(now) {
			delete(c.entries, question)

			removed += 1
			if removed >= atLeast {
				return
			}
		}
	}

	// TODO: be more aggressive: remove the oldest replies...
}
