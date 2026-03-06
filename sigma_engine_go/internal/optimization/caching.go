// Package optimization provides caching and performance utilities.
package optimization

import (
	"sync"
	"time"
)

// CacheEntry represents a cached item.
type CacheEntry struct {
	Value     interface{}
	ExpiresAt time.Time
}

// Cache provides in-memory caching with TTL.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	ttl     time.Duration
	maxSize int
}

// NewCache creates a new cache with default settings.
func NewCache(ttl time.Duration, maxSize int) *Cache {
	c := &Cache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
	go c.cleanup()
	return c
}

// Get retrieves a cached value.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry.Value, true
}

// Set stores a value with default TTL.
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL stores a value with custom TTL.
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if full
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a cached value.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all cached values.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]CacheEntry)
}

// Size returns number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evictOldest removes entries closest to expiration.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ExpiresAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// cleanup periodically removes expired entries.
func (c *Cache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// Stats returns cache statistics.
func (c *Cache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"size":     len(c.entries),
		"max_size": c.maxSize,
		"ttl_secs": c.ttl.Seconds(),
	}
}

// ResponseCache caches API responses.
type ResponseCache struct {
	alerts    *Cache
	baselines *Cache
	metrics   *Cache
}

// NewResponseCache creates response cache.
func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		alerts:    NewCache(30*time.Second, 1000),
		baselines: NewCache(5*time.Minute, 100),
		metrics:   NewCache(1*time.Minute, 50),
	}
}

// GetAlerts retrieves cached alerts.
func (r *ResponseCache) GetAlerts(key string) (interface{}, bool) {
	return r.alerts.Get(key)
}

// SetAlerts caches alerts.
func (r *ResponseCache) SetAlerts(key string, value interface{}) {
	r.alerts.Set(key, value)
}

// GetBaseline retrieves cached baseline.
func (r *ResponseCache) GetBaseline(key string) (interface{}, bool) {
	return r.baselines.Get(key)
}

// SetBaseline caches baseline.
func (r *ResponseCache) SetBaseline(key string, value interface{}) {
	r.baselines.Set(key, value)
}

// InvalidateAlerts clears alert cache.
func (r *ResponseCache) InvalidateAlerts() {
	r.alerts.Clear()
}

// Stats returns all cache stats.
func (r *ResponseCache) Stats() map[string]interface{} {
	return map[string]interface{}{
		"alerts":    r.alerts.Stats(),
		"baselines": r.baselines.Stats(),
		"metrics":   r.metrics.Stats(),
	}
}
