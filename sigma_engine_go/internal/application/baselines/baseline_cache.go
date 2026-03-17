// Package baselines provides BaselineCache — a thin in-process read cache
// that sits in front of the BaselineRepository for scoring-time lookups.
//
// Motivation:
//
//	The RiskScorer is called synchronously inside the EventLoop's detection
//	worker goroutines.  Each call would normally require a DB roundtrip to
//	fetch the baseline for a (agent, process, hour) tuple.  At typical SOC
//	event rates this would add 2–5 ms per alert.
//
//	BaselineCache uses a simple TTL map: entries are cached for 30 minutes,
//	which is long enough to be highly effective (hot processes repeat rapidly)
//	while staying fresh enough that a machine learning a new process pattern
//	quickly appears in the scorer without a service restart.
package baselines

import (
	"context"
	"sync"
	"time"
)

const (
	// defaultCacheTTL is how long a baseline entry is held in memory.
	// 30 minutes balances freshness vs DB load.
	defaultCacheTTL = 30 * time.Minute

	// defaultCleanupInterval controls how often expired entries are evicted.
	defaultCleanupInterval = 5 * time.Minute
)

// =============================================================================
// BaselineProvider interface
// =============================================================================

// BaselineProvider is the interface the RiskScorer uses to look up baselines.
// Implemented by BaselineCache (production) and InMemoryBaselineRepository
// (unit tests, via the adapter below).
type BaselineProvider interface {
	// Lookup returns the baseline for the given (agentID, processName, hourOfDay).
	// Returns nil, nil if no baseline is found (process not yet profiled).
	Lookup(ctx context.Context, agentID, processName string, hourOfDay int) (*ProcessBaseline, error)
}

// =============================================================================
// BaselineCache
// =============================================================================

type cacheEntry struct {
	baseline  *ProcessBaseline // nil → "not profiled" (negative cached)
	expiresAt time.Time
}

// BaselineCache wraps a BaselineRepository with an in-process TTL cache.
type BaselineCache struct {
	repo    BaselineRepository
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

// NewBaselineCache creates a new read cache around the given repository.
// ttl=0 uses the default (30 minutes).
func NewBaselineCache(repo BaselineRepository, ttl time.Duration) *BaselineCache {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	c := &BaselineCache{
		repo:    repo,
		ttl:     ttl,
		entries: make(map[string]*cacheEntry),
	}
	go c.cleanupLoop()
	return c
}

// cacheKey returns a compact string key for the cache map.
func cacheKey(agentID, processName string, hourOfDay int) string {
	return agentID + "|" + processName + "|" + string(rune('0'+hourOfDay/10)) + string(rune('0'+hourOfDay%10))
}

// Lookup returns the cached baseline, fetching from DB if not cached.
func (c *BaselineCache) Lookup(ctx context.Context, agentID, processName string, hourOfDay int) (*ProcessBaseline, error) {
	key := cacheKey(agentID, processName, hourOfDay)

	// Fast path: cache hit
	c.mu.RLock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.baseline, nil
	}
	c.mu.RUnlock()

	// Slow path: DB fetch
	baseline, err := c.repo.GetBaseline(ctx, agentID, processName, hourOfDay)
	if err != nil {
		return nil, err
	}

	// Cache the result (including nil → negative cache so we don't hammer DB for new processes)
	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		baseline:  baseline,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return baseline, nil
}

// Invalidate removes a cached entry (call after a successful Upsert in tests).
func (c *BaselineCache) Invalidate(agentID, processName string, hourOfDay int) {
	key := cacheKey(agentID, processName, hourOfDay)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// cleanupLoop evicts expired entries periodically to prevent unbounded growth.
func (c *BaselineCache) cleanupLoop() {
	ticker := time.NewTicker(defaultCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// =============================================================================
// RepositoryProviderAdapter
// =============================================================================

// RepositoryProviderAdapter adapts an InMemoryBaselineRepository to the
// BaselineProvider interface for use in unit tests that bypass the cache.
type RepositoryProviderAdapter struct {
	repo BaselineRepository
}

// NewRepositoryProviderAdapter wraps any BaselineRepository as a BaselineProvider.
func NewRepositoryProviderAdapter(repo BaselineRepository) *RepositoryProviderAdapter {
	return &RepositoryProviderAdapter{repo: repo}
}

// Lookup delegates directly to the repository (no caching).
func (a *RepositoryProviderAdapter) Lookup(ctx context.Context, agentID, processName string, hourOfDay int) (*ProcessBaseline, error) {
	return a.repo.GetBaseline(ctx, agentID, processName, hourOfDay)
}

// =============================================================================
// NoopBaselineProvider (for when DB is unavailable)
// =============================================================================

// NoopBaselineProvider always returns nil (no baseline profiled).
// Used when the PostgreSQL pool is not available at startup.
type NoopBaselineProvider struct{}

// Lookup always returns nil, nil.
func (NoopBaselineProvider) Lookup(_ context.Context, _, _ string, _ int) (*ProcessBaseline, error) {
	return nil, nil
}
