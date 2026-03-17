package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// NoopLineageCache is a no-op implementation of LineageCache used when
// Redis is unavailable (e.g. development without Docker, Kafka-only integration
// tests). All write operations succeed silently; all read operations return
// cache misses. This allows the risk scoring pipeline to run without crashing
// while emitting "lineage unavailable" log lines upstream.
type NoopLineageCache struct {
	missCount atomic.Uint64
}

// NewNoopLineageCache returns a new no-op cache.
func NewNoopLineageCache() *NoopLineageCache {
	return &NoopLineageCache{}
}

func (n *NoopLineageCache) WriteEntry(_ context.Context, _ *ProcessLineageEntry) error {
	return nil
}

func (n *NoopLineageCache) GetEntry(_ context.Context, _ string, _ int64) (*ProcessLineageEntry, error) {
	n.missCount.Add(1)
	return nil, nil
}

func (n *NoopLineageCache) GetLineageChain(_ context.Context, _ string, _ int64) ([]*ProcessLineageEntry, error) {
	n.missCount.Add(1)
	return nil, nil
}

func (n *NoopLineageCache) Ping(_ context.Context) error {
	return nil
}

// MissCount returns the total number of Get operations that returned a miss.
// Useful for monitoring how often lineage data was unavailable.
func (n *NoopLineageCache) MissCount() uint64 {
	return n.missCount.Load()
}

// =============================================================================
// InMemoryLineageCache — lightweight non-Redis cache for unit tests
// =============================================================================

// InMemoryLineageCache stores lineage entries in a thread-safe Go map.
// Entries expire after the given TTL (checked lazily on Get).
// This is suitable for unit tests and avoids a real Redis dependency.
type InMemoryLineageCache struct {
	mu      sync.RWMutex
	entries map[string]*inMemoryEntry
	ttl     time.Duration
}

type inMemoryEntry struct {
	data      *ProcessLineageEntry
	expiresAt time.Time
}

// NewInMemoryLineageCache creates a new in-memory lineage cache with the given TTL.
func NewInMemoryLineageCache(ttl time.Duration) *InMemoryLineageCache {
	if ttl <= 0 {
		ttl = lineageTTL
	}
	return &InMemoryLineageCache{
		entries: make(map[string]*inMemoryEntry),
		ttl:     ttl,
	}
}

func (m *InMemoryLineageCache) WriteEntry(_ context.Context, entry *ProcessLineageEntry) error {
	if entry == nil || entry.AgentID == "" || entry.PID == 0 {
		return nil
	}
	key := buildKey(entry.AgentID, entry.PID)
	m.mu.Lock()
	m.entries[key] = &inMemoryEntry{data: entry, expiresAt: time.Now().Add(m.ttl)}
	m.mu.Unlock()
	return nil
}

func (m *InMemoryLineageCache) GetEntry(_ context.Context, agentID string, pid int64) (*ProcessLineageEntry, error) {
	key := buildKey(agentID, pid)
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, nil
	}
	return e.data, nil
}

func (m *InMemoryLineageCache) GetLineageChain(ctx context.Context, agentID string, pid int64) ([]*ProcessLineageEntry, error) {
	chain := make([]*ProcessLineageEntry, 0, maxLineageDepth)
	visited := make(map[int64]bool, maxLineageDepth)
	currentPID := pid

	for depth := 0; depth < maxLineageDepth; depth++ {
		if currentPID == 0 || visited[currentPID] {
			break
		}
		visited[currentPID] = true

		entry, err := m.GetEntry(ctx, agentID, currentPID)
		if err != nil || entry == nil {
			break
		}
		chain = append(chain, entry)
		currentPID = entry.PPID
	}
	return chain, nil
}

func (m *InMemoryLineageCache) Ping(_ context.Context) error { return nil }

// Size returns the current number of cached entries (including expired ones
// not yet evicted). Useful for test assertions.
func (m *InMemoryLineageCache) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}
