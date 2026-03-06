package alert

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// Deduplicator prevents duplicate alerts within a time window.
type Deduplicator struct {
	window time.Duration
	alerts map[string]*AlertEntry
	mu     sync.RWMutex
	stats  *DeduplicationStats
}

// AlertEntry represents an alert in the deduplication cache.
type AlertEntry struct {
	Alert      *domain.Alert
	Count      int
	FirstSeen  time.Time
	LastSeen   time.Time
	Suppressed int
}

// DeduplicationStats tracks deduplication statistics.
type DeduplicationStats struct {
	TotalAlerts      uint64
	UniqueAlerts     uint64
	DuplicateAlerts  uint64
	SuppressedAlerts uint64
	mu               sync.RWMutex
}

// NewDeduplicator creates a new deduplicator with the specified time window.
func NewDeduplicator(window time.Duration) *Deduplicator {
	if window <= 0 {
		window = time.Hour // Default: 1 hour
	}

	return &Deduplicator{
		window: window,
		alerts: make(map[string]*AlertEntry),
		stats:  &DeduplicationStats{},
	}
}

// Deduplicate processes alerts and removes duplicates.
func (d *Deduplicator) Deduplicate(alerts []*domain.Alert) []*domain.Alert {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var result []*domain.Alert

	// Clean old entries (outside time window)
	d.cleanOldEntries(now)

	// Process incoming alerts
	for _, alert := range alerts {
		if alert == nil {
			continue
		}

		signature := d.generateSignature(alert)
		d.stats.mu.Lock()
		d.stats.TotalAlerts++
		d.stats.mu.Unlock()

		if existing, found := d.alerts[signature]; found {
			// Duplicate detected
			existing.Count++
			existing.LastSeen = now
			existing.Suppressed++
			alert.FalsePositiveRisk = 0.9 // Mark as likely duplicate
			alert.Suppressed = true
			d.stats.mu.Lock()
			d.stats.DuplicateAlerts++
			d.stats.SuppressedAlerts++
			d.stats.mu.Unlock()
		} else {
			// New alert
			alert.FalsePositiveRisk = 0.0
			alert.Suppressed = false
			d.alerts[signature] = &AlertEntry{
				Alert:     alert,
				Count:     1,
				FirstSeen: now,
				LastSeen:  now,
			}
			result = append(result, alert)
			d.stats.mu.Lock()
			d.stats.UniqueAlerts++
			d.stats.mu.Unlock()
		}
	}

	return result
}

// generateSignature creates a unique signature for an alert.
func (d *Deduplicator) generateSignature(alert *domain.Alert) string {
	h := fnv.New64a()
	h.Write([]byte(alert.RuleID))
	h.Write([]byte(alert.RuleTitle))

	// Hash matched fields (critical fields only for performance)
	criticalFields := []string{"Image", "CommandLine", "ParentImage", "User", "TargetFilename"}
	for _, field := range criticalFields {
		if value, ok := alert.MatchedFields[field]; ok {
			h.Write([]byte(field))
			h.Write([]byte(fmt.Sprintf("%v", value)))
		}
	}

	// Include confidence level (rounded to 0.1 precision)
	confidenceLevel := int(alert.Confidence * 10)
	h.Write([]byte(fmt.Sprintf("%d", confidenceLevel)))

	return fmt.Sprintf("%x", h.Sum64())
}

// cleanOldEntries removes alerts outside the time window.
func (d *Deduplicator) cleanOldEntries(now time.Time) {
	for signature, entry := range d.alerts {
		if now.Sub(entry.LastSeen) > d.window {
			delete(d.alerts, signature)
		}
	}
}

// Stats returns deduplication statistics.
func (d *Deduplicator) Stats() DeduplicationStats {
	d.stats.mu.RLock()
	defer d.stats.mu.RUnlock()

	return DeduplicationStats{
		TotalAlerts:      d.stats.TotalAlerts,
		UniqueAlerts:     d.stats.UniqueAlerts,
		DuplicateAlerts:  d.stats.DuplicateAlerts,
		SuppressedAlerts: d.stats.SuppressedAlerts,
	}
}

// ResetStats resets deduplication statistics.
func (d *Deduplicator) ResetStats() {
	d.stats.mu.Lock()
	defer d.stats.mu.Unlock()

	d.stats.TotalAlerts = 0
	d.stats.UniqueAlerts = 0
	d.stats.DuplicateAlerts = 0
	d.stats.SuppressedAlerts = 0
}

// Size returns the number of alerts in the deduplication cache.
func (d *Deduplicator) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.alerts)
}

// Clear clears all alerts from the deduplication cache.
func (d *Deduplicator) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.alerts = make(map[string]*AlertEntry)
}

