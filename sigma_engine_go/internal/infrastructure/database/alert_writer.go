// Package database provides alert writer that bridges detection and storage.
package database

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// AlertWriterConfig configures the alert writer.
type AlertWriterConfig struct {
	DeduplicationWindow time.Duration `yaml:"deduplication_window"`
	BatchSize           int           `yaml:"batch_size"`
	FlushInterval       time.Duration `yaml:"flush_interval"`
	MaxQueueSize        int           `yaml:"max_queue_size"`
}

// DefaultAlertWriterConfig returns default configuration.
func DefaultAlertWriterConfig() AlertWriterConfig {
	return AlertWriterConfig{
		DeduplicationWindow: 5 * time.Minute,
		// Low-latency defaults so alerts show up near real-time in the dashboard.
		// Throughput is still protected by batching; the writer flushes at most every 100ms
		// unless BatchSize is hit first.
		BatchSize:           25,
		FlushInterval:       100 * time.Millisecond,
		MaxQueueSize:        10000,
	}
}

// AlertWriterMetrics tracks writer statistics.
type AlertWriterMetrics struct {
	AlertsWritten      uint64
	AlertsDeduplicated uint64
	AlertsDropped      uint64
	WriteErrors        uint64
	BatchesWritten     uint64
	AvgWriteLatencyMs  float64
	mu                 sync.RWMutex
}

// Snapshot returns a copy of metrics.
func (m *AlertWriterMetrics) Snapshot() AlertWriterMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return AlertWriterMetrics{
		AlertsWritten:      atomic.LoadUint64(&m.AlertsWritten),
		AlertsDeduplicated: atomic.LoadUint64(&m.AlertsDeduplicated),
		AlertsDropped:      atomic.LoadUint64(&m.AlertsDropped),
		WriteErrors:        atomic.LoadUint64(&m.WriteErrors),
		BatchesWritten:     atomic.LoadUint64(&m.BatchesWritten),
		AvgWriteLatencyMs:  m.AvgWriteLatencyMs,
	}
}

// AlertWriter writes alerts to PostgreSQL with deduplication.
type AlertWriter struct {
	repo    AlertRepository
	config  AlertWriterConfig
	metrics *AlertWriterMetrics
	// onAlertPersisted is an optional callback fired after a NEW alert is
	// successfully inserted into storage. It is used to fan out real-time
	// notifications (e.g. WebSocket broadcast) without coupling writer logic
	// to transport concerns.
	onAlertPersisted func(*Alert)

	alertChan chan *domain.Alert
	doneChan  chan struct{}

	running atomic.Bool
	wg      sync.WaitGroup
}

// NewAlertWriter creates a new alert writer.
func NewAlertWriter(repo AlertRepository, config AlertWriterConfig) *AlertWriter {
	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = 10000
	}

	return &AlertWriter{
		repo:      repo,
		config:    config,
		metrics:   &AlertWriterMetrics{},
		alertChan: make(chan *domain.Alert, config.MaxQueueSize),
		doneChan:  make(chan struct{}),
	}
}

// Start begins the background writer.
func (w *AlertWriter) Start(ctx context.Context) error {
	if w.running.Load() {
		return nil
	}
	w.running.Store(true)

	logger.Info("Starting alert writer...")

	w.wg.Add(1)
	go w.writeLoop(ctx)

	return nil
}

// writeLoop processes alerts in the background.
func (w *AlertWriter) writeLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*domain.Alert, 0, w.config.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		for _, alert := range batch {
			start := time.Now()
			if err := w.writeWithDedup(ctx, alert); err != nil {
				atomic.AddUint64(&w.metrics.WriteErrors, 1)
				logger.Errorf("❌ Failed to write alert to database: %v", err)
			} else {
				atomic.AddUint64(&w.metrics.AlertsWritten, 1)
			}

			latency := float64(time.Since(start).Milliseconds())
			w.metrics.mu.Lock()
			w.metrics.AvgWriteLatencyMs = w.metrics.AvgWriteLatencyMs*0.9 + latency*0.1
			w.metrics.mu.Unlock()
		}

		atomic.AddUint64(&w.metrics.BatchesWritten, 1)
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-w.doneChan:
			flush()
			return
		case alert, ok := <-w.alertChan:
			if !ok {
				flush()
				return
			}
			batch = append(batch, alert)
			if len(batch) >= w.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// writeWithDedup writes an alert with deduplication.
func (w *AlertWriter) writeWithDedup(ctx context.Context, domainAlert *domain.Alert) error {
	// Convert domain alert to database alert
	dbAlert := w.convertToDBAlert(domainAlert)

	// Check for recent similar alert (deduplication)
	since := time.Now().Add(-w.config.DeduplicationWindow)
	existing, err := w.repo.FindRecent(ctx, dbAlert.AgentID, dbAlert.RuleID, since)
	if err != nil {
		return err
	}

	if existing != nil {
		// Deduplicate: increment event count
		eventIDs := []string{dbAlert.EventIDs[0]} // Add new event
		if err := w.repo.IncrementEventCount(ctx, existing.ID, eventIDs); err != nil {
			return err
		}
		atomic.AddUint64(&w.metrics.AlertsDeduplicated, 1)
		return nil
	}

	// Create new alert
	created, err := w.repo.Create(ctx, dbAlert)
	if err != nil {
		return err
	}
	if w.onAlertPersisted != nil && created != nil {
		w.onAlertPersisted(created)
	}
	return err
}

// SetOnAlertPersisted registers an optional callback invoked for newly created
// alerts (not deduplicated updates).
func (w *AlertWriter) SetOnAlertPersisted(fn func(*Alert)) {
	w.onAlertPersisted = fn
}

// convertToDBAlert converts a domain Alert to database Alert.
func (w *AlertWriter) convertToDBAlert(da *domain.Alert) *Alert {
	// Extract MITRE tactics/techniques from alert
	tactics := make([]string, 0, len(da.MITRETactics))
	tactics = append(tactics, da.MITRETactics...)

	techniques := make([]string, 0, len(da.MITRETechniques))
	techniques = append(techniques, da.MITRETechniques...)

	// Generate event ID from EventID pointer
	eventID := ""
	if da.EventID != nil {
		eventID = *da.EventID
	}

	// Get original severity string
	origSeverity := ""
	if da.OriginalSeverity != 0 {
		origSeverity = da.OriginalSeverity.String()
	}

	// Extract agent_id from event data
	agentID := ""
	if da.EventData != nil {
		if aid, ok := da.EventData["agent_id"]; ok {
			if s, ok := aid.(string); ok {
				agentID = s
			}
		}
	}

	return &Alert{
		Timestamp:          da.Timestamp,
		AgentID:            agentID,
		RuleID:             da.RuleID,
		RuleTitle:          da.RuleTitle,
		Severity:           da.Severity.String(),
		Category:           string(da.EventCategory),
		EventCount:         1,
		EventIDs:           []string{eventID},
		MitreTactics:       tactics,
		MitreTechniques:    techniques,
		MatchedFields:      da.MatchedFields,
		MatchedSelections:  da.MatchedSelections,
		ContextData:        da.EventData,
		Status:             "open",
		Confidence:         &da.Confidence,
		FalsePositiveRisk:  &da.FalsePositiveRisk,
		MatchCount:         &da.MatchCount,
		RelatedRules:       da.RelatedRules,
		CombinedConfidence: &da.CombinedConfidence,
		SeverityPromoted:   &da.SeverityPromoted,
		OriginalSeverity:   origSeverity,
		// Context-Aware Risk Scoring fields (Phase 1)
		RiskScore:       da.RiskScore,
		ContextSnapshot: da.ContextSnapshot,
		ScoreBreakdown:  da.ScoreBreakdown,
	}
}

// Write queues an alert for writing.
func (w *AlertWriter) Write(alert *domain.Alert) error {
	if !w.running.Load() {
		return fmt.Errorf("alert writer is not running")
	}

	select {
	case w.alertChan <- alert:
		return nil
	default:
		atomic.AddUint64(&w.metrics.AlertsDropped, 1)
		return fmt.Errorf("alert writer queue full")
	}
}

// Metrics returns writer metrics.
func (w *AlertWriter) Metrics() AlertWriterMetrics {
	return w.metrics.Snapshot()
}

// Stop gracefully stops the writer.
func (w *AlertWriter) Stop() error {
	if !w.running.Load() {
		return nil
	}
	w.running.Store(false)

	logger.Info("Stopping alert writer...")
	close(w.doneChan)
	w.wg.Wait()

	logger.Info("Alert writer stopped")
	return nil
}

// IsRunning returns whether the writer is running.
func (w *AlertWriter) IsRunning() bool {
	return w.running.Load()
}
