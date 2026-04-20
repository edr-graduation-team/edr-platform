package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/application/alert"
	"github.com/edr-platform/sigma-engine/internal/application/detection"
	"github.com/edr-platform/sigma-engine/internal/analytics"
	"github.com/edr-platform/sigma-engine/internal/automation"
	"github.com/edr-platform/sigma-engine/internal/application/scoring"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/output"
)

// ProcessorConfig configures the parallel event processor.
type ProcessorConfig struct {
	NumWorkers      int
	BatchSize       int
	ChannelBuffers  int
	WorkerTimeout   time.Duration
	MetricsInterval time.Duration
}

// DefaultConfig returns default processor configuration.
func DefaultConfig() ProcessorConfig {
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 4
	}

	return ProcessorConfig{
		NumWorkers:      numWorkers,
		BatchSize:       50,
		ChannelBuffers:  1000,
		WorkerTimeout:   30 * time.Second,
		MetricsInterval: time.Second,
	}
}

// ParallelEventProcessor orchestrates parallel event processing with worker pools.
type ParallelEventProcessor struct {
	config          ProcessorConfig
	detectionEngine *detection.SigmaDetectionEngine
	alertGenerator  *alert.AlertGenerator
	deduplicator    *alert.Deduplicator
	outputManager   *output.OutputManager

	// riskScorer enriches aggregated alerts when set (parity with kafka.EventLoop).
	// Call SetRiskScorer before Start(); nil skips scoring.
	riskScorer scoring.RiskScorer

	// correlator records edges when set (parity with kafka.EventLoop). Nil skips.
	correlator *analytics.CorrelationManager

	playbooks   *automation.PlaybookManager
	escalations *automation.EscalationManager

	// Channels
	eventChan  chan *domain.LogEvent
	resultChan chan *ProcessingResult
	errorChan  chan ProcessingError

	// Statistics
	stats *ProcessorStats

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// ProcessingResult represents the result of processing an event.
type ProcessingResult struct {
	Event      *domain.LogEvent
	Detections []*domain.DetectionResult // Deprecated: kept for compatibility
	Alerts     []*domain.Alert
	Success    bool
	Error      error
	Duration   time.Duration
}

// ProcessingError represents an error during event processing.
type ProcessingError struct {
	Event *domain.LogEvent
	Error error
	Stage string
	Time  time.Time
}

// BatchProcessingResult represents aggregated results from batch processing.
type BatchProcessingResult struct {
	Results             []*ProcessingResult
	Errors              []ProcessingError
	EventsQueued        int
	TotalAlerts         int
	ErrorCount          int
	StartTime           time.Time
	Duration            time.Duration
	ThroughputPerSecond float64
}

// NewParallelEventProcessor creates a new parallel event processor.
func NewParallelEventProcessor(
	detectionEngine *detection.SigmaDetectionEngine,
	alertGenerator *alert.AlertGenerator,
	deduplicator *alert.Deduplicator,
	outputManager *output.OutputManager,
	config ProcessorConfig,
) *ParallelEventProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	return &ParallelEventProcessor{
		config:          config,
		detectionEngine: detectionEngine,
		alertGenerator:  alertGenerator,
		deduplicator:    deduplicator,
		outputManager:   outputManager,
		eventChan:       make(chan *domain.LogEvent, config.ChannelBuffers),
		resultChan:      make(chan *ProcessingResult, config.ChannelBuffers),
		errorChan:       make(chan ProcessingError, config.ChannelBuffers),
		stats:           NewProcessorStats(),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the worker pool.
func (p *ParallelEventProcessor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Start workers
	for i := 0; i < p.config.NumWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	logger.Infof("Started parallel event processor with %d workers", p.config.NumWorkers)
	return nil
}

// SetRiskScorer injects context-aware scoring (optional). Set before Start().
func (p *ParallelEventProcessor) SetRiskScorer(rs scoring.RiskScorer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.riskScorer = rs
}

// SetCorrelationManager injects the same CorrelationManager used by the Kafka EventLoop when both paths run.
func (p *ParallelEventProcessor) SetCorrelationManager(m *analytics.CorrelationManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.correlator = m
}

// SetPlaybookManager injects playbook automation (optional). Set before Start().
func (p *ParallelEventProcessor) SetPlaybookManager(m *automation.PlaybookManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playbooks = m
}

// SetEscalationManager injects escalation tracking (optional). Set before Start().
func (p *ParallelEventProcessor) SetEscalationManager(m *automation.EscalationManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.escalations = m
}

// ProcessEvent processes a single event.
func (p *ParallelEventProcessor) ProcessEvent(ctx context.Context, event *domain.LogEvent) (*ProcessingResult, error) {
	select {
	case p.eventChan <- event:
		// Wait for result
		select {
		case result := <-p.resultChan:
			return result, nil
		case err := <-p.errorChan:
			return nil, err.Error
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ProcessBatch processes a batch of events.
func (p *ParallelEventProcessor) ProcessBatch(
	ctx context.Context,
	events []*domain.LogEvent,
) *BatchProcessingResult {
	result := &BatchProcessingResult{
		StartTime: time.Now(),
		Results:   make([]*ProcessingResult, 0, len(events)),
		Errors:    make([]ProcessingError, 0),
	}

	// Send events to processing channel
	go func() {
		for _, event := range events {
			select {
			case p.eventChan <- event:
				result.EventsQueued++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	alertCount := 0
	errorCount := 0

	for i := 0; i < len(events); {
		select {
		case res := <-p.resultChan:
			result.Results = append(result.Results, res)
			alertCount += len(res.Alerts)
			if res.Error != nil {
				errorCount++
			}
			i++

		case err := <-p.errorChan:
			result.Errors = append(result.Errors, err)
			errorCount++
			i++

		case <-ctx.Done():
			result.Duration = time.Since(result.StartTime)
			return result
		}
	}

	result.Duration = time.Since(result.StartTime)
	result.TotalAlerts = alertCount
	result.ErrorCount = errorCount
	if result.Duration > 0 {
		result.ThroughputPerSecond = float64(len(events)) / result.Duration.Seconds()
	}

	return result
}

// ProcessStream processes events from a stream.
func (p *ParallelEventProcessor) ProcessStream(
	ctx context.Context,
	eventSource <-chan *domain.LogEvent,
) <-chan *ProcessingResult {
	resultChan := make(chan *ProcessingResult, p.config.BatchSize)

	go func() {
		defer close(resultChan)

		for {
			select {
			case event, ok := <-eventSource:
				if !ok {
					return // Source closed
				}

				// Send to processing
				select {
				case p.eventChan <- event:
				case <-ctx.Done():
					return
				}

				// Collect result
				select {
				case res := <-p.resultChan:
					resultChan <- res
				case err := <-p.errorChan:
					resultChan <- &ProcessingResult{
						Event:   event,
						Success: false,
						Error:   err.Error,
					}
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return resultChan
}

// worker processes events from the work channel.
// Includes panic recovery to prevent worker crashes from taking down the pool.
func (p *ParallelEventProcessor) worker(workerID int) {
	defer p.wg.Done()

	// Top-level panic recovery for the entire worker
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Worker %d fatal panic recovered: %v\nStack:\n%s", workerID, r, debug.Stack())
			p.stats.RecordPanic(workerID)
		}
	}()

	localStats := NewWorkerStats(workerID)

	for {
		select {
		case event, ok := <-p.eventChan:
			if !ok {
				// Channel closed, shutdown
				p.stats.MergeWorkerStats(localStats)
				return
			}

			// Process event with panic protection
			start := time.Now()
			result := p.safeProcessEvent(event, workerID)
			duration := time.Since(start)

			// Update statistics
			if result.Success {
				localStats.RecordSuccess(duration)
				p.stats.RecordEvent(true, duration)
				// Count detections as 1 per event (aggregated), but track alert count
				p.stats.RecordDetection(1, len(result.Alerts))
			} else {
				localStats.RecordError(duration)
				p.stats.RecordEvent(false, duration)
			}

			// Send result
			select {
			case p.resultChan <- result:
			case <-p.ctx.Done():
				p.stats.MergeWorkerStats(localStats)
				return
			}

		case <-p.ctx.Done():
			p.stats.MergeWorkerStats(localStats)
			return
		}
	}
}

// safeProcessEvent wraps processEvent with panic recovery.
// If a panic occurs during event processing, it is caught and returned as an error result.
// This ensures the worker continues processing subsequent events.
func (p *ParallelEventProcessor) safeProcessEvent(event *domain.LogEvent, workerID int) (result *ProcessingResult) {
	start := time.Now()

	// Panic recovery for individual event processing
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic in processEvent (worker %d): %v\nStack:\n%s", workerID, r, debug.Stack())
			p.stats.RecordPanic(workerID)
			result = &ProcessingResult{
				Event:    event,
				Success:  false,
				Error:    fmt.Errorf("panic recovered: %v", r),
				Duration: time.Since(start),
			}
		}
	}()

	return p.processEvent(event)
}

// processEvent processes a single event through the detection pipeline.
func (p *ParallelEventProcessor) processEvent(event *domain.LogEvent) *ProcessingResult {
	start := time.Now()

	// Step 1: Validate event
	if event == nil {
		return &ProcessingResult{
			Event:    event,
			Success:  false,
			Error:    fmt.Errorf("event is nil"),
			Duration: time.Since(start),
		}
	}

	// Step 2: Run aggregated detection (collects ALL matches for this event)
	matchResult := p.detectionEngine.DetectAggregated(event)

	// Step 3: Generate ONE aggregated alert from ALL matches
	var alerts []*domain.Alert
	if matchResult.HasMatches() {
		aggAlert := p.alertGenerator.GenerateAggregatedAlert(matchResult)
		if aggAlert != nil {
			p.mu.RLock()
			rs := p.riskScorer
			co := p.correlator
			p.mu.RUnlock()
			if rs != nil {
				agentStr := ""
				if agentID, _ := event.GetField("agent_id"); agentID != nil {
					agentStr, _ = agentID.(string)
				}
				scoringInput := scoring.ScoringInput{
					MatchResult: matchResult,
					Event:       event,
					AgentID:     agentStr,
				}
				scoreOut, scoreErr := rs.Score(context.Background(), scoringInput)
				if scoreErr != nil {
					logger.Warnf("RiskScorer error (parallel processor) rule %s: %v", aggAlert.RuleID, scoreErr)
				} else {
					aggAlert.RiskScore = scoreOut.RiskScore
					aggAlert.FalsePositiveRisk = scoreOut.FalsePositiveRisk
					if snap := scoreOut.Snapshot; snap != nil {
						importJSON, _ := json.Marshal(snap)
						_ = json.Unmarshal(importJSON, &aggAlert.ContextSnapshot)
						bdJSON, _ := json.Marshal(snap.ScoreBreakdown)
						_ = json.Unmarshal(bdJSON, &aggAlert.ScoreBreakdown)
					}
				}
			}
			if co != nil {
				co.CorrelateAlert(aggAlert)
			}
			alerts = append(alerts, aggAlert)
		}
	}

	// Step 4: Deduplicate
	alerts = p.deduplicator.Deduplicate(alerts)

	// Step 5: Write to output
	for _, alert := range alerts {
		if !alert.ShouldSuppress() {
			if err := p.outputManager.WriteAlert(alert); err != nil {
				logger.Warnf("Failed to write alert %s: %v", alert.ID, err)
			} else {
				p.mu.RLock()
				es := p.escalations
				pb := p.playbooks
				p.mu.RUnlock()
				if es != nil {
					es.TrackAlert(alert)
				}
				if pb != nil {
					pb.ExecuteForAlert(context.Background(), alert)
				}
			}
		}
	}

	return &ProcessingResult{
		Event:      event,
		Detections: nil, // Deprecated: using aggregated alerts now
		Alerts:     alerts,
		Success:    true,
		Duration:   time.Since(start),
	}
}

// Shutdown gracefully shuts down the processor.
func (p *ParallelEventProcessor) Shutdown(ctx context.Context) error {
	// Signal workers to stop accepting new events
	close(p.eventChan)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All workers finished
		p.cancel()
		logger.Info("Parallel event processor shutdown complete")
		return nil
	case <-ctx.Done():
		// Timeout exceeded
		p.cancel()
		return fmt.Errorf("shutdown timeout: %d events in flight", p.stats.GetInFlightCount())
	}
}

// Stats returns processor statistics.
func (p *ParallelEventProcessor) Stats() *ProcessorStatsSnapshot {
	return p.stats.GetSnapshot()
}
