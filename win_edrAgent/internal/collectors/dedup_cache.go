// Package collectors — Time-windowed event deduplication cache.
//
// Reduces event volume by suppressing duplicate events for the same
// resource within a configurable time window. The FIRST event in every
// window is always emitted (so Sigma rules still match); only subsequent
// duplicates within the TTL are suppressed.
//
// Thread-safe: uses sync.Map for lock-free concurrent reads + periodic
// cleanup via a background goroutine.
//
//go:build windows
// +build windows

package collectors

import (
	"sync"
	"sync/atomic"
	"time"
)

// DedupCache provides time-windowed deduplication.
// The key is an opaque string (caller-defined), and the value is the
// timestamp of the first occurrence. If a key is seen again within the
// TTL, IsDuplicate returns true.
type DedupCache struct {
	entries sync.Map    // map[string]int64 (Unix nanoseconds)
	ttl     int64       // nanoseconds
	stopped atomic.Bool

	// Metrics
	Hits   atomic.Uint64 // duplicates suppressed
	Misses atomic.Uint64 // unique events passed through
}

// NewDedupCache creates a new dedup cache.
// ttl controls how long a key is remembered (duplicates within this window
// are suppressed). cleanupInterval controls how often expired entries are
// purged from memory.
func NewDedupCache(ttl time.Duration, cleanupInterval time.Duration) *DedupCache {
	dc := &DedupCache{
		ttl: ttl.Nanoseconds(),
	}
	go dc.cleanupLoop(cleanupInterval)
	return dc
}

// IsDuplicate returns true if the key was seen within the TTL window.
// If false (first occurrence), the key is recorded.
func (dc *DedupCache) IsDuplicate(key string) bool {
	now := time.Now().UnixNano()

	if prev, loaded := dc.entries.Load(key); loaded {
		if now-prev.(int64) < dc.ttl {
			dc.Hits.Add(1)
			return true
		}
		// TTL expired — treat as new
	}

	dc.entries.Store(key, now)
	dc.Misses.Add(1)
	return false
}

// Stop halts the background cleanup goroutine.
func (dc *DedupCache) Stop() {
	dc.stopped.Store(true)
}

// cleanupLoop periodically removes expired entries to bound memory usage.
func (dc *DedupCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if dc.stopped.Load() {
			return
		}
		now := time.Now().UnixNano()
		dc.entries.Range(func(key, value interface{}) bool {
			if now-value.(int64) > dc.ttl {
				dc.entries.Delete(key)
			}
			return true
		})
	}
}
