package detection

import (
	"sync"
	"sync/atomic"
	"time"
)

// DetectionStats tracks detection engine statistics.
// Thread-safe using atomic operations for counters.
type DetectionStats struct {
	totalEvents         uint64
	totalDetections     uint64
	totalRuleEvals      uint64
	candidateRules      uint64
	totalProcessingTime time.Duration

	ruleMatchCounts map[string]uint64
	mu              sync.RWMutex
}

// DetectionStatsSnapshot is a thread-safe snapshot of statistics.
type DetectionStatsSnapshot struct {
	TotalEvents       uint64
	TotalDetections   uint64
	TotalRuleEvals    uint64
	CandidateRules    uint64
	AvgProcessingTime time.Duration
	RuleMatchCounts   map[string]uint64
	DetectionRate     float64
}

// NewDetectionStats creates a new statistics tracker.
func NewDetectionStats() *DetectionStats {
	return &DetectionStats{
		ruleMatchCounts: make(map[string]uint64),
	}
}

// RecordEvent records a processed event.
func (ds *DetectionStats) RecordEvent() {
	atomic.AddUint64(&ds.totalEvents, 1)
}

// RecordDetection records a detection (match).
func (ds *DetectionStats) RecordDetection(matched bool) {
	if matched {
		atomic.AddUint64(&ds.totalDetections, 1)
	}
}

// RecordRuleEvaluation records a rule evaluation.
func (ds *DetectionStats) RecordRuleEvaluation(ruleID string, matched bool) {
	atomic.AddUint64(&ds.totalRuleEvals, 1)

	if matched {
		ds.mu.Lock()
		ds.ruleMatchCounts[ruleID]++
		ds.mu.Unlock()
	}
}

// RecordCandidateCount records the number of candidate rules for an event.
func (ds *DetectionStats) RecordCandidateCount(count int) {
	atomic.AddUint64(&ds.candidateRules, uint64(count))
}

// RecordProcessingTime records processing time for an event.
func (ds *DetectionStats) RecordProcessingTime(duration time.Duration) {
	ds.mu.Lock()
	ds.totalProcessingTime += duration
	ds.mu.Unlock()
}

// Snapshot returns a thread-safe snapshot of current statistics.
func (ds *DetectionStats) Snapshot() *DetectionStatsSnapshot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	totalEvents := atomic.LoadUint64(&ds.totalEvents)
	totalDetections := atomic.LoadUint64(&ds.totalDetections)
	totalRuleEvals := atomic.LoadUint64(&ds.totalRuleEvals)
	candidateRules := atomic.LoadUint64(&ds.candidateRules)

	avgTime := time.Duration(0)
	if totalEvents > 0 {
		avgTime = ds.totalProcessingTime / time.Duration(totalEvents)
	}

	detectionRate := 0.0
	if totalEvents > 0 {
		detectionRate = float64(totalDetections) / float64(totalEvents)
	}

	// Copy rule match counts
	ruleCounts := make(map[string]uint64)
	for k, v := range ds.ruleMatchCounts {
		ruleCounts[k] = v
	}

	return &DetectionStatsSnapshot{
		TotalEvents:       totalEvents,
		TotalDetections:   totalDetections,
		TotalRuleEvals:    totalRuleEvals,
		CandidateRules:    candidateRules,
		AvgProcessingTime: avgTime,
		RuleMatchCounts:   ruleCounts,
		DetectionRate:     detectionRate,
	}
}

// Reset clears all statistics.
func (ds *DetectionStats) Reset() {
	atomic.StoreUint64(&ds.totalEvents, 0)
	atomic.StoreUint64(&ds.totalDetections, 0)
	atomic.StoreUint64(&ds.totalRuleEvals, 0)
	atomic.StoreUint64(&ds.candidateRules, 0)

	ds.mu.Lock()
	ds.totalProcessingTime = 0
	ds.ruleMatchCounts = make(map[string]uint64)
	ds.mu.Unlock()
}

// TotalEvents returns the current total event count (atomic).
func (ds *DetectionStats) TotalEvents() uint64 {
	return atomic.LoadUint64(&ds.totalEvents)
}
