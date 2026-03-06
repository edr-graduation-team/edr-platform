package cache

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
)

// LRUCache is a thread-safe LRU cache implementation.
type LRUCache[K comparable, V any] struct {
	cache    *lru.Cache[K, V]
	capacity int
	mu       sync.RWMutex
	stats    CacheStats
}

// NewLRUCache creates a new LRU cache with the specified capacity.
func NewLRUCache[K comparable, V any](capacity int) (*LRUCache[K, V], error) {
	c, err := lru.New[K, V](capacity)
	if err != nil {
		return nil, err
	}
	return &LRUCache[K, V]{
		cache:    c,
		capacity: capacity,
		stats:    CacheStats{},
	}, nil
}

// Get retrieves an item from the cache.
func (l *LRUCache[K, V]) Get(key K) (V, bool) {
	// NOTE: hashicorp/golang-lru Cache.Get mutates internal state (LRU recency),
	// so this must take a full write lock. Using RLock here can lead to
	// concurrent map/list mutations and data races under load.
	l.mu.Lock()
	defer l.mu.Unlock()

	val, ok := l.cache.Get(key)
	if ok {
		l.stats.Hits++
	} else {
		l.stats.Misses++
	}
	return val, ok
}

// Put stores an item in the cache.
func (l *LRUCache[K, V]) Put(key K, value V) {
	l.mu.Lock()
	defer l.mu.Unlock()

	evicted := l.cache.Add(key, value)
	if evicted {
		l.stats.Evictions++
	}
}

// Contains checks if a key exists in the cache without affecting LRU order.
func (l *LRUCache[K, V]) Contains(key K) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cache.Contains(key)
}

// Remove removes an item from the cache.
func (l *LRUCache[K, V]) Remove(key K) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cache.Remove(key)
}

// Clear removes all items from the cache.
func (l *LRUCache[K, V]) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache.Purge()
}

// Size returns the current number of items in the cache.
func (l *LRUCache[K, V]) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cache.Len()
}

// Capacity returns the maximum capacity of the cache.
func (l *LRUCache[K, V]) Capacity() int {
	return l.capacity
}

// Stats returns cache performance statistics.
func (l *LRUCache[K, V]) Stats() CacheStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := l.stats.Hits + l.stats.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(l.stats.Hits) / float64(total)
	}

	return CacheStats{
		Hits:      l.stats.Hits,
		Misses:    l.stats.Misses,
		Evictions: l.stats.Evictions,
		HitRate:   hitRate,
	}
}

// ResetStats resets all statistics.
func (l *LRUCache[K, V]) ResetStats() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats = CacheStats{}
}

