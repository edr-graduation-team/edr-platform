// Package ports defines the public API contracts for the Sigma Detection Engine.
// External modules should import only from this package, never from internal/.
package ports

import (
	"context"
	"time"
)

// DetectionEngine is the public interface for Sigma detection.
// This interface defines the contract for integrating the detection engine
// into a modular monolith or other external systems.
type DetectionEngine interface {
	// Match evaluates a single event against all loaded rules.
	// Returns MatchResult containing all matched rules with confidence scores.
	// Thread-safe: can be called concurrently from multiple goroutines.
	Match(ctx context.Context, event Event) (*MatchResult, error)

	// MatchBatch processes multiple events efficiently using internal worker pools.
	// More efficient than calling Match() in a loop for large batches.
	MatchBatch(ctx context.Context, events []Event) (*BatchMatchResult, error)

	// LoadRules loads all rules into the engine, replacing any existing rules.
	// This triggers index rebuilding and is safe to call at runtime (hot-reload).
	LoadRules(ctx context.Context, rules []Rule) error

	// AddRules adds rules to the existing ruleset without replacing.
	// Returns error if any rule ID already exists.
	AddRules(ctx context.Context, rules []Rule) error

	// RemoveRule removes a single rule by its ID.
	// Returns error if rule not found.
	RemoveRule(ctx context.Context, ruleID string) error

	// GetRules returns rules matching the given filter.
	// Pass empty filter to get all rules.
	GetRules(ctx context.Context, filter RuleFilter) ([]Rule, error)

	// RuleCount returns the number of loaded rules.
	RuleCount() int

	// Stats returns engine runtime statistics.
	Stats() *EngineStats

	// Health returns engine health status for monitoring.
	Health() *EngineHealth

	// Shutdown gracefully stops the engine.
	// All in-flight detections will complete before returning.
	Shutdown(ctx context.Context) error
}

// =============================================================================
// RESULT TYPES
// =============================================================================

// MatchResult contains detection results for a single event.
type MatchResult struct {
	// EventID identifies the event that was evaluated
	EventID string `json:"event_id"`

	// Matched is true if at least one rule matched
	Matched bool `json:"matched"`

	// MatchCount is the number of rules that matched
	MatchCount int `json:"match_count"`

	// Matches contains details of each matching rule
	Matches []RuleMatch `json:"matches,omitempty"`

	// EvaluatedRules is the number of rules evaluated (after index filtering)
	EvaluatedRules int `json:"evaluated_rules"`

	// LatencyMs is the time taken for evaluation in milliseconds
	LatencyMs float64 `json:"latency_ms"`

	// Timestamp when the match was performed
	Timestamp time.Time `json:"timestamp"`
}

// RuleMatch represents a single matched rule.
type RuleMatch struct {
	RuleID          string                 `json:"rule_id"`
	RuleTitle       string                 `json:"rule_title"`
	Severity        string                 `json:"severity"`
	Confidence      float64                `json:"confidence"`
	MatchedFields   map[string]interface{} `json:"matched_fields,omitempty"`
	MITRETechniques []string               `json:"mitre_techniques,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
}

// BatchMatchResult aggregates batch processing results.
type BatchMatchResult struct {
	// TotalEvents is the number of events in the batch
	TotalEvents int `json:"total_events"`

	// MatchedEvents is the number of events with at least one match
	MatchedEvents int `json:"matched_events"`

	// TotalMatches is the sum of all rule matches across all events
	TotalMatches int `json:"total_matches"`

	// Results contains per-event results (may be nil for large batches)
	Results []MatchResult `json:"results,omitempty"`

	// Stats contains batch processing statistics
	Stats BatchStats `json:"stats"`
}

// BatchStats contains batch processing statistics.
type BatchStats struct {
	TotalTimeMs   float64 `json:"total_time_ms"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	MaxLatencyMs  float64 `json:"max_latency_ms"`
	MinLatencyMs  float64 `json:"min_latency_ms"`
	ThroughputEPS float64 `json:"throughput_eps"` // events per second
}

// =============================================================================
// STATISTICS & HEALTH
// =============================================================================

// EngineStats contains engine runtime statistics.
type EngineStats struct {
	// LoadedRules is the number of rules currently loaded
	LoadedRules int `json:"loaded_rules"`

	// RulesByProduct maps product name to rule count
	RulesByProduct map[string]int `json:"rules_by_product,omitempty"`

	// EventsProcessed is the total number of events processed
	EventsProcessed uint64 `json:"events_processed"`

	// DetectionsFound is the total number of detections (rule matches)
	DetectionsFound uint64 `json:"detections_found"`

	// AlertsGenerated is the number of alerts generated
	AlertsGenerated uint64 `json:"alerts_generated"`

	// PanicCount is the number of panics recovered
	PanicCount uint64 `json:"panic_count"`

	// AvgLatencyMs is the average detection latency in milliseconds
	AvgLatencyMs float64 `json:"avg_latency_ms"`

	// P99LatencyMs is the 99th percentile latency (if tracked)
	P99LatencyMs float64 `json:"p99_latency_ms,omitempty"`

	// Uptime is how long the engine has been running
	Uptime time.Duration `json:"uptime"`

	// LastUpdated is when stats were last updated
	LastUpdated time.Time `json:"last_updated"`
}

// EngineHealth represents engine health status.
type EngineHealth struct {
	// Status is the overall health status
	Status HealthStatus `json:"status"`

	// IsHealthy is true if the engine is functioning normally
	IsHealthy bool `json:"is_healthy"`

	// PanicCount is the number of panics since startup
	PanicCount uint64 `json:"panic_count"`

	// WorkerHealth maps worker ID to health status
	WorkerHealth map[int]WorkerHealth `json:"worker_health,omitempty"`

	// LastError is the most recent error message (if any)
	LastError string `json:"last_error,omitempty"`

	// CheckedAt is when the health check was performed
	CheckedAt time.Time `json:"checked_at"`
}

// HealthStatus represents the overall health state.
type HealthStatus string

const (
	// HealthStatusHealthy indicates the engine is fully operational
	HealthStatusHealthy HealthStatus = "HEALTHY"

	// HealthStatusDegraded indicates reduced functionality (e.g., some panics)
	HealthStatusDegraded HealthStatus = "DEGRADED"

	// HealthStatusCritical indicates severe issues requiring attention
	HealthStatusCritical HealthStatus = "CRITICAL"
)

// WorkerHealth represents individual worker health status.
type WorkerHealth struct {
	WorkerID   int       `json:"worker_id"`
	IsHealthy  bool      `json:"is_healthy"`
	PanicCount uint64    `json:"panic_count"`
	LastActive time.Time `json:"last_active"`
}

// =============================================================================
// FILTER & CONFIG TYPES
// =============================================================================

// RuleFilter for querying rules.
type RuleFilter struct {
	// Product filters by logsource product (e.g., "windows", "linux")
	Product string `json:"product,omitempty"`

	// Category filters by logsource category (e.g., "process_creation")
	Category string `json:"category,omitempty"`

	// Level filters by severity level (e.g., "high", "critical")
	Level string `json:"level,omitempty"`

	// Tags filters by rule tags (rules must have ALL specified tags)
	Tags []string `json:"tags,omitempty"`

	// Status filters by rule status (e.g., "stable", "experimental")
	Status string `json:"status,omitempty"`

	// IDs filters by specific rule IDs
	IDs []string `json:"ids,omitempty"`
}

// =============================================================================
// MINIMAL INTERFACES FOR LOOSE COUPLING
// =============================================================================

// Event is the minimal interface for events passed to the detection engine.
// This allows the engine to work with any event type that implements these methods.
type Event interface {
	// GetField retrieves a field value by path (e.g., "process.command_line")
	GetField(path string) (interface{}, bool)

	// GetStringField retrieves a field value as string
	GetStringField(path string) string

	// GetCategory returns the event category for rule filtering
	GetCategory() string

	// GetProduct returns the event product for rule filtering
	GetProduct() string
}

// Rule is the minimal interface for rules.
// This allows external code to work with rules without importing internal types.
type Rule interface {
	// GetID returns the unique rule identifier
	GetID() string

	// GetTitle returns the human-readable rule title
	GetTitle() string

	// GetLevel returns the severity level
	GetLevel() string

	// GetTags returns the rule tags
	GetTags() []string
}
