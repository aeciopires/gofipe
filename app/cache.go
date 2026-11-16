package main

import (
	"context"
	"maps"
	"sync"
	"time"
)

// cacheItem stores a cached payload and its expiration time.
type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

// ttlCache is a small, concurrency-safe in-memory cache with per-entry TTL.
//
// Keys follow the "<kind>:<part>:<part>" convention, e.g. "brands:cars" or
// "years:cars:59:5940".
type ttlCache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
	now   func() time.Time // injectable clock for tests
}

func newTTLCache() *ttlCache {
	return &ttlCache{items: make(map[string]cacheItem), now: time.Now}
}

// get returns the cached data for key, if present and not expired.
func (c *ttlCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	it, ok := c.items[key]
	if !ok || c.now().After(it.expiresAt) {
		return nil, false
	}
	return it.data, true
}

// set stores data under key for the given ttl.
func (c *ttlCache) set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheItem{data: data, expiresAt: c.now().Add(ttl)}
}

// purgeExpired removes every expired entry so memory does not grow forever.
func (c *ttlCache) purgeExpired() {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.items, func(_ string, it cacheItem) bool {
		return now.After(it.expiresAt)
	})
}

// runJanitor periodically purges expired entries until ctx is canceled.
func (c *ttlCache) runJanitor(ctx context.Context, interval time.Duration) {
	tick := time.Tick(interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			c.purgeExpired()
		}
	}
}
