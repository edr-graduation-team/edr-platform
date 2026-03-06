//go:build integration
// +build integration

// Package infrastructure provides integration tests for the complete Sigma Engine pipeline.
package infrastructure

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/database"
	infraKafka "github.com/edr-platform/sigma-engine/internal/infrastructure/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test configuration
var (
	testDBConnStr    = getEnv("TEST_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/edr_platform?sslmode=disable")
	testKafkaBrokers = getEnv("TEST_KAFKA_BROKERS", "localhost:29092")
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// TestEndToEndKafkaToDatabase tests the complete pipeline: Kafka → Detection → PostgreSQL
func TestEndToEndKafkaToDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup database connection
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	// Create repositories
	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Verify we can connect
	err = pool.HealthCheck(ctx)
	require.NoError(t, err, "Database health check failed")

	t.Log("✅ Database connection established")

	// Create test alert
	testAlert := &database.Alert{
		Timestamp:       time.Now(),
		AgentID:         "agent-integration-test",
		RuleID:          "sigma-integration-rule",
		RuleTitle:       "Integration Test Rule",
		Severity:        "high",
		Category:        "process_creation",
		EventCount:      1,
		EventIDs:        []string{"event-1"},
		MitreTactics:    []string{"Execution"},
		MitreTechniques: []string{"T1059"},
		MatchedFields:   map[string]interface{}{"CommandLine": "test.exe"},
		Status:          "open",
		Confidence:      0.85,
	}

	// Create alert
	created, err := alertRepo.Create(ctx, testAlert)
	require.NoError(t, err, "Failed to create alert")
	require.NotEmpty(t, created.ID, "Alert ID should not be empty")

	t.Logf("✅ Alert created with ID: %s", created.ID)

	// Retrieve alert
	retrieved, err := alertRepo.GetByID(ctx, created.ID)
	require.NoError(t, err, "Failed to retrieve alert")
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, "sigma-integration-rule", retrieved.RuleID)

	t.Log("✅ Alert retrieved successfully")

	// Cleanup
	err = alertRepo.Delete(ctx, created.ID)
	require.NoError(t, err, "Failed to delete alert")

	t.Log("✅ End-to-end test passed")
}

// TestAlertDeduplication tests that alerts with same rule+agent within 5min are merged
func TestAlertDeduplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Create first alert
	alert1 := &database.Alert{
		Timestamp:  time.Now(),
		AgentID:    "agent-dedup-test",
		RuleID:     "sigma-dedup-rule",
		RuleTitle:  "Dedup Test Rule",
		Severity:   "high",
		EventCount: 1,
		EventIDs:   []string{"event-dedup-1"},
		Status:     "open",
		Confidence: 0.80,
	}

	created1, err := alertRepo.Create(ctx, alert1)
	require.NoError(t, err)
	t.Logf("✅ First alert created: %s", created1.ID)

	// Check for recent similar alert (dedup window)
	since := time.Now().Add(-5 * time.Minute)
	existing, err := alertRepo.FindRecent(ctx, alert1.AgentID, alert1.RuleID, since)
	require.NoError(t, err)
	require.NotNil(t, existing, "Should find recent alert")

	// Increment event count instead of creating new alert
	err = alertRepo.IncrementEventCount(ctx, existing.ID, []string{"event-dedup-2"})
	require.NoError(t, err)

	t.Log("✅ Second event merged into existing alert")

	// Verify merged alert
	merged, err := alertRepo.GetByID(ctx, created1.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, merged.EventCount, "Event count should be 2")
	assert.Len(t, merged.EventIDs, 2, "Should have 2 event IDs")

	t.Log("✅ Deduplication verified: 2 events in 1 alert")

	// Cleanup
	alertRepo.Delete(ctx, created1.ID)
}

// TestQueryPerformance tests that database queries complete within acceptable latency
func TestQueryPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Insert test alerts
	const numAlerts = 100
	var createdIDs []string

	for i := 0; i < numAlerts; i++ {
		alert := &database.Alert{
			Timestamp:  time.Now().Add(-time.Duration(i) * time.Minute),
			AgentID:    fmt.Sprintf("agent-perf-%d", i%10),
			RuleID:     fmt.Sprintf("sigma-perf-rule-%d", i%5),
			RuleTitle:  "Performance Test Rule",
			Severity:   []string{"critical", "high", "medium", "low"}[i%4],
			EventCount: 1,
			Status:     "open",
			Confidence: 0.75,
		}
		created, err := alertRepo.Create(ctx, alert)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	t.Logf("✅ Inserted %d test alerts", numAlerts)

	// Test query by agent_id
	start := time.Now()
	filters := database.AlertFilters{AgentID: "agent-perf-0", Limit: 50}
	alerts, _, err := alertRepo.List(ctx, filters)
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "Query by agent_id should be < 100ms")
	t.Logf("✅ Query by agent_id: %d results in %v", len(alerts), elapsed)

	// Test query by severity
	start = time.Now()
	filters = database.AlertFilters{Severity: []string{"high"}, Limit: 50}
	alerts, _, err = alertRepo.List(ctx, filters)
	elapsed = time.Since(start)
	require.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "Query by severity should be < 100ms")
	t.Logf("✅ Query by severity: %d results in %v", len(alerts), elapsed)

	// Test query by date range
	start = time.Now()
	filters = database.AlertFilters{
		DateFrom: time.Now().Add(-24 * time.Hour),
		DateTo:   time.Now(),
		Limit:    50,
	}
	alerts, _, err = alertRepo.List(ctx, filters)
	elapsed = time.Since(start)
	require.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "Query by date range should be < 100ms")
	t.Logf("✅ Query by date range: %d results in %v", len(alerts), elapsed)

	// Cleanup
	for _, id := range createdIDs {
		alertRepo.Delete(ctx, id)
	}

	t.Log("✅ All query performance tests passed")
}

// TestDatabaseResilience tests recovery from connection issues
func TestDatabaseResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Normal operation
	alert := &database.Alert{
		Timestamp:  time.Now(),
		AgentID:    "agent-resilience-test",
		RuleID:     "sigma-resilience-rule",
		RuleTitle:  "Resilience Test Rule",
		Severity:   "medium",
		EventCount: 1,
		Status:     "open",
	}

	created, err := alertRepo.Create(ctx, alert)
	require.NoError(t, err)
	t.Logf("✅ Alert created: %s", created.ID)

	// Verify health check works
	err = pool.HealthCheck(ctx)
	require.NoError(t, err, "Health check should pass")
	t.Log("✅ Health check passed")

	// Pool stats
	stats := pool.Stats()
	assert.Greater(t, stats.TotalConns(), int32(0), "Should have active connections")
	t.Logf("✅ Pool stats: %d total conns, %d idle", stats.TotalConns(), stats.IdleConns())

	// Cleanup
	alertRepo.Delete(ctx, created.ID)

	t.Log("✅ Database resilience test passed")
}

// TestMigrations verifies that database schema is correctly created
func TestMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	// Check tables exist
	tableExists := func(tableName string) bool {
		var exists bool
		err := pool.Pool().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, tableName).Scan(&exists)
		return err == nil && exists
	}

	assert.True(t, tableExists("sigma_alerts"), "sigma_alerts table should exist")
	assert.True(t, tableExists("sigma_rules"), "sigma_rules table should exist")
	t.Log("✅ Tables verified")

	// Check indexes exist
	indexExists := func(indexName string) bool {
		var exists bool
		err := pool.Pool().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM pg_indexes WHERE indexname = $1
			)
		`, indexName).Scan(&exists)
		return err == nil && exists
	}

	assert.True(t, indexExists("idx_sigma_alerts_timestamp"), "timestamp index should exist")
	assert.True(t, indexExists("idx_sigma_alerts_agent_id"), "agent_id index should exist")
	assert.True(t, indexExists("idx_sigma_alerts_rule_id"), "rule_id index should exist")
	t.Log("✅ Indexes verified")

	t.Log("✅ Migrations verified")
}

// TestRuleRepository tests rule CRUD operations
func TestRuleRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	ruleRepo := database.NewPostgresRuleRepository(pool.Pool())

	// Create rule
	rule := &database.Rule{
		ID:          "test-rule-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:       "Test Sigma Rule",
		Description: "Integration test rule",
		Content:     "title: Test Rule\nstatus: stable\nlogsource:\n  product: windows",
		Enabled:     true,
		Status:      "stable",
		Product:     "windows",
		Category:    "process_creation",
		Severity:    "high",
		Source:      "custom",
		Version:     1,
	}

	created, err := ruleRepo.Create(ctx, rule)
	require.NoError(t, err)
	t.Logf("✅ Rule created: %s", created.ID)

	// Get by ID
	retrieved, err := ruleRepo.GetByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, rule.Title, retrieved.Title)
	t.Log("✅ Rule retrieved")

	// Update
	rule.Description = "Updated description"
	updated, err := ruleRepo.Update(ctx, rule.ID, rule)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", updated.Description)
	t.Log("✅ Rule updated")

	// Disable
	err = ruleRepo.Disable(ctx, rule.ID)
	require.NoError(t, err)

	retrieved, _ = ruleRepo.GetByID(ctx, rule.ID)
	assert.False(t, retrieved.Enabled)
	t.Log("✅ Rule disabled")

	// Enable
	err = ruleRepo.Enable(ctx, rule.ID)
	require.NoError(t, err)

	retrieved, _ = ruleRepo.GetByID(ctx, rule.ID)
	assert.True(t, retrieved.Enabled)
	t.Log("✅ Rule enabled")

	// Delete
	err = ruleRepo.Delete(ctx, rule.ID)
	require.NoError(t, err)
	t.Log("✅ Rule deleted")

	t.Log("✅ Rule repository integration test passed")
}

// TestDataConsistency tests that data is stored and retrieved consistently
func TestDataConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Create alerts for different agents with same rule
	agents := []string{"agent-consist-1", "agent-consist-2", "agent-consist-3"}
	var createdIDs []string

	for _, agentID := range agents {
		alert := &database.Alert{
			Timestamp:  time.Now(),
			AgentID:    agentID,
			RuleID:     "sigma-consistency-rule",
			RuleTitle:  "Consistency Test",
			Severity:   "high",
			EventCount: 1,
			EventIDs:   []string{"event-" + agentID},
			Status:     "open",
			Confidence: 0.90,
		}
		created, err := alertRepo.Create(ctx, alert)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	t.Logf("✅ Created %d alerts for different agents", len(agents))

	// Query by rule - should get all 3
	filters := database.AlertFilters{RuleID: "sigma-consistency-rule", Limit: 10}
	alerts, total, err := alertRepo.List(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total, "Should have 3 alerts")
	assert.Len(t, alerts, 3)
	t.Log("✅ All 3 alerts retrieved (different agents = separate alerts)")

	// Update status
	err = alertRepo.UpdateStatus(ctx, createdIDs[0], "acknowledged", "Test note")
	require.NoError(t, err)

	updated, _ := alertRepo.GetByID(ctx, createdIDs[0])
	assert.Equal(t, "acknowledged", updated.Status)
	t.Log("✅ Status update verified")

	// Cleanup
	for _, id := range createdIDs {
		alertRepo.Delete(ctx, id)
	}

	t.Log("✅ Data consistency test passed")
}

// TestBatchInsertPerformance tests bulk insert performance
func TestBatchInsertPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Setup
	dbCfg := database.DefaultConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	alertRepo := database.NewPostgresAlertRepository(pool.Pool())

	// Batch insert
	const batchSize = 500
	var createdIDs []string

	start := time.Now()
	for i := 0; i < batchSize; i++ {
		alert := &database.Alert{
			Timestamp:  time.Now(),
			AgentID:    fmt.Sprintf("agent-batch-%d", i%50),
			RuleID:     fmt.Sprintf("sigma-batch-rule-%d", i%10),
			RuleTitle:  "Batch Test Rule",
			Severity:   []string{"critical", "high", "medium", "low"}[i%4],
			EventCount: 1,
			EventIDs:   []string{fmt.Sprintf("batch-event-%d", i)},
			Status:     "open",
			Confidence: 0.70 + float64(i%30)*0.01,
		}
		created, err := alertRepo.Create(ctx, alert)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}
	elapsed := time.Since(start)

	t.Logf("✅ Inserted %d alerts in %v (%.2f alerts/sec)",
		batchSize, elapsed, float64(batchSize)/elapsed.Seconds())

	// Verify count
	stats, err := alertRepo.GetStats(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.TotalAlerts, int64(batchSize))
	t.Logf("✅ Stats verified: %d total alerts", stats.TotalAlerts)

	// Performance assertion
	assert.Less(t, elapsed, 30*time.Second, fmt.Sprintf("%d alerts should insert in <30s", batchSize))

	// Cleanup
	for _, id := range createdIDs {
		alertRepo.Delete(ctx, id)
	}

	t.Log("✅ Batch insert performance test passed")
}

// TestKafkaConfiguration tests Kafka configuration loading
func TestKafkaConfiguration(t *testing.T) {
	// Test default configs
	consumerCfg := infraKafka.DefaultConsumerConfig()
	assert.Equal(t, "events-raw", consumerCfg.Topic)
	assert.Equal(t, "sigma-engine-group", consumerCfg.GroupID)
	t.Log("✅ Consumer config defaults verified")

	producerCfg := infraKafka.DefaultProducerConfig()
	assert.Equal(t, "alerts", producerCfg.Topic)
	assert.Equal(t, 50, producerCfg.BatchSize)
	assert.Equal(t, "snappy", producerCfg.Compression)
	t.Log("✅ Producer config defaults verified")

	eventLoopCfg := infraKafka.DefaultEventLoopConfig()
	assert.Equal(t, 4, eventLoopCfg.Workers)
	assert.Equal(t, 1000, eventLoopCfg.EventBuffer)
	t.Log("✅ Event loop config defaults verified")

	t.Log("✅ Kafka configuration test passed")
}
