package cache

import (
	"sync"
)

// FieldResolutionCache caches field resolution paths for frequently accessed fields.
// Thread-safe and optimized for high-concurrency scenarios.
type FieldResolutionCache struct {
	cache *LRUCache[string, interface{}]
	mu    sync.RWMutex
}

// NewFieldResolutionCache creates a new field resolution cache with the specified capacity.
func NewFieldResolutionCache(capacity int) (*FieldResolutionCache, error) {
	cache, err := NewLRUCache[string, interface{}](capacity)
	if err != nil {
		return nil, err
	}
	return &FieldResolutionCache{
		cache: cache,
	}, nil
}

// Get retrieves a cached field value.
func (frc *FieldResolutionCache) Get(fieldPath string) (interface{}, bool) {
	return frc.cache.Get(fieldPath)
}

// Put stores a field value in the cache.
func (frc *FieldResolutionCache) Put(fieldPath string, value interface{}) {
	frc.cache.Put(fieldPath, value)
}

// Clear removes all cached entries.
func (frc *FieldResolutionCache) Clear() {
	frc.cache.Clear()
}

// Size returns the current number of cached entries.
func (frc *FieldResolutionCache) Size() int {
	return frc.cache.Size()
}

// Capacity returns the maximum capacity of the cache.
func (frc *FieldResolutionCache) Capacity() int {
	return frc.cache.Capacity()
}

// Stats returns cache performance statistics.
func (frc *FieldResolutionCache) Stats() CacheStats {
	return frc.cache.Stats()
}

// ResetStats resets all statistics.
func (frc *FieldResolutionCache) ResetStats() {
	frc.cache.ResetStats()
}

