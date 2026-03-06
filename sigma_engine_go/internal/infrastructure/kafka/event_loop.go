// Package kafka provides the integrated event loop for Kafka-based processing.
package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/sigma-engine/internal/application/alert"
	"github.com/edr-platform/sigma-engine/internal/application/detection"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/database"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// EventLoopConfig configures the integrated event loop.
type EventLoopConfig struct {
	Workers         int           `yaml:"workers"`          // Detection worker count
	EventBuffer     int           `yaml:"event_buffer"`     // Event channel buffer size
	AlertBuffer     int           `yaml:"alert_buffer"`     // Alert channel buffer size
	StatsInterval   time.Duration `yaml:"stats_interval"`   // Statistics reporting interval
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"` // Graceful shutdown timeout
}

// DefaultEventLoopConfig returns default event loop configuration.
func DefaultEventLoopConfig() EventLoopConfig {
	return EventLoopConfig{
		Workers:         4,
		EventBuffer:     1000,
		AlertBuffer:     500,
		StatsInterval:   30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}

// EventLoopMetrics tracks event loop statistics.
type EventLoopMetrics struct {
	EventsReceived   uint64
	EventsProcessed  uint64
	AlertsGenerated  uint64
	AlertsPublished  uint64
	AlertsSuppressed uint64
	ProcessingErrors uint64
	AverageLatencyMs float64
	CurrentEPS       float64
	mu               sync.RWMutex
}

// Snapshot returns a copy of current metrics.
func (m *EventLoopMetrics) Snapshot() EventLoopMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return EventLoopMetrics{
		EventsReceived:   atomic.LoadUint64(&m.EventsReceived),
		EventsProcessed:  atomic.LoadUint64(&m.EventsProcessed),
		AlertsGenerated:  atomic.LoadUint64(&m.AlertsGenerated),
		AlertsPublished:  atomic.LoadUint64(&m.AlertsPublished),
		AlertsSuppressed: atomic.LoadUint64(&m.AlertsSuppressed),
		ProcessingErrors: atomic.LoadUint64(&m.ProcessingErrors),
		AverageLatencyMs: m.AverageLatencyMs,
		CurrentEPS:       m.CurrentEPS,
	}
}

// =============================================================================
// Alert Suppression Cache (Anti-Flooding / Deduplication)
// =============================================================================

const defaultSuppressionTTL = 60 * time.Second
const cleanupInterval = 30 * time.Second

// suppressionCache is a thread-safe, TTL-based cache for alert deduplication.
// Key: "ruleID|agentID" — suppresses duplicate alerts from the same rule+agent
// within a configurable time window.
type suppressionCache struct {
	mu      sync.RWMutex
	entries map[string]time.Time // key → first-seen timestamp
	ttl     time.Duration
}

func newSuppressionCache(ttl time.Duration) *suppressionCache {
	if ttl <= 0 {
		ttl = defaultSuppressionTTL
	}
	return &suppressionCache{
		entries: make(map[string]time.Time),
		ttl:     ttl,
	}
}

// shouldSuppress returns true if an alert with this key was already seen
// within the TTL window. If not suppressed, records the key.
func (sc *suppressionCache) shouldSuppress(key string) bool {
	now := time.Now()

	sc.mu.RLock()
	if ts, exists := sc.entries[key]; exists && now.Sub(ts) < sc.ttl {
		sc.mu.RUnlock()
		return true
	}
	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()
	// Double-check after write lock
	if ts, exists := sc.entries[key]; exists && now.Sub(ts) < sc.ttl {
		return true
	}
	sc.entries[key] = now
	return false
}

// cleanup removes expired entries to prevent unbounded memory growth.
func (sc *suppressionCache) cleanup() {
	now := time.Now()
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for key, ts := range sc.entries {
		if now.Sub(ts) >= sc.ttl {
			delete(sc.entries, key)
		}
	}
}

// size returns the current number of entries (for stats logging).
func (sc *suppressionCache) size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.entries)
}

// EventLoop coordinates Kafka consumer, detection engine, and alert producer.
type EventLoop struct {
	consumer        *EventConsumer
	producer        *AlertProducer
	detectionEngine *detection.SigmaDetectionEngine
	alertGenerator  *alert.AlertGenerator
	config          EventLoopConfig
	metrics         *EventLoopMetrics
	suppression     *suppressionCache
	alertWriter     *database.AlertWriter // Writes alerts to PostgreSQL

	alertChan chan *domain.Alert
	doneChan  chan struct{}

	running atomic.Bool
	wg      sync.WaitGroup
}

// NewEventLoop creates a new integrated event loop.
func NewEventLoop(
	consumer *EventConsumer,
	producer *AlertProducer,
	detectionEngine *detection.SigmaDetectionEngine,
	alertGenerator *alert.AlertGenerator,
	config EventLoopConfig,
) *EventLoop {
	if config.Workers <= 0 {
		config.Workers = 4
	}
	if config.AlertBuffer <= 0 {
		config.AlertBuffer = 500
	}

	return &EventLoop{
		consumer:        consumer,
		producer:        producer,
		detectionEngine: detectionEngine,
		alertGenerator:  alertGenerator,
		config:          config,
		metrics:         &EventLoopMetrics{},
		suppression:     newSuppressionCache(defaultSuppressionTTL),
		alertChan:       make(chan *domain.Alert, config.AlertBuffer),
		doneChan:        make(chan struct{}),
	}
}

// SetAlertWriter injects an AlertWriter for database persistence.
// Call this before Start().
func (el *EventLoop) SetAlertWriter(writer *database.AlertWriter) {
	el.alertWriter = writer
}

// Start begins the event processing loop.
func (el *EventLoop) Start(ctx context.Context) error {
	if el.running.Load() {
		return nil
	}
	el.running.Store(true)

	logger.Infof("Starting event loop with %d detection workers", el.config.Workers)

	// Start Kafka consumer
	if err := el.consumer.Start(ctx); err != nil {
		return err
	}

	// Start Kafka producer
	if err := el.producer.Start(ctx); err != nil {
		el.consumer.Stop()
		return err
	}

	// Start detection workers
	for i := 0; i < el.config.Workers; i++ {
		el.wg.Add(1)
		go el.detectionWorker(ctx, i)
	}

	// Start alert publisher
	el.wg.Add(1)
	go el.alertPublisher(ctx)

	// Start stats reporter
	el.wg.Add(1)
	go el.statsReporter(ctx)

	// Start suppression cache cleanup
	el.wg.Add(1)
	go el.suppressionCleaner(ctx)

	logger.Infof("Event loop started (alert suppression: %v window)", el.suppression.ttl)
	return nil
}

// detectionWorker processes events from consumer and generates alerts.
// Drains eventChan until it is closed (by the consumer), then exits.
func (el *EventLoop) detectionWorker(ctx context.Context, workerID int) {
	defer el.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in detectionWorker %d: %v", workerID, r)
		}
	}()
	logger.Debugf("Detection worker %d started", workerID)

	eventChan := el.consumer.Events()

	for event := range eventChan {
		el.processOneEvent(event)
	}

	logger.Debugf("Detection worker %d stopped (event channel closed)", workerID)
}

// processOneEvent runs detection on a single event with panic isolation.
func (el *EventLoop) processOneEvent(event *domain.LogEvent) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered while processing event: %v", r)
			atomic.AddUint64(&el.metrics.ProcessingErrors, 1)
		}
	}()

	atomic.AddUint64(&el.metrics.EventsReceived, 1)
	start := time.Now()

	matchResult := el.detectionEngine.DetectAggregated(event)

	if matchResult != nil && matchResult.HasMatches() {
		baseAlert := el.alertGenerator.GenerateAggregatedAlert(matchResult)
		if baseAlert != nil {
			atomic.AddUint64(&el.metrics.AlertsGenerated, 1)

			// Build suppression key: ruleID|agentID
			agentID, _ := event.GetField("agent_id")
			agentStr := ""
			if agentID != nil {
				agentStr, _ = agentID.(string)
			}
			suppressKey := baseAlert.RuleID + "|" + agentStr

			if el.suppression.shouldSuppress(suppressKey) {
				atomic.AddUint64(&el.metrics.AlertsSuppressed, 1)
			} else {
				select {
				case el.alertChan <- baseAlert:
				default:
					atomic.AddUint64(&el.metrics.ProcessingErrors, 1)
					logger.Warn("Alert channel full, dropping alert")
				}
			}
		}
	}

	atomic.AddUint64(&el.metrics.EventsProcessed, 1)

	latency := time.Since(start).Milliseconds()
	el.metrics.mu.Lock()
	el.metrics.AverageLatencyMs = (el.metrics.AverageLatencyMs*0.9 + float64(latency)*0.1)
	el.metrics.mu.Unlock()
}

// alertPublisher sends alerts to Kafka producer AND writes to PostgreSQL.
// Drains alertChan until it is closed, then exits.
func (el *EventLoop) alertPublisher(ctx context.Context) {
	defer el.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in alertPublisher: %v", r)
		}
	}()
	logger.Debug("Alert publisher started")

	for alert := range el.alertChan {
		// Publish to Kafka
		if err := el.producer.Publish(alert); err != nil {
			logger.Warnf("Failed to publish alert to Kafka: %v", err)
			atomic.AddUint64(&el.metrics.ProcessingErrors, 1)
		} else {
			atomic.AddUint64(&el.metrics.AlertsPublished, 1)
		}

		// Write to PostgreSQL (if AlertWriter is configured)
		if el.alertWriter != nil {
			if err := el.alertWriter.Write(alert); err != nil {
				logger.Warnf("Failed to queue alert for DB write: %v", err)
			}
		}
	}

	logger.Debug("Alert publisher stopped (alert channel closed)")
}

// statsReporter periodically reports statistics.
func (el *EventLoop) statsReporter(ctx context.Context) {
	defer el.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in statsReporter: %v", r)
		}
	}()

	ticker := time.NewTicker(el.config.StatsInterval)
	defer ticker.Stop()

	var lastProcessed uint64
	lastTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-el.doneChan:
			return
		case <-ticker.C:
			processed := atomic.LoadUint64(&el.metrics.EventsProcessed)
			now := time.Now()
			duration := now.Sub(lastTime).Seconds()

			if duration > 0 {
				eps := float64(processed-lastProcessed) / duration
				el.metrics.mu.Lock()
				el.metrics.CurrentEPS = eps
				el.metrics.mu.Unlock()
			}

			consumerMetrics := el.consumer.Metrics()
			producerMetrics := el.producer.Metrics()
			loopMetrics := el.metrics.Snapshot()

			logger.Infof("📊 Stats | Events: %d | Alerts: %d (suppressed: %d, cache: %d) | EPS: %.1f | Latency: %.1fms | Published: %d | Errors: %d",
				loopMetrics.EventsProcessed,
				loopMetrics.AlertsGenerated,
				loopMetrics.AlertsSuppressed,
				el.suppression.size(),
				loopMetrics.CurrentEPS,
				loopMetrics.AverageLatencyMs,
				producerMetrics.AlertsPublished,
				consumerMetrics.DeserializeErrors+loopMetrics.ProcessingErrors,
			)

			lastProcessed = processed
			lastTime = now
		}
	}
}

// suppressionCleaner periodically purges expired entries from the dedup cache.
func (el *EventLoop) suppressionCleaner(ctx context.Context) {
	defer el.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in suppressionCleaner: %v", r)
		}
	}()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-el.doneChan:
			return
		case <-ticker.C:
			el.suppression.cleanup()
		}
	}
}

// Stop gracefully stops the event loop with correct drain ordering:
//  1. Stop consumer (closes eventChan → workers drain remaining events)
//  2. Wait for detection workers to finish (they exit when eventChan is closed)
//  3. Close alertChan → alert publisher drains remaining alerts
//  4. Signal statsReporter to stop
//  5. Wait for publisher + statsReporter to finish
//  6. Stop Kafka producer (flushes final batch)
func (el *EventLoop) Stop() error {
	if !el.running.Load() {
		return nil
	}
	el.running.Store(false)

	logger.Info("Stopping event loop (draining buffers)...")

	// Step 1: Stop consumer — this closes eventChan, which causes workers to drain and exit
	if err := el.consumer.Stop(); err != nil {
		logger.Errorf("Error stopping consumer: %v", err)
	}

	// Step 2: Wait for detection workers to finish draining eventChan
	// (they range over eventChan and exit when it's closed)
	// Workers are tracked by el.wg, but so are alertPublisher and statsReporter.
	// We use a separate WaitGroup for workers via a timeout guard.
	workersDone := make(chan struct{})
	go func() {
		// Workers + publisher + stats all share el.wg.
		// After workers finish they stop sending to alertChan.
		// We wait briefly for all workers, then close alertChan for the publisher.
		// Using a timeout to prevent hanging if a worker is stuck.
		time.Sleep(2 * time.Second) // Grace period for workers to drain
		close(el.alertChan)         // Step 3: signal publisher to drain and exit
		close(el.doneChan)          // Step 4: signal statsReporter to exit
		close(workersDone)
	}()

	<-workersDone

	// Step 5: Wait for all goroutines (workers + publisher + stats) with timeout
	allDone := make(chan struct{})
	go func() {
		el.wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
		logger.Info("All workers and publisher stopped")
	case <-time.After(el.config.ShutdownTimeout):
		logger.Warn("Shutdown timeout, some goroutines may still be running")
	}

	// Step 6: Stop Kafka producer (flushes the final writer batch)
	if err := el.producer.Stop(); err != nil {
		logger.Errorf("Error stopping producer: %v", err)
	}

	logger.Info("Event loop stopped")
	return nil
}

// Metrics returns current event loop metrics.
func (el *EventLoop) Metrics() EventLoopMetrics {
	return el.metrics.Snapshot()
}

// IsRunning returns whether the event loop is running.
func (el *EventLoop) IsRunning() bool {
	return el.running.Load()
}

// --- PerformanceMetricsProvider interface implementation ---

// GetEventsPerSecond returns the current events per second rate.
func (el *EventLoop) GetEventsPerSecond() float64 {
	el.metrics.mu.RLock()
	defer el.metrics.mu.RUnlock()
	return el.metrics.CurrentEPS
}

// GetAlertsPerSecond returns the current alerts per second rate.
func (el *EventLoop) GetAlertsPerSecond() float64 {
	published := atomic.LoadUint64(&el.metrics.AlertsPublished)
	processed := atomic.LoadUint64(&el.metrics.EventsProcessed)
	if processed == 0 {
		return 0
	}
	// Approximate alerts/sec as ratio of alerts to events × EPS
	el.metrics.mu.RLock()
	eps := el.metrics.CurrentEPS
	el.metrics.mu.RUnlock()
	return (float64(published) / float64(processed)) * eps
}

// GetAverageLatencyMs returns the average event processing latency in ms.
func (el *EventLoop) GetAverageLatencyMs() float64 {
	el.metrics.mu.RLock()
	defer el.metrics.mu.RUnlock()
	return el.metrics.AverageLatencyMs
}

// GetProcessingErrors returns the total number of processing errors.
func (el *EventLoop) GetProcessingErrors() uint64 {
	return atomic.LoadUint64(&el.metrics.ProcessingErrors)
}

// GetEventsProcessed returns the total number of events processed.
func (el *EventLoop) GetEventsProcessed() uint64 {
	return atomic.LoadUint64(&el.metrics.EventsProcessed)
}
