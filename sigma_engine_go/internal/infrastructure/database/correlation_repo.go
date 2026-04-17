// Package database — PostgreSQL persistence for alert correlation edges.
package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CorrelationRepository stores sigma_alert_correlations rows.
type CorrelationRepository struct {
	pool *pgxpool.Pool
}

// NewCorrelationRepository creates a correlation edge repository.
func NewCorrelationRepository(pool *pgxpool.Pool) *CorrelationRepository {
	return &CorrelationRepository{pool: pool}
}

// UpsertEdge inserts or updates a single undirected edge (canonical low < high IDs).
func (r *CorrelationRepository) UpsertEdge(ctx context.Context, low, high, relationType string, score float64) error {
	if r == nil || r.pool == nil {
		return nil
	}
	q := `
		INSERT INTO sigma_alert_correlations (alert_low_id, alert_high_id, relation_type, correlation_score, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (alert_low_id, alert_high_id) DO UPDATE SET
			correlation_score = GREATEST(sigma_alert_correlations.correlation_score, EXCLUDED.correlation_score),
			relation_type = CASE
				WHEN EXCLUDED.correlation_score > sigma_alert_correlations.correlation_score THEN EXCLUDED.relation_type
				ELSE sigma_alert_correlations.relation_type
			END,
			updated_at = CURRENT_TIMESTAMP`
	_, err := r.pool.Exec(ctx, q, low, high, relationType, score)
	if err != nil {
		return fmt.Errorf("upsert correlation edge: %w", err)
	}
	return nil
}

// PruneEdgesOlderThan deletes stale edges and returns removed row count.
func (r *CorrelationRepository) PruneEdgesOlderThan(ctx context.Context, ttl time.Duration) (int64, error) {
	if r == nil || r.pool == nil || ttl <= 0 {
		return 0, nil
	}
	q := `DELETE FROM sigma_alert_correlations WHERE updated_at < NOW() - $1::interval`
	tag, err := r.pool.Exec(ctx, q, fmt.Sprintf("%f seconds", ttl.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("prune correlation edges: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CorrelationEdgeRow is a row from sigma_alert_correlations.
type CorrelationEdgeRow struct {
	AlertLowID        string
	AlertHighID       string
	RelationType      string
	CorrelationScore  float64
	CreatedAt         time.Time
}

// ListRecentEdges returns the most recent edges up to limit (for bootstrap).
func (r *CorrelationRepository) ListRecentEdges(ctx context.Context, limit int) ([]CorrelationEdgeRow, error) {
	if r == nil || r.pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10000
	}
	q := `
		SELECT alert_low_id, alert_high_id, relation_type, correlation_score, created_at
		FROM sigma_alert_correlations
		ORDER BY created_at DESC
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CorrelationEdgeRow
	for rows.Next() {
		var row CorrelationEdgeRow
		if err := rows.Scan(&row.AlertLowID, &row.AlertHighID, &row.RelationType, &row.CorrelationScore, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LoadRecentAlertsForCorrelation loads recent alerts to warm the in-memory correlation cache.
func (r *CorrelationRepository) LoadRecentAlertsForCorrelation(ctx context.Context, limit int) ([]*domain.Alert, error) {
	if r == nil || r.pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	q := `
		SELECT id::text, timestamp, agent_id, rule_id, context_data
		FROM sigma_alerts
		ORDER BY timestamp DESC
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Alert
	for rows.Next() {
		var id, agentID, ruleID string
		var ts time.Time
		var contextJSON []byte
		if err := rows.Scan(&id, &ts, &agentID, &ruleID, &contextJSON); err != nil {
			return nil, err
		}
		var ctxData map[string]interface{}
		if len(contextJSON) > 0 {
			_ = json.Unmarshal(contextJSON, &ctxData)
		}
		if ctxData == nil {
			ctxData = make(map[string]interface{})
		}
		if agentID != "" {
			ctxData["agent_id"] = agentID
		}
		out = append(out, &domain.Alert{
			ID:        id,
			Timestamp: ts,
			RuleID:    ruleID,
			EventData: ctxData,
		})
	}
	return out, rows.Err()
}

// LoadCandidateAlertsForCorrelation loads recent alerts likely to correlate with the provided alert.
// Used to improve cross-replica correlation when each engine instance has its own memory cache.
func (r *CorrelationRepository) LoadCandidateAlertsForCorrelation(
	ctx context.Context,
	seed *domain.Alert,
	window time.Duration,
	limit int,
) ([]*domain.Alert, error) {
	if r == nil || r.pool == nil || seed == nil {
		return nil, nil
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	if limit <= 0 {
		limit = 200
	}

	agentID := ""
	if seed.EventData != nil {
		if v, ok := seed.EventData["agent_id"]; ok && v != nil {
			agentID = fmt.Sprint(v)
		}
	}
	userSID := ""
	userName := ""
	if seed.EventData != nil {
		if v, ok := seed.EventData["user_sid"]; ok && v != nil {
			userSID = fmt.Sprint(v)
		}
		if v, ok := seed.EventData["user_name"]; ok && v != nil {
			userName = fmt.Sprint(v)
		}
	}

	q := `
		SELECT id::text, timestamp, agent_id, rule_id, context_data
		FROM sigma_alerts
		WHERE id::text <> $1
		  AND timestamp >= $2
		  AND (
			 rule_id = $3
			 OR agent_id = $4
			 OR context_data->>'agent_id' = $4
			 OR context_data->>'user_sid' = $5
			 OR context_data->>'user_name' = $6
		  )
		ORDER BY timestamp DESC
		LIMIT $7`
	start := seed.Timestamp.Add(-window)
	rows, err := r.pool.Query(ctx, q, seed.ID, start, seed.RuleID, agentID, userSID, userName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Alert
	for rows.Next() {
		var id, dbAgentID, ruleID string
		var ts time.Time
		var contextJSON []byte
		if err := rows.Scan(&id, &ts, &dbAgentID, &ruleID, &contextJSON); err != nil {
			return nil, err
		}
		var ctxData map[string]interface{}
		if len(contextJSON) > 0 {
			_ = json.Unmarshal(contextJSON, &ctxData)
		}
		if ctxData == nil {
			ctxData = make(map[string]interface{})
		}
		if dbAgentID != "" {
			ctxData["agent_id"] = dbAgentID
		}
		out = append(out, &domain.Alert{
			ID:        id,
			Timestamp: ts,
			RuleID:    ruleID,
			EventData: ctxData,
		})
	}
	return out, rows.Err()
}
