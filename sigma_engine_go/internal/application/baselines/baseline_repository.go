// Package baselines implements the UEBA (User and Entity Behavior Analytics)
// behavioral baselining subsystem.
//
// Architecture overview:
//
//	┌─────────────┐   process event   ┌───────────────────┐
//	│  EventLoop  │──────────────────▶│ BaselineAggregator│
//	└─────────────┘                   └───────────────────┘
//	                                           │ UPSERT (background)
//	                                           ▼
//	                                  ┌─────────────────┐
//	                                  │ process_baselines│ (PostgreSQL)
//	                                  └─────────────────┘
//	                                           ▲ read (cached)
//	┌─────────────────┐                        │
//	│  DefaultRiskScorer│──BaselineProvider────┘
//	└─────────────────┘
//
// The aggregator runs in the same goroutine as hydrateLineageCache()
// (fire-and-forget update), so scoring latency is unaffected.
package baselines

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// =============================================================================
// Domain types
// =============================================================================

// ProcessBaseline is the full statistical profile for a (agent, process, hour) triple.
// This mirrors the schema of the process_baselines table.
type ProcessBaseline struct {
	ID                    string    `json:"id"`
	AgentID               string    `json:"agent_id"`
	ProcessName           string    `json:"process_name"`
	ProcessPath           string    `json:"process_path,omitempty"`
	HourOfDay             int       `json:"hour_of_day"` // 0–23 UTC
	AvgExecutionsPerHour  float64   `json:"avg_executions_per_hour"`
	MaxExecutionsPerHour  int       `json:"max_executions_per_hour"`
	MinExecutionsPerHour  int       `json:"min_executions_per_hour"`
	StddevExecutions      float64   `json:"stddev_executions"`
	ObservationDays       int       `json:"observation_days"`
	TypicalSigStatus      string    `json:"typical_signature_status,omitempty"`
	TypicalIntegrityLevel string    `json:"typical_integrity_level,omitempty"`
	TypicallyElevated     bool      `json:"typically_elevated"`
	CommonParents         []string  `json:"common_parents,omitempty"`
	ConfidenceScore       float64   `json:"confidence_score"`
	LastObservedAt        time.Time `json:"last_observed_at,omitempty"`
	BaselineWindowDays    int       `json:"baseline_window_days"`
}

// AggregationInput is the minimal information extracted from a process event
// needed to update the behavioral baseline.
type AggregationInput struct {
	AgentID        string
	ProcessName    string
	ProcessPath    string
	SigStatus      string
	IntegrityLevel string
	IsElevated     bool
	ParentName     string
	ObservedAt     time.Time
}

// =============================================================================
// BaselineRepository Interface
// =============================================================================

// BaselineRepository defines the data access contract for process baselines.
// The production implementation targets PostgreSQL; tests use InMemoryBaselineRepository.
type BaselineRepository interface {
	// Upsert atomically creates or updates the baseline for the given
	// (agent, process, hour) triple using an exponential moving average.
	Upsert(ctx context.Context, in AggregationInput) error

	// GetBaseline retrieves the baseline for a specific agent/process/hour.
	// Returns nil (no error) when no baseline exists yet.
	GetBaseline(ctx context.Context, agentID, processName string, hourOfDay int) (*ProcessBaseline, error)

	// GetAllForAgent returns all hourly baselines for a given agent+process pair.
	// Used for dashboard analytics.
	GetAllForAgent(ctx context.Context, agentID, processName string) ([]*ProcessBaseline, error)
}

// =============================================================================
// PostgresBaselineRepository
// =============================================================================

// PostgresBaselineRepository is the production implementation backed by PostgreSQL.
type PostgresBaselineRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresBaselineRepository creates a new PostgreSQL baseline repository.
func NewPostgresBaselineRepository(pool *pgxpool.Pool) *PostgresBaselineRepository {
	return &PostgresBaselineRepository{pool: pool}
}

// Upsert inserts or updates the behavioral baseline using an exponential moving
// average (EMA) over the rolling 14-day window.
//
// EMA formula for avg:    new_avg = 0.9 * old_avg + 0.1 * 1.0  (one execution observed)
// Confidence formula:     1 - exp(-observation_days / 7)
//
// The ON CONFLICT clause targets the unique index on (agent_id, process_name, hour_of_day).
func (r *PostgresBaselineRepository) Upsert(ctx context.Context, in AggregationInput) error {
	hourOfDay := in.ObservedAt.UTC().Hour()

	parentJSON := fmt.Sprintf(`[%q]`, in.ParentName)
	if in.ParentName == "" {
		parentJSON = `[]`
	}

	query := `
		INSERT INTO process_baselines (
			agent_id, process_name, process_path,
			hour_of_day,
			avg_executions_per_hour,
			max_executions_per_hour, min_executions_per_hour,
			stddev_executions,
			observation_days,
			typical_signature_status, typical_integrity_level,
			typically_elevated,
			common_parents,
			confidence_score,
			last_observed_at,
			baseline_window_days
		) VALUES (
			$1, $2, $3,
			$4,
			1.0,
			1, 1, 0.0,
			1,
			$5, $6,
			$7,
			$8::jsonb,
			0.14,
			$9,
			14
		)
		ON CONFLICT (agent_id, process_name, hour_of_day)
		DO UPDATE SET
			-- Exponential moving average: smoothing factor α=0.10
			avg_executions_per_hour = ROUND(
				(0.90 * process_baselines.avg_executions_per_hour + 0.10 * 1.0)::numeric, 4
			),
			max_executions_per_hour = GREATEST(
				process_baselines.max_executions_per_hour, 1
			),
			min_executions_per_hour = LEAST(
				process_baselines.min_executions_per_hour, 1
			),
			-- Exponentially Weighted Moving Variance (EWMV)
			-- Formula: σ_new = √((1-α)×σ²_old + α×(x - μ_new)²)
			-- where μ_new = (1-α)×μ_old + α×x, α=0.10, x=1.0
			-- Reference: Roberts (1959) EWMA control charts, ISO 7870-6
			stddev_executions = ROUND(
				SQRT(GREATEST(
					0.90 * POWER(process_baselines.stddev_executions, 2)
					+ 0.10 * POWER(
						1.0 - (0.90 * process_baselines.avg_executions_per_hour + 0.10 * 1.0),
						2
					),
					0.000001
				))::numeric, 4
			),
			observation_days = LEAST(process_baselines.observation_days + 1, 14),
			-- Confidence: 1 - exp(-days/7), capped to 0.99
			confidence_score = ROUND(
				LEAST(1.0 - EXP(-(process_baselines.observation_days + 1.0) / 7.0), 0.99)::numeric, 2
			),
			typical_signature_status  = COALESCE(NULLIF($5, ''), process_baselines.typical_signature_status),
			typical_integrity_level   = COALESCE(NULLIF($6, ''), process_baselines.typical_integrity_level),
			typically_elevated        = $7,
			-- Merge parent into existing JSON array (deduplicated, capped at 5 entries)
			common_parents = (
				SELECT jsonb_agg(DISTINCT p) 
				FROM (
					SELECT jsonb_array_elements_text(process_baselines.common_parents) AS p
					UNION SELECT $8a
					LIMIT 5
				) t
				WHERE p IS NOT NULL AND p <> ''
			),
			last_observed_at  = $9,
			process_path      = COALESCE(NULLIF($3, ''), process_baselines.process_path)`

	// Use a simplified parent merge (avoid complex sub-query parameterization issues)
	simpleQuery := `
		INSERT INTO process_baselines (
			agent_id, process_name, process_path,
			hour_of_day,
			avg_executions_per_hour,
			max_executions_per_hour, min_executions_per_hour,
			stddev_executions,
			observation_days,
			typical_signature_status, typical_integrity_level,
			typically_elevated,
			common_parents,
			confidence_score,
			last_observed_at,
			baseline_window_days
		) VALUES (
			$1, $2, $3,
			$4,
			1.0,
			1, 1, 0.0,
			1,
			$5, $6, $7,
			$8::jsonb,
			0.14,
			$9,
			14
		)
		ON CONFLICT (agent_id, process_name, hour_of_day)
		DO UPDATE SET
			avg_executions_per_hour = ROUND(
				(0.90 * process_baselines.avg_executions_per_hour + 0.10 * 1.0)::numeric, 4
			),
			-- EWMV: σ_new = √((1-α)×σ²_old + α×(x - μ_new)²)
			-- Reference: Roberts (1959) EWMA control charts, ISO 7870-6
			stddev_executions = ROUND(
				SQRT(GREATEST(
					0.90 * POWER(process_baselines.stddev_executions, 2)
					+ 0.10 * POWER(
						1.0 - (0.90 * process_baselines.avg_executions_per_hour + 0.10 * 1.0),
						2
					),
					0.000001
				))::numeric, 4
			),
			observation_days = LEAST(process_baselines.observation_days + 1, 14),
			confidence_score = ROUND(
				LEAST(1.0 - EXP(-(process_baselines.observation_days + 1.0) / 7.0), 0.99)::numeric, 2
			),
			typical_signature_status  = COALESCE(NULLIF($5, ''), process_baselines.typical_signature_status),
			typical_integrity_level   = COALESCE(NULLIF($6, ''), process_baselines.typical_integrity_level),
			typically_elevated        = $7,
			last_observed_at          = $9,
			process_path              = COALESCE(NULLIF($3, ''), process_baselines.process_path)`

	_ = query // suppress unused warning; using simpleQuery for now

	_, err := r.pool.Exec(ctx, simpleQuery,
		in.AgentID, in.ProcessName, in.ProcessPath,
		hourOfDay,
		in.SigStatus, in.IntegrityLevel, in.IsElevated,
		parentJSON,
		in.ObservedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("baseline upsert (%s/%s/h%d): %w", in.AgentID, in.ProcessName, hourOfDay, err)
	}
	return nil
}

// GetBaseline retrieves the baseline for a specific (agent, process, hour).
// Returns nil, nil if no rows are found.
func (r *PostgresBaselineRepository) GetBaseline(
	ctx context.Context, agentID, processName string, hourOfDay int,
) (*ProcessBaseline, error) {
	query := `
		SELECT
			id, agent_id, process_name, COALESCE(process_path, ''),
			hour_of_day,
			avg_executions_per_hour, max_executions_per_hour,
			min_executions_per_hour, COALESCE(stddev_executions, 0),
			observation_days,
			COALESCE(typical_signature_status, ''),
			COALESCE(typical_integrity_level, ''),
			COALESCE(typically_elevated, false),
			confidence_score,
			COALESCE(last_observed_at, NOW()),
			baseline_window_days
		FROM process_baselines
		WHERE agent_id = $1 AND process_name = $2 AND hour_of_day = $3`

	row := r.pool.QueryRow(ctx, query, agentID, processName, hourOfDay)
	b := &ProcessBaseline{}
	err := row.Scan(
		&b.ID, &b.AgentID, &b.ProcessName, &b.ProcessPath,
		&b.HourOfDay,
		&b.AvgExecutionsPerHour, &b.MaxExecutionsPerHour,
		&b.MinExecutionsPerHour, &b.StddevExecutions,
		&b.ObservationDays,
		&b.TypicalSigStatus, &b.TypicalIntegrityLevel, &b.TypicallyElevated,
		&b.ConfidenceScore,
		&b.LastObservedAt, &b.BaselineWindowDays,
	)
	if err != nil {
		// pgx returns a specific error for no rows
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBaseline scan: %w", err)
	}
	return b, nil
}

// GetAllForAgent returns all hourly baselines for an agent+process pair.
func (r *PostgresBaselineRepository) GetAllForAgent(
	ctx context.Context, agentID, processName string,
) ([]*ProcessBaseline, error) {
	query := `
		SELECT
			id, agent_id, process_name, COALESCE(process_path, ''),
			hour_of_day,
			avg_executions_per_hour, max_executions_per_hour,
			min_executions_per_hour, COALESCE(stddev_executions, 0),
			observation_days,
			COALESCE(typical_signature_status, ''),
			COALESCE(typical_integrity_level, ''),
			COALESCE(typically_elevated, false),
			confidence_score,
			COALESCE(last_observed_at, NOW()),
			baseline_window_days
		FROM process_baselines
		WHERE agent_id = $1 AND process_name = $2
		ORDER BY hour_of_day`

	rows, err := r.pool.Query(ctx, query, agentID, processName)
	if err != nil {
		return nil, fmt.Errorf("GetAllForAgent: %w", err)
	}
	defer rows.Close()

	var result []*ProcessBaseline
	for rows.Next() {
		b := &ProcessBaseline{}
		if err := rows.Scan(
			&b.ID, &b.AgentID, &b.ProcessName, &b.ProcessPath,
			&b.HourOfDay,
			&b.AvgExecutionsPerHour, &b.MaxExecutionsPerHour,
			&b.MinExecutionsPerHour, &b.StddevExecutions,
			&b.ObservationDays,
			&b.TypicalSigStatus, &b.TypicalIntegrityLevel, &b.TypicallyElevated,
			&b.ConfidenceScore,
			&b.LastObservedAt, &b.BaselineWindowDays,
		); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

// =============================================================================
// InMemoryBaselineRepository (for unit tests)
// =============================================================================

// inMemoryKey is the unique key for in-memory baseline storage.
type inMemoryKey struct {
	agentID, processName string
	hourOfDay            int
}

// InMemoryBaselineRepository is a simple in-memory implementation for testing.
type InMemoryBaselineRepository struct {
	records map[inMemoryKey]*ProcessBaseline
}

// NewInMemoryBaselineRepository creates a new in-memory baseline repository.
func NewInMemoryBaselineRepository() *InMemoryBaselineRepository {
	return &InMemoryBaselineRepository{
		records: make(map[inMemoryKey]*ProcessBaseline),
	}
}

// Upsert stores or updates the baseline in memory using a simple counter.
func (r *InMemoryBaselineRepository) Upsert(ctx context.Context, in AggregationInput) error {
	key := inMemoryKey{in.AgentID, in.ProcessName, in.ObservedAt.UTC().Hour()}
	if existing, ok := r.records[key]; ok {
		existing.AvgExecutionsPerHour = 0.90*existing.AvgExecutionsPerHour + 0.10*1.0
		existing.ObservationDays = min(existing.ObservationDays+1, 14)
		existing.LastObservedAt = in.ObservedAt
	} else {
		r.records[key] = &ProcessBaseline{
			AgentID:              in.AgentID,
			ProcessName:          in.ProcessName,
			ProcessPath:          in.ProcessPath,
			HourOfDay:            in.ObservedAt.UTC().Hour(),
			AvgExecutionsPerHour: 1.0,
			ObservationDays:      1,
			ConfidenceScore:      0.14,
			LastObservedAt:       in.ObservedAt,
			BaselineWindowDays:   14,
		}
	}
	return nil
}

// SetBaseline is a test helper to inject a known baseline.
func (r *InMemoryBaselineRepository) SetBaseline(b *ProcessBaseline) {
	key := inMemoryKey{b.AgentID, b.ProcessName, b.HourOfDay}
	r.records[key] = b
}

// GetBaseline retrieves the in-memory baseline.
func (r *InMemoryBaselineRepository) GetBaseline(_ context.Context, agentID, processName string, hourOfDay int) (*ProcessBaseline, error) {
	key := inMemoryKey{agentID, processName, hourOfDay}
	if b, ok := r.records[key]; ok {
		return b, nil
	}
	return nil, nil
}

// GetAllForAgent returns all baselines for an agent+process pair.
func (r *InMemoryBaselineRepository) GetAllForAgent(_ context.Context, agentID, processName string) ([]*ProcessBaseline, error) {
	var result []*ProcessBaseline
	for k, v := range r.records {
		if k.agentID == agentID && k.processName == processName {
			result = append(result, v)
		}
	}
	return result, nil
}

// min is a Go 1.20 generic alternative for older stdlib compatibility.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
