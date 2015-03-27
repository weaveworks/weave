package router

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"
)

type MacCacheEntry struct {
	lastSeen time.Time
	peer     *Peer
}

type MacCache struct {
	sync.RWMutex
	table       map[uint64]*MacCacheEntry
	maxAge      time.Duration
	expiryTimer *time.Timer
	onExpiry    func(net.HardwareAddr, *Peer)
}

func NewMacCache(maxAge time.Duration, onExpiry func(net.HardwareAddr, *Peer)) *MacCache {
	return &MacCache{
		table:    make(map[uint64]*MacCacheEntry),
		maxAge:   maxAge,
		onExpiry: onExpiry}
}

func (cache *MacCache) Start() {
	cache.setExpiryTimer()
}

func (cache *MacCache) Enter(mac net.HardwareAddr, peer *Peer) bool {
	key := macint(mac)
	now := time.Now()
	cache.RLock()
	entry, found := cache.table[key]
	if found && entry.peer == peer && now.Before(entry.lastSeen.Add(cache.maxAge/10)) {
		cache.RUnlock()
		return false
	}
	cache.RUnlock()
	cache.Lock()
	defer cache.Unlock()
	entry, found = cache.table[key]
	if !found {
		cache.table[key] = &MacCacheEntry{lastSeen: now, peer: peer}
		return true
	}
	if entry.peer != peer {
		entry.lastSeen = now
		entry.peer = peer
		return true
	}
	if now.After(entry.lastSeen.Add(cache.maxAge / 10)) {
		entry.lastSeen = now
	}
	return false
}

func (cache *MacCache) Lookup(mac net.HardwareAddr) (*Peer, bool) {
	key := macint(mac)
	cache.RLock()
	defer cache.RUnlock()
	entry, found := cache.table[key]
	if !found {
		return nil, false
	}
	return entry.peer, true
}

func (cache *MacCache) Delete(peer *Peer) bool {
	found := false
	cache.Lock()
	defer cache.Unlock()
	for key, entry := range cache.table {
		if entry.peer == peer {
			delete(cache.table, key)
			found = true
		}
	}
	return found
}

func (cache *MacCache) String() string {
	var buf bytes.Buffer
	cache.RLock()
	defer cache.RUnlock()
	for key, entry := range cache.table {
		fmt.Fprintf(&buf, "%v -> %s (%v)\n", intmac(key), entry.peer.Name, entry.lastSeen)
	}
	return buf.String()
}

func (cache *MacCache) setExpiryTimer() {
	cache.expiryTimer = time.AfterFunc(cache.maxAge/10, func() { cache.expire() })
}

func (cache *MacCache) expire() {
	now := time.Now()
	cache.Lock()
	defer cache.Unlock()
	for key, entry := range cache.table {
		if now.After(entry.lastSeen.Add(cache.maxAge)) {
			delete(cache.table, key)
			cache.onExpiry(intmac(key), entry.peer)
		}
	}
	cache.setExpiryTimer()
}
