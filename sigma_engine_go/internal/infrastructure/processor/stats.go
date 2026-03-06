package processor

import (
	"sync"
	"sync/atomic"
	"time"
)

// ProcessorStats tracks processor performance statistics.
type ProcessorStats struct {
	totalEvents      uint64
	successfulEvents uint64
	failedEvents     uint64
	totalDetections  uint64
	totalAlerts      uint64
	duplicateAlerts  uint64
	suppressedAlerts uint64

	// Panic tracking
	totalPanics    uint64
	panicsByWorker map[int]uint64

	startTime   time.Time
	lastUpdated time.Time

	totalLatency time.Duration
	minLatency   time.Duration
	maxLatency   time.Duration
	latencyCount uint64

	perWorkerStats map[int]*WorkerStats

	mu sync.RWMutex
}

// WorkerStats tracks statistics for a single worker.
type WorkerStats struct {
	WorkerID        int
	EventsProcessed uint64
	Detections      uint64
	Errors          uint64
	TotalTime       time.Duration
	AverageTime     time.Duration
	mu              sync.RWMutex
}

// ProcessorStatsSnapshot is a thread-safe snapshot of statistics.
type ProcessorStatsSnapshot struct {
	TotalEvents      uint64
	SuccessfulEvents uint64
	FailedEvents     uint64
	TotalDetections  uint64
	TotalAlerts      uint64
	DuplicateAlerts  uint64
	SuppressedAlerts uint64
	TotalPanics      uint64
	PanicsByWorker   map[int]uint64
	EventsPerSecond  float64
	AlertsPerSecond  float64
	SuccessRate      float64
	AverageLatency   time.Duration
	MinLatency       time.Duration
	MaxLatency       time.Duration
	Uptime           time.Duration
}

// NewProcessorStats creates a new statistics tracker.
func NewProcessorStats() *ProcessorStats {
	return &ProcessorStats{
		startTime:      time.Now(),
		lastUpdated:    time.Now(),
		minLatency:     time.Hour, // Initialize to high value
		perWorkerStats: make(map[int]*WorkerStats),
		panicsByWorker: make(map[int]uint64),
	}
}

// NewWorkerStats creates new worker statistics.
func NewWorkerStats(workerID int) *WorkerStats {
	return &WorkerStats{
		WorkerID: workerID,
	}
}

// RecordEvent records a processed event.
func (ps *ProcessorStats) RecordEvent(success bool, latency time.Duration) {
	atomic.AddUint64(&ps.totalEvents, 1)

	if success {
		atomic.AddUint64(&ps.successfulEvents, 1)
	} else {
		atomic.AddUint64(&ps.failedEvents, 1)
	}

	ps.updateLatency(latency)
	ps.mu.Lock()
	ps.lastUpdated = time.Now()
	ps.mu.Unlock()
}

// RecordDetection records detections and alerts.
func (ps *ProcessorStats) RecordDetection(detectionCount, alertCount int) {
	atomic.AddUint64(&ps.totalDetections, uint64(detectionCount))
	atomic.AddUint64(&ps.totalAlerts, uint64(alertCount))
}

// RecordDuplicate records a duplicate alert.
func (ps *ProcessorStats) RecordDuplicate() {
	atomic.AddUint64(&ps.duplicateAlerts, 1)
}

// RecordSuppressed records a suppressed alert.
func (ps *ProcessorStats) RecordSuppressed() {
	atomic.AddUint64(&ps.suppressedAlerts, 1)
}

// RecordPanic records a panic that occurred in a worker.
// This is used for monitoring worker health and detecting unstable rules.
func (ps *ProcessorStats) RecordPanic(workerID int) {
	atomic.AddUint64(&ps.totalPanics, 1)
	ps.mu.Lock()
	ps.panicsByWorker[workerID]++
	ps.mu.Unlock()
}

// updateLatency updates latency statistics.
func (ps *ProcessorStats) updateLatency(latency time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	atomic.AddUint64(&ps.latencyCount, 1)
	ps.totalLatency += latency

	if latency < ps.minLatency {
		ps.minLatency = latency
	}
	if latency > ps.maxLatency {
		ps.maxLatency = latency
	}
}

// MergeWorkerStats merges worker statistics.
func (ps *ProcessorStats) MergeWorkerStats(ws *WorkerStats) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.perWorkerStats[ws.WorkerID] = ws
}

// GetSnapshot returns a thread-safe snapshot of statistics.
func (ps *ProcessorStats) GetSnapshot() *ProcessorStatsSnapshot {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	totalEvents := atomic.LoadUint64(&ps.totalEvents)
	successfulEvents := atomic.LoadUint64(&ps.successfulEvents)
	failedEvents := atomic.LoadUint64(&ps.failedEvents)
	totalDetections := atomic.LoadUint64(&ps.totalDetections)
	totalAlerts := atomic.LoadUint64(&ps.totalAlerts)
	duplicateAlerts := atomic.LoadUint64(&ps.duplicateAlerts)
	suppressedAlerts := atomic.LoadUint64(&ps.suppressedAlerts)
	latencyCount := atomic.LoadUint64(&ps.latencyCount)

	elapsed := time.Since(ps.startTime).Seconds()
	if elapsed == 0 {
		elapsed = 1
	}

	eventsPerSecond := float64(totalEvents) / elapsed
	alertsPerSecond := float64(totalAlerts) / elapsed

	successRate := 0.0
	if totalEvents > 0 {
		successRate = float64(successfulEvents) / float64(totalEvents)
	}

	avgLatency := time.Duration(0)
	if latencyCount > 0 {
		avgLatency = ps.totalLatency / time.Duration(latencyCount)
	}

	totalPanics := atomic.LoadUint64(&ps.totalPanics)

	// Copy panics by worker map
	panicsByWorker := make(map[int]uint64)
	for k, v := range ps.panicsByWorker {
		panicsByWorker[k] = v
	}

	return &ProcessorStatsSnapshot{
		TotalEvents:      totalEvents,
		SuccessfulEvents: successfulEvents,
		FailedEvents:     failedEvents,
		TotalDetections:  totalDetections,
		TotalAlerts:      totalAlerts,
		DuplicateAlerts:  duplicateAlerts,
		SuppressedAlerts: suppressedAlerts,
		TotalPanics:      totalPanics,
		PanicsByWorker:   panicsByWorker,
		EventsPerSecond:  eventsPerSecond,
		AlertsPerSecond:  alertsPerSecond,
		SuccessRate:      successRate,
		AverageLatency:   avgLatency,
		MinLatency:       ps.minLatency,
		MaxLatency:       ps.maxLatency,
		Uptime:           time.Since(ps.startTime),
	}
}

// GetInFlightCount returns the approximate number of events in flight.
func (ps *ProcessorStats) GetInFlightCount() int {
	totalEvents := atomic.LoadUint64(&ps.totalEvents)
	successfulEvents := atomic.LoadUint64(&ps.successfulEvents)
	failedEvents := atomic.LoadUint64(&ps.failedEvents)
	return int(totalEvents - successfulEvents - failedEvents)
}

// RecordSuccess records a successful event processing.
func (ws *WorkerStats) RecordSuccess(duration time.Duration) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.EventsProcessed++
	ws.TotalTime += duration
	ws.AverageTime = ws.TotalTime / time.Duration(ws.EventsProcessed)
}

// RecordError records an error during event processing.
func (ws *WorkerStats) RecordError(duration time.Duration) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.EventsProcessed++
	ws.Errors++
	ws.TotalTime += duration
	if ws.EventsProcessed > 0 {
		ws.AverageTime = ws.TotalTime / time.Duration(ws.EventsProcessed)
	}
}
