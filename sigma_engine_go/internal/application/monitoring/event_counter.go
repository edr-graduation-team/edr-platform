package monitoring

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// EventCounter counts events within a time window and tracks statistics.
// It generates unique signatures for event groups, calculates rates and trends,
// and automatically cleans up old events outside the window.
type EventCounter struct {
	windowSize time.Duration

	// Event groups: signature -> eventGroup
	eventGroups map[string]*eventGroup
	mu          sync.RWMutex

	// Configuration
	alertThreshold      int
	rateThresholdPerMin float64

	// Statistics
	stats *CounterStats
}

// eventGroup represents a group of similar events.
type eventGroup struct {
	Signature    string
	FirstSeen    time.Time
	LastSeen     time.Time
	Occurrences  []time.Time // Sorted timestamps
	Count        int
}

// CounterStats tracks event counter statistics.
type CounterStats struct {
	TotalEventsRecorded int64
	UniqueEventGroups   int64
	EventsCleaned       int64
	mu                  sync.RWMutex
}

// EventStatistics represents statistics for an event group.
type EventStatistics struct {
	EventCount    int
	FirstSeen     time.Time
	LastSeen      time.Time
	RatePerMinute float64
	CountTrend    string // "↑" (uptrend), "↓" (downtrend), "→" (stable)
	WindowSize    time.Duration
}

// NewEventCounter creates a new event counter.
// Parameters:
//   - windowSize: Time window for counting events (default: 5 minutes)
//   - alertThreshold: Alert if count >= this threshold (default: 10)
//   - rateThresholdPerMin: Alert if rate >= this threshold (default: 5.0)
func NewEventCounter(windowSize time.Duration, alertThreshold int, rateThresholdPerMin float64) *EventCounter {
	if windowSize <= 0 {
		windowSize = 5 * time.Minute
	}
	if alertThreshold <= 0 {
		alertThreshold = 10
	}
	if rateThresholdPerMin <= 0 {
		rateThresholdPerMin = 5.0
	}

	return &EventCounter{
		windowSize:          windowSize,
		eventGroups:         make(map[string]*eventGroup),
		alertThreshold:      alertThreshold,
		rateThresholdPerMin: rateThresholdPerMin,
		stats:               &CounterStats{},
	}
}

// RecordEvent records an event and returns its signature.
// The signature is generated from the event's key characteristics.
func (ec *EventCounter) RecordEvent(event *domain.LogEvent) string {
	if event == nil {
		return ""
	}

	// Generate signature
	signature := ec.generateSignature(event)

	// Record event
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now()
	group, exists := ec.eventGroups[signature]

	if !exists {
		// Create new group
		group = &eventGroup{
			Signature:   signature,
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: []time.Time{now},
			Count:       1,
		}
		ec.eventGroups[signature] = group
		ec.stats.mu.Lock()
		ec.stats.UniqueEventGroups++
		ec.stats.mu.Unlock()
	} else {
		// Add to existing group
		group.Occurrences = append(group.Occurrences, now)
		group.Count++
		group.LastSeen = now
		// Keep occurrences sorted
		sort.Slice(group.Occurrences, func(i, j int) bool {
			return group.Occurrences[i].Before(group.Occurrences[j])
		})
	}

	// Clean old occurrences (outside window)
	ec.cleanOldOccurrences(group, now)

	ec.stats.mu.Lock()
	ec.stats.TotalEventsRecorded++
	ec.stats.mu.Unlock()

	return signature
}

// GetStatistics returns statistics for an event signature.
func (ec *EventCounter) GetStatistics(signature string) (*EventStatistics, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	group, exists := ec.eventGroups[signature]
	if !exists {
		return nil, false
	}

	now := time.Now()
	// Clean old occurrences before calculating stats
	ec.cleanOldOccurrences(group, now)

	// Recalculate count after cleanup
	group.Count = len(group.Occurrences)

	if group.Count == 0 {
		return nil, false
	}

	// Calculate rate per minute
	windowMinutes := ec.windowSize.Minutes()
	ratePerMinute := float64(group.Count) / windowMinutes

	// Calculate trend
	trend := ec.calculateTrend(group.Occurrences)

	return &EventStatistics{
		EventCount:    group.Count,
		FirstSeen:     group.FirstSeen,
		LastSeen:      group.LastSeen,
		RatePerMinute: ratePerMinute,
		CountTrend:    trend,
		WindowSize:    ec.windowSize,
	}, true
}

// CheckAlertConditions checks if alert conditions are met for a signature.
// Returns true if:
//   - count >= alertThreshold OR
//   - rate >= rateThresholdPerMin OR
//   - trend == "↑"
func (ec *EventCounter) CheckAlertConditions(signature string) bool {
	stats, exists := ec.GetStatistics(signature)
	if !exists {
		return false
	}

	// Check count threshold
	if stats.EventCount >= ec.alertThreshold {
		return true
	}

	// Check rate threshold
	if stats.RatePerMinute >= ec.rateThresholdPerMin {
		return true
	}

	// Check trend
	if stats.CountTrend == "↑" {
		return true
	}

	return false
}

// Cleanup removes all event groups with no occurrences in the window.
func (ec *EventCounter) Cleanup() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for signature, group := range ec.eventGroups {
		ec.cleanOldOccurrences(group, now)
		if len(group.Occurrences) == 0 {
			delete(ec.eventGroups, signature)
			cleaned++
		}
	}

	ec.stats.mu.Lock()
	ec.stats.EventsCleaned += int64(cleaned)
	ec.stats.mu.Unlock()
}

// Stats returns counter statistics.
func (ec *EventCounter) Stats() CounterStats {
	ec.stats.mu.RLock()
	defer ec.stats.mu.RUnlock()

	return CounterStats{
		TotalEventsRecorded: ec.stats.TotalEventsRecorded,
		UniqueEventGroups:   ec.stats.UniqueEventGroups,
		EventsCleaned:       ec.stats.EventsCleaned,
	}
}

// generateSignature generates a unique signature for an event.
// Uses FNV hash of key event characteristics.
func (ec *EventCounter) generateSignature(event *domain.LogEvent) string {
	h := fnv.New64a()

	// Hash event ID
	if event.EventID != nil {
		h.Write([]byte(*event.EventID))
	}

	// Hash category and product
	h.Write([]byte(string(event.Category)))
	h.Write([]byte(event.Product))

	// Hash key fields from raw data
	keyFields := []string{
		"process.name",
		"process.command_line",
		"process.parent.name",
		"destination.ip",
		"file.path",
		"registry.path",
	}

	for _, field := range keyFields {
		if val, ok := event.GetField(field); ok {
			h.Write([]byte(field))
			h.Write([]byte(fmt.Sprintf("%v", val)))
		}
	}

	return fmt.Sprintf("%x", h.Sum64())
}

// cleanOldOccurrences removes occurrences outside the time window.
func (ec *EventCounter) cleanOldOccurrences(group *eventGroup, now time.Time) {
	cutoff := now.Add(-ec.windowSize)

	// Binary search for cutoff point
	idx := sort.Search(len(group.Occurrences), func(i int) bool {
		return !group.Occurrences[i].Before(cutoff)
	})

	// Remove old occurrences
	if idx > 0 {
		group.Occurrences = group.Occurrences[idx:]
		group.Count = len(group.Occurrences)

		// Update FirstSeen if needed
		if len(group.Occurrences) > 0 {
			group.FirstSeen = group.Occurrences[0]
		}
	}
}

// calculateTrend calculates the trend of event occurrences.
// FIX ISSUE-10: Splits at the temporal midpoint instead of the count midpoint.
// Previously, splitting by array index could misrepresent the trend when events
// are clustered in time (e.g., 90% in the last 30s of a 5-min window).
// Now we split at the chronological midpoint for accurate trend assessment.
// Returns "↑" (uptrend), "↓" (downtrend), or "→" (stable).
func (ec *EventCounter) calculateTrend(occurrences []time.Time) string {
	if len(occurrences) < 2 {
		return "→"
	}

	// Calculate temporal midpoint (halfway between first and last event)
	first := occurrences[0]
	last := occurrences[len(occurrences)-1]
	timeMid := first.Add(last.Sub(first) / 2)

	// Split occurrences at temporal midpoint using binary search
	splitIdx := sort.Search(len(occurrences), func(i int) bool {
		return occurrences[i].After(timeMid)
	})

	// Need at least 2 events in each half for meaningful comparison
	if splitIdx < 2 || len(occurrences)-splitIdx < 2 {
		return "→"
	}

	firstHalf := occurrences[:splitIdx]
	secondHalf := occurrences[splitIdx:]

	// Calculate average interval for each half
	firstInterval := ec.avgInterval(firstHalf)
	secondInterval := ec.avgInterval(secondHalf)

	// Determine trend
	threshold := 0.2 // 20% change threshold
	if firstInterval == 0 {
		return "→"
	}

	change := (firstInterval - secondInterval) / firstInterval

	if change > threshold {
		return "↑" // Uptrend (intervals decreasing = more frequent)
	} else if change < -threshold {
		return "↓" // Downtrend (intervals increasing = less frequent)
	}

	return "→" // Stable
}

// avgInterval calculates the average interval between occurrences.
func (ec *EventCounter) avgInterval(occurrences []time.Time) float64 {
	if len(occurrences) < 2 {
		return 0
	}

	totalInterval := 0.0
	for i := 1; i < len(occurrences); i++ {
		interval := occurrences[i].Sub(occurrences[i-1]).Seconds()
		totalInterval += interval
	}

	return totalInterval / float64(len(occurrences)-1)
}

