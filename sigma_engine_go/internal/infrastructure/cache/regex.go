package cache

import (
	"fmt"
	"regexp"
	"sync"
)

// RegexCacheImpl is a thread-safe cache for compiled regex patterns.
type RegexCacheImpl struct {
	cache *LRUCache[string, *regexp.Regexp]
	mu    sync.RWMutex
}

// NewRegexCache creates a new regex cache with the specified capacity.
func NewRegexCache(capacity int) (*RegexCacheImpl, error) {
	cache, err := NewLRUCache[string, *regexp.Regexp](capacity)
	if err != nil {
		return nil, err
	}
	return &RegexCacheImpl{
		cache: cache,
	}, nil
}

// GetOrCompile retrieves a compiled regex pattern from cache or compiles it if missing.
// Flags are currently ignored as Go's regexp doesn't support flags the same way.
func (r *RegexCacheImpl) GetOrCompile(pattern string, flags int) (interface{}, error) {
	// Serialize "check → compile → store" to avoid thundering-herd compilation
	// under high concurrency. The underlying LRUCache is itself thread-safe,
	// but this lock prevents redundant compiles of the same pattern.
	r.mu.Lock()
	defer r.mu.Unlock()

	if compiled, ok := r.cache.Get(pattern); ok {
		return compiled, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex pattern %q: %w", pattern, err)
	}

	r.cache.Put(pattern, compiled)
	return compiled, nil
}

// Clear removes all cached patterns.
func (r *RegexCacheImpl) Clear() {
	r.cache.Clear()
}

// Stats returns cache statistics.
func (r *RegexCacheImpl) Stats() CacheStats {
	return r.cache.Stats()
}

