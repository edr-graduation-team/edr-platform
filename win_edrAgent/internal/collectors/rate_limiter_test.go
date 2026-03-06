package collectors

import (
	"testing"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
)

func TestRateLimiterDisabled(t *testing.T) {
	cfg := config.RateLimitConfig{Enabled: false}
	rl := NewRateLimiter(cfg, nil)

	// When disabled, all events should pass
	for i := 0; i < 1000; i++ {
		if !rl.Allow(event.EventTypeDNS, event.SeverityLow) {
			t.Fatal("disabled limiter should allow all events")
		}
	}
	if rl.DroppedCount() != 0 {
		t.Errorf("expected 0 drops, got %d", rl.DroppedCount())
	}
}

func TestRateLimiterCriticalBypass(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:        true,
		CriticalBypass: true,
		PerEventType:   map[string]int{"process": 1}, // 1 EPS — very restrictive
	}
	rl := NewRateLimiter(cfg, nil)

	// First event consumes the only token
	if !rl.Allow(event.EventTypeProcess, event.SeverityLow) {
		t.Fatal("first event should pass")
	}

	// Second low-severity event should be rate-limited
	if rl.Allow(event.EventTypeProcess, event.SeverityLow) {
		t.Fatal("second low-severity event should be rate-limited")
	}

	// Critical severity should ALWAYS pass regardless of token state
	if !rl.Allow(event.EventTypeProcess, event.SeverityCritical) {
		t.Fatal("critical event must bypass rate limiter")
	}

	// High severity should also bypass
	if !rl.Allow(event.EventTypeProcess, event.SeverityHigh) {
		t.Fatal("high severity event must bypass rate limiter")
	}
}

func TestRateLimiterPerEventType(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		PerEventType: map[string]int{
			"dns":     2, // 2 EPS for DNS
			"network": 3, // 3 EPS for network
		},
		DefaultMaxEPS: 100, // High default → effectively unlimited for other types
	}
	rl := NewRateLimiter(cfg, nil)

	// DNS: first 2 should pass, 3rd should be limited
	for i := 0; i < 2; i++ {
		if !rl.Allow(event.EventTypeDNS, event.SeverityLow) {
			t.Fatalf("DNS event %d should pass", i)
		}
	}
	if rl.Allow(event.EventTypeDNS, event.SeverityLow) {
		t.Fatal("3rd DNS event should be rate-limited")
	}

	// Network: first 3 should pass
	for i := 0; i < 3; i++ {
		if !rl.Allow(event.EventTypeNetwork, event.SeverityLow) {
			t.Fatalf("network event %d should pass", i)
		}
	}
	if rl.Allow(event.EventTypeNetwork, event.SeverityLow) {
		t.Fatal("4th network event should be rate-limited")
	}

	// Registry has no explicit limit → uses default (100) → should pass
	if !rl.Allow(event.EventTypeRegistry, event.SeverityLow) {
		t.Fatal("registry event should pass with default limit")
	}
}

func TestRateLimiterDroppedCount(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:      true,
		PerEventType: map[string]int{"dns": 1},
	}
	rl := NewRateLimiter(cfg, nil)

	// First passes, rest drop
	rl.Allow(event.EventTypeDNS, event.SeverityLow)
	rl.Allow(event.EventTypeDNS, event.SeverityLow)
	rl.Allow(event.EventTypeDNS, event.SeverityLow)

	if rl.DroppedCount() != 2 {
		t.Errorf("expected 2 drops, got %d", rl.DroppedCount())
	}

	// Test reset
	prev := rl.ResetDroppedCount()
	if prev != 2 {
		t.Errorf("expected reset to return 2, got %d", prev)
	}
	if rl.DroppedCount() != 0 {
		t.Errorf("expected 0 after reset, got %d", rl.DroppedCount())
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:      true,
		PerEventType: map[string]int{"dns": 10},
	}
	rl := NewRateLimiter(cfg, nil)

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		rl.Allow(event.EventTypeDNS, event.SeverityLow)
	}

	// Should be rate-limited now
	if rl.Allow(event.EventTypeDNS, event.SeverityLow) {
		t.Fatal("should be rate-limited after exhausting tokens")
	}

	// Wait for refill (100ms = 1 token at 10/s)
	time.Sleep(150 * time.Millisecond)

	// Should have at least 1 token now
	if !rl.Allow(event.EventTypeDNS, event.SeverityLow) {
		t.Fatal("should have refilled at least 1 token after 150ms")
	}
}

func TestRateLimiterUpdateLimits(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:      true,
		PerEventType: map[string]int{"dns": 100},
	}
	rl := NewRateLimiter(cfg, nil)

	// Update to very restrictive
	newCfg := config.RateLimitConfig{
		Enabled:      true,
		PerEventType: map[string]int{"dns": 1},
	}
	rl.UpdateLimits(newCfg)

	// First passes
	if !rl.Allow(event.EventTypeDNS, event.SeverityLow) {
		t.Fatal("first event after update should pass")
	}
	// Second should be limited
	if rl.Allow(event.EventTypeDNS, event.SeverityLow) {
		t.Fatal("second event should be rate-limited after restrictive update")
	}
}

func TestFilterEventID(t *testing.T) {
	cfg := FilterConfig{
		ExcludeEventIDs: []int{4, 7, 15, 22},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		eventID      interface{}
		shouldFilter bool
	}{
		{"excluded ID 4 (int)", 4, true},
		{"excluded ID 7 (int)", 7, true},
		{"excluded ID 22 (int)", 22, true},
		{"excluded ID 15 (float64)", float64(15), true},
		{"excluded ID 4 (string)", "4", true},
		{"allowed ID 1 (int)", 1, false},
		{"allowed ID 3 (int)", 3, false},
		{"allowed ID 10 (int)", 10, false},
		{"no event_id field", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{}
			if tt.eventID != nil {
				data["event_id"] = tt.eventID
			}
			evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, data)

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterTrustedHash(t *testing.T) {
	cfg := FilterConfig{
		TrustedHashes: []string{
			"abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
			"UPPERCASE999HASH000UPPERCASE999HASH000UPPERCASE999HASH000UPPER",
		},
	}
	filter := NewFilter(cfg, nil)

	tests := []struct {
		name         string
		hash         string
		shouldFilter bool
	}{
		{"exact match lowercase", "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", true},
		{"case insensitive match", "UPPERCASE999HASH000UPPERCASE999HASH000UPPERCASE999HASH000UPPER", true},
		{"unknown hash", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", false},
		{"empty hash", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{}
			if tt.hash != "" {
				data["hash_sha256"] = tt.hash
			}
			evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, data)

			result := filter.ShouldFilter(evt)
			if result != tt.shouldFilter {
				t.Errorf("%s: expected filter=%v, got %v", tt.name, tt.shouldFilter, result)
			}
		})
	}
}

func TestFilterDroppedCount(t *testing.T) {
	cfg := FilterConfig{
		ExcludeProcesses: []string{"svchost.exe"},
		ExcludeEventIDs:  []int{22},
	}
	filter := NewFilter(cfg, nil)

	// Generate some noise
	for i := 0; i < 5; i++ {
		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"name": "svchost.exe",
		})
		filter.ShouldFilter(evt)
	}

	for i := 0; i < 3; i++ {
		evt := event.NewEvent(event.EventTypeDNS, event.SeverityLow, map[string]interface{}{
			"event_id": 22,
		})
		filter.ShouldFilter(evt)
	}

	// 2 non-matching events
	for i := 0; i < 2; i++ {
		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"name": "powershell.exe",
		})
		filter.ShouldFilter(evt)
	}

	if filter.DroppedCount() != 8 {
		t.Errorf("expected 8 dropped, got %d", filter.DroppedCount())
	}

	stats := filter.Stats()
	if stats.TotalEvents != 10 {
		t.Errorf("expected 10 total, got %d", stats.TotalEvents)
	}
	if stats.PassedEvents != 2 {
		t.Errorf("expected 2 passed, got %d", stats.PassedEvents)
	}
}

func BenchmarkFilterEventID(b *testing.B) {
	cfg := FilterConfig{
		ExcludeEventIDs: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	}
	filter := NewFilter(cfg, nil)

	evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
		"event_id": 99, // Not excluded — worst case path
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldFilter(evt)
	}
}
