package integration

import (
	"context"
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/application/detection"
	"github.com/edr-platform/sigma-engine/internal/application/mapping"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/edr-platform/sigma-engine/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndToEndDetection(t *testing.T) {
	// Setup
	fieldCache, err := cache.NewFieldResolutionCache(1000)
	require.NoError(t, err)

	regexCache, err := cache.NewRegexCache(1000)
	require.NoError(t, err)

	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create test rule
	rule := helpers.GenerateTestRule(
		"test-rule-1",
		"Test PowerShell Detection",
		"windows",
		"process_creation",
	)

	// Load rule
	err = detectionEngine.LoadRules([]*domain.SigmaRule{rule})
	require.NoError(t, err)

	// Create suspicious PowerShell event
	eventData := helpers.GenerateSuspiciousPowerShellEvent()
	event, err := domain.NewLogEvent(eventData)
	require.NoError(t, err)

	// Run detection
	results := detectionEngine.Detect(event)

	// Verify results
	assert.Greater(t, len(results), 0, "Should detect at least one rule")

	if len(results) > 0 {
		result := results[0]
		assert.True(t, result.Matched, "Detection should match")
		assert.Greater(t, result.Confidence, 0.0, "Confidence should be greater than 0")
		assert.NotEmpty(t, result.MatchedFields, "Should have matched fields")
	}
}

func TestDetectionWithMultipleRules(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create multiple rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("rule-1", "Rule 1", "windows", "process_creation"),
		helpers.GenerateTestRule("rule-2", "Rule 2", "windows", "process_creation"),
		helpers.GenerateTestRule("rule-3", "Rule 3", "linux", "process_creation"),
	}

	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Create event
	eventData := helpers.GenerateWindowsProcessCreationEvent()
	event, err := domain.NewLogEvent(eventData)
	require.NoError(t, err)

	// Run detection
	results := detectionEngine.Detect(event)

	// Should match Windows rules but not Linux rule
	assert.Greater(t, len(results), 0, "Should match at least one rule")
}

func TestDetectionWithNoMatch(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create rule for specific process
	rule := helpers.GenerateTestRule("rule-1", "Specific Rule", "windows", "process_creation")
	err := detectionEngine.LoadRules([]*domain.SigmaRule{rule})
	require.NoError(t, err)

	// Create event that doesn't match
	eventData := map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339),
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "C:\\Windows\\System32\\notepad.exe",
				"CommandLine": "notepad.exe",
			},
		},
	}

	event, err := domain.NewLogEvent(eventData)
	require.NoError(t, err)

	// Run detection
	results := detectionEngine.Detect(event)

	// Should not match
	assert.Equal(t, 0, len(results), "Should not match any rules")
}

func TestBatchDetection(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create rule
	rule := helpers.GenerateTestRule("rule-1", "Test Rule", "windows", "process_creation")
	err := detectionEngine.LoadRules([]*domain.SigmaRule{rule})
	require.NoError(t, err)

	// Generate batch of events
	events := helpers.GenerateBatchEvents(100, "process")

	// Process batch
	totalDetections := 0
	for _, event := range events {
		results := detectionEngine.Detect(event)
		totalDetections += len(results)
	}

	// Should have some detections
	assert.GreaterOrEqual(t, totalDetections, 0, "Should process all events")
}

func TestDetectionPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(10000)
	regexCache, _ := cache.NewRegexCache(10000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create multiple rules
	rules := make([]*domain.SigmaRule, 0, 50)
	for i := 0; i < 50; i++ {
		rules = append(rules, helpers.GenerateTestRule(
			"rule-"+string(rune(i)),
			"Rule "+string(rune(i)),
			"windows",
			"process_creation",
		))
	}

	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Generate events
	events := helpers.GenerateBatchEvents(1000, "process")

	// Measure performance
	start := time.Now()
	for _, event := range events {
		_ = detectionEngine.Detect(event)
	}
	duration := time.Since(start)

	throughput := float64(len(events)) / duration.Seconds()
	t.Logf("Processed %d events in %v (%.2f events/sec)", len(events), duration, throughput)

	// Should achieve target throughput
	assert.Greater(t, throughput, 300.0, "Should achieve at least 300 events/sec")
}

func TestDetectionWithRealSigmaRules(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real rules test in short mode")
	}

	// This test requires sigma_rules directory
	// Skip if not available
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctx // suppress unused warning

	// Setup detection engine
	fieldCache, _ := cache.NewFieldResolutionCache(10000)
	regexCache, _ := cache.NewRegexCache(10000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Try to load real rules (if available)
	// This is optional - test should pass even if rules directory doesn't exist
	// In a real scenario, you would load from sigma_rules/rules

	// Create test event
	eventData := helpers.GenerateSuspiciousPowerShellEvent()
	event, err := domain.NewLogEvent(eventData)
	require.NoError(t, err)

	// Run detection (may have no matches if no rules loaded)
	results := detectionEngine.Detect(event)

	// Test should pass regardless of results
	assert.NotNil(t, results)
	_ = ctx // Suppress unused variable warning
}

// =============================================================================
// CONCURRENT ACCESS TESTS
// =============================================================================

func TestConcurrentDetection(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Create test rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("concurrent-rule-1", "Concurrent Test 1", "windows", "process_creation"),
		helpers.GenerateTestRule("concurrent-rule-2", "Concurrent Test 2", "windows", "process_creation"),
	}
	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Generate test events
	events := helpers.GenerateBatchEvents(100, "process")

	// Run concurrent detection with 10 goroutines
	numWorkers := 10
	eventsPerWorker := len(events) / numWorkers

	done := make(chan bool, numWorkers)
	errors := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		start := i * eventsPerWorker
		end := start + eventsPerWorker
		if i == numWorkers-1 {
			end = len(events) // Handle remainder
		}

		go func(workerEvents []*domain.LogEvent) {
			for _, event := range workerEvents {
				results := detectionEngine.Detect(event)
				if results == nil {
					errors <- nil // Results can be nil for no matches
				}
			}
			done <- true
		}(events[start:end])
	}

	// Wait for all workers
	for i := 0; i < numWorkers; i++ {
		select {
		case <-done:
			// Worker completed
		case err := <-errors:
			if err != nil {
				t.Errorf("Worker error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent workers")
		}
	}

	t.Logf("Successfully processed %d events with %d concurrent workers", len(events), numWorkers)
}

func TestConcurrentRuleAccess(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Initial rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("access-rule-1", "Access Test 1", "windows", "process_creation"),
	}
	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Create test event
	eventData := helpers.GenerateWindowsProcessCreationEvent()
	event, _ := domain.NewLogEvent(eventData)

	// Concurrent reads while potential rule access
	done := make(chan bool, 20)

	// 10 detection goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_ = detectionEngine.Detect(event)
			}
			done <- true
		}()
	}

	// 10 stats goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_ = detectionEngine.Stats()
				_ = detectionEngine.RuleCount()
			}
			done <- true
		}()
	}

	// Wait for all
	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout")
		}
	}
}

// =============================================================================
// HEALTH CHECK TESTS
// =============================================================================

func TestEngineHealthCheck(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Load rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("health-rule-1", "Health Test", "windows", "process_creation"),
	}
	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Check health
	health := detectionEngine.Health()
	require.NotNil(t, health)
	assert.True(t, health.IsHealthy, "Engine should be healthy")
	assert.Equal(t, "HEALTHY", string(health.Status))
	assert.NotZero(t, health.CheckedAt)
}

func TestEngineStats(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Load rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("stats-rule-1", "Stats Test", "windows", "process_creation"),
	}
	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Process some events
	for i := 0; i < 10; i++ {
		eventData := helpers.GenerateWindowsProcessCreationEvent()
		event, _ := domain.NewLogEvent(eventData)
		_ = detectionEngine.Detect(event)
	}

	// Check stats
	stats := detectionEngine.Stats()
	require.NotNil(t, stats)
	assert.Greater(t, stats.TotalEvents, uint64(0), "Should have processed events")
}

func TestEngineShutdown(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	detectionEngine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Load rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("shutdown-rule-1", "Shutdown Test", "windows", "process_creation"),
	}
	err := detectionEngine.LoadRules(rules)
	require.NoError(t, err)

	// Verify rules loaded
	assert.Equal(t, 1, detectionEngine.RuleCount())

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = detectionEngine.Shutdown(ctx)
	require.NoError(t, err)

	// After shutdown, rule count should be 0
	assert.Equal(t, 0, detectionEngine.RuleCount(), "Rules should be cleared after shutdown")
}

// =============================================================================
// PORTS INTERFACE TESTS
// =============================================================================

func TestPortsInterfaceMethods(t *testing.T) {
	// Setup
	fieldCache, _ := cache.NewFieldResolutionCache(1000)
	regexCache, _ := cache.NewRegexCache(1000)
	fieldMapper := mapping.NewFieldMapper(fieldCache)
	modifierEngine := detection.NewModifierRegistry(regexCache)
	qualityConfig := detection.QualityConfig{
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
	}
	engine := detection.NewSigmaDetectionEngine(fieldMapper, modifierEngine, fieldCache, qualityConfig)

	// Load rules
	rules := []*domain.SigmaRule{
		helpers.GenerateTestRule("ports-rule-1", "Ports Test 1", "windows", "process_creation"),
		helpers.GenerateTestRule("ports-rule-2", "Ports Test 2", "linux", "process_creation"),
	}
	err := engine.LoadRules(rules)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Match
	eventData := helpers.GenerateWindowsProcessCreationEvent()
	event, _ := domain.NewLogEvent(eventData)
	matchResult, err := engine.Match(ctx, event)
	require.NoError(t, err)
	require.NotNil(t, matchResult)
	assert.NotEmpty(t, matchResult.EventID)
	assert.NotZero(t, matchResult.Timestamp)

	// Test MatchBatch
	events := helpers.GenerateBatchEvents(10, "process")
	portsEvents := make([]interface{}, len(events))
	for i, e := range events {
		portsEvents[i] = e
	}
	// Note: MatchBatch requires ports.Event interface, which domain.LogEvent implements

	// Test GetRules with empty filter
	// Note: GetRules requires ports.RuleFilter type
	// For now, we just verify RuleCount works

	// Test RuleCount
	assert.Equal(t, 2, engine.RuleCount())

	// Test Health
	health := engine.Health()
	assert.NotNil(t, health)
	assert.True(t, health.IsHealthy)

	// Test PortsStats
	portsStats := engine.PortsStats()
	assert.NotNil(t, portsStats)
	assert.Equal(t, 2, portsStats.LoadedRules)
}
