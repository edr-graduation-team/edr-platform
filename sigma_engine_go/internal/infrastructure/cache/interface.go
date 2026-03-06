package cache

// Cache defines the interface for a generic cache implementation.
type Cache[K comparable, V any] interface {
	Get(key K) (V, bool)
	Put(key K, value V)
	Contains(key K) bool
	Remove(key K) bool
	Clear()
	Size() int
	Capacity() int
}

// CacheStats represents statistics for cache performance monitoring.
type CacheStats struct {
	Hits       int     `json:"hits"`
	Misses     int     `json:"misses"`
	Evictions  int     `json:"evictions"`
	HitRate    float64 `json:"hit_rate"`
}

// StatsCache extends Cache with statistics tracking.
type StatsCache[K comparable, V any] interface {
	Cache[K, V]
	Stats() CacheStats
	ResetStats()
}

// RegexCache defines the interface for a regex pattern cache.
type RegexCache interface {
	GetOrCompile(pattern string, flags int) (interface{}, error)
	Clear()
	Stats() CacheStats
}

