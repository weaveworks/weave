package router

import (
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
	cache := &MacCache{
		table:    make(map[uint64]*MacCacheEntry),
		maxAge:   maxAge,
		onExpiry: onExpiry}
	cache.setExpiryTimer()
	return cache
}

func (cache *MacCache) add(mac net.HardwareAddr, peer *Peer, force bool) (bool, *Peer) {
	key := macint(mac)
	now := time.Now()

	cache.RLock()
	entry, found := cache.table[key]
	if found && entry.peer == peer && now.Before(entry.lastSeen.Add(cache.maxAge/10)) {
		cache.RUnlock()
		return false, nil
	}
	cache.RUnlock()

	cache.Lock()
	defer cache.Unlock()

	entry, found = cache.table[key]
	if !found {
		cache.table[key] = &MacCacheEntry{lastSeen: now, peer: peer}
		return true, nil
	}

	if entry.peer != peer {
		if !force {
			return false, entry.peer
		}

		entry.peer = peer
	}

	if now.After(entry.lastSeen.Add(cache.maxAge / 10)) {
		entry.lastSeen = now
	}

	return false, nil
}

func (cache *MacCache) Add(mac net.HardwareAddr, peer *Peer) (bool, *Peer) {
	return cache.add(mac, peer, false)
}

func (cache *MacCache) AddForced(mac net.HardwareAddr, peer *Peer) bool {
	newMac, _ := cache.add(mac, peer, true)
	return newMac
}

func (cache *MacCache) Lookup(mac net.HardwareAddr) *Peer {
	key := macint(mac)
	cache.RLock()
	defer cache.RUnlock()
	entry, found := cache.table[key]
	if !found {
		return nil
	}
	return entry.peer
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
