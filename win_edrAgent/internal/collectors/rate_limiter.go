// Package collectors provides a thread-safe Token Bucket rate limiter for QoS
// enforcement at the agent edge. Critical/High severity events bypass rate limiting
// entirely, ensuring zero data loss for security-relevant telemetry.
package collectors

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// tokenBucket implements the token bucket algorithm for a single event type.
// Tokens refill at a constant rate (refillRate tokens/second). When a token is
// consumed, the event passes; when no tokens remain, the event is rate-limited.
type tokenBucket struct {
	tokens    float64
	maxTokens float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// refill adds tokens based on elapsed time since last refill.
func (tb *tokenBucket) refill(now time.Time) {
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
}

// tryConsume attempts to consume one token. Returns true if a token was available.
func (tb *tokenBucket) tryConsume(now time.Time) bool {
	tb.refill(now)
	if tb.tokens >= 1.0 {
		tb.tokens--
		return true
	}
	return false
}

// RateLimiter enforces per-event-type rate limits using token buckets.
//
// Thread safety:
//   - The buckets map is protected by sync.RWMutex because Allow() is called from
//     the high-frequency event pipeline goroutine while UpdateLimits() can be called
//     from the config update goroutine.
//   - droppedCount uses sync/atomic for zero-lock metrics reads from the heartbeat goroutine.
type RateLimiter struct {
	mu             sync.RWMutex
	enabled        bool
	criticalBypass bool
	defaultMaxEPS  int
	buckets        map[event.EventType]*tokenBucket
	droppedCount   atomic.Uint64
	logger         *logging.Logger
}

// NewRateLimiter creates a rate limiter from the given configuration.
func NewRateLimiter(cfg config.RateLimitConfig, logger *logging.Logger) *RateLimiter {
	rl := &RateLimiter{
		enabled:        cfg.Enabled,
		criticalBypass: cfg.CriticalBypass,
		defaultMaxEPS:  cfg.DefaultMaxEPS,
		buckets:        make(map[event.EventType]*tokenBucket),
		logger:         logger,
	}

	now := time.Now()

	// Initialize per-event-type buckets from config
	for eventTypeStr, maxEPS := range cfg.PerEventType {
		if maxEPS <= 0 {
			continue
		}
		et := event.EventType(eventTypeStr)
		rl.buckets[et] = &tokenBucket{
			tokens:     float64(maxEPS), // Start full
			maxTokens:  float64(maxEPS), // Burst capacity = 1 second of tokens
			refillRate: float64(maxEPS),
			lastRefill: now,
		}
	}

	if logger != nil && cfg.Enabled {
		logger.Infof("RateLimiter initialized: default_eps=%d, critical_bypass=%v, per_type_rules=%d",
			cfg.DefaultMaxEPS, cfg.CriticalBypass, len(cfg.PerEventType))
	}

	return rl
}

// Allow checks if an event of the given type and severity is allowed through
// the rate limiter. Returns true if the event should pass, false if rate-limited.
//
// Critical/High severity events always return true when CriticalBypass is enabled,
// ensuring zero data loss for security-critical telemetry.
func (rl *RateLimiter) Allow(eventType event.EventType, severity event.Severity) bool {
	if !rl.enabled {
		return true
	}

	// Critical Bypass — high-severity events must NEVER be dropped
	if rl.criticalBypass && (severity == event.SeverityCritical || severity == event.SeverityHigh) {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Look up per-type bucket
	bucket, exists := rl.buckets[eventType]
	if !exists {
		// No explicit rule — use default limit (0 = unlimited)
		if rl.defaultMaxEPS <= 0 {
			return true
		}
		// Lazily create a bucket for this event type with default limits
		bucket = &tokenBucket{
			tokens:     float64(rl.defaultMaxEPS),
			maxTokens:  float64(rl.defaultMaxEPS),
			refillRate: float64(rl.defaultMaxEPS),
			lastRefill: now,
		}
		rl.buckets[eventType] = bucket
	}

	if bucket.tryConsume(now) {
		return true
	}

	// Rate limited — increment atomic counter
	rl.droppedCount.Add(1)
	return false
}

// DroppedCount returns the total number of events dropped by rate limiting.
// Uses atomic load — safe to call from any goroutine without locking.
func (rl *RateLimiter) DroppedCount() uint64 {
	return rl.droppedCount.Load()
}

// ResetDroppedCount atomically resets the dropped counter and returns the
// previous value. Useful for per-heartbeat-window reporting.
func (rl *RateLimiter) ResetDroppedCount() uint64 {
	return rl.droppedCount.Swap(0)
}

// UpdateLimits reconfigures the rate limiter with a new configuration.
// Thread-safe: acquires write lock before mutating bucket state.
func (rl *RateLimiter) UpdateLimits(cfg config.RateLimitConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.enabled = cfg.Enabled
	rl.criticalBypass = cfg.CriticalBypass
	rl.defaultMaxEPS = cfg.DefaultMaxEPS

	now := time.Now()

	// Rebuild buckets — preserve token state for existing types where possible
	newBuckets := make(map[event.EventType]*tokenBucket)
	for eventTypeStr, maxEPS := range cfg.PerEventType {
		if maxEPS <= 0 {
			continue
		}
		et := event.EventType(eventTypeStr)
		if existing, ok := rl.buckets[et]; ok {
			// Preserve remaining tokens but update rate
			existing.maxTokens = float64(maxEPS)
			existing.refillRate = float64(maxEPS)
			if existing.tokens > existing.maxTokens {
				existing.tokens = existing.maxTokens
			}
			newBuckets[et] = existing
		} else {
			newBuckets[et] = &tokenBucket{
				tokens:     float64(maxEPS),
				maxTokens:  float64(maxEPS),
				refillRate: float64(maxEPS),
				lastRefill: now,
			}
		}
	}
	rl.buckets = newBuckets

	if rl.logger != nil {
		rl.logger.Infof("RateLimiter reconfigured: enabled=%v, default_eps=%d, rules=%d",
			rl.enabled, rl.defaultMaxEPS, len(newBuckets))
	}
}
