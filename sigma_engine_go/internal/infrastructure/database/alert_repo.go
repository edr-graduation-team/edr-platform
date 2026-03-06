// Package database provides PostgreSQL alert repository implementation.
package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAlertRepository implements AlertRepository using PostgreSQL.
type PostgresAlertRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAlertRepository creates a new PostgreSQL alert repository.
func NewPostgresAlertRepository(pool *pgxpool.Pool) *PostgresAlertRepository {
	return &PostgresAlertRepository{pool: pool}
}

// Create inserts a new alert into the database.
func (r *PostgresAlertRepository) Create(ctx context.Context, alert *Alert) (*Alert, error) {
	matchedFieldsJSON, _ := json.Marshal(alert.MatchedFields)
	contextDataJSON, _ := json.Marshal(alert.ContextData)

	query := `
		INSERT INTO sigma_alerts (
			timestamp, agent_id, rule_id, rule_title, severity, category,
			event_count, event_ids, mitre_tactics, mitre_techniques,
			matched_fields, matched_selections, context_data,
			status, confidence, false_positive_risk,
			match_count, related_rules, combined_confidence,
			severity_promoted, original_severity
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15, $16,
			$17, $18, $19,
			$20, $21
		) RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		alert.Timestamp, alert.AgentID, alert.RuleID, alert.RuleTitle, alert.Severity, alert.Category,
		alert.EventCount, alert.EventIDs, alert.MitreTactics, alert.MitreTechniques,
		matchedFieldsJSON, alert.MatchedSelections, contextDataJSON,
		alert.Status, alert.Confidence, alert.FalsePositiveRisk,
		alert.MatchCount, alert.RelatedRules, alert.CombinedConfidence,
		alert.SeverityPromoted, alert.OriginalSeverity,
	).Scan(&alert.ID, &alert.CreatedAt, &alert.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create alert: %w", err)
	}

	return alert, nil
}

// GetByID retrieves an alert by its ID.
func (r *PostgresAlertRepository) GetByID(ctx context.Context, id string) (*Alert, error) {
	query := `
		SELECT id, timestamp, agent_id, rule_id, rule_title, severity, category,
			event_count, event_ids, mitre_tactics, mitre_techniques,
			matched_fields, matched_selections, context_data,
			status, assigned_to, resolution_notes,
			confidence, false_positive_risk,
			match_count, related_rules, combined_confidence,
			severity_promoted, original_severity,
			created_at, updated_at
		FROM sigma_alerts
		WHERE id = $1`

	return r.scanAlert(r.pool.QueryRow(ctx, query, id))
}

// List retrieves alerts matching the given filters.
func (r *PostgresAlertRepository) List(ctx context.Context, filters AlertFilters) ([]*Alert, int64, error) {
	// Build WHERE clause
	var conditions []string
	var args []interface{}
	argNum := 1

	if filters.AgentID != "" {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", argNum))
		args = append(args, filters.AgentID)
		argNum++
	}
	if filters.RuleID != "" {
		conditions = append(conditions, fmt.Sprintf("rule_id = $%d", argNum))
		args = append(args, filters.RuleID)
		argNum++
	}
	if len(filters.Severity) > 0 {
		conditions = append(conditions, fmt.Sprintf("severity = ANY($%d)", argNum))
		args = append(args, filters.Severity)
		argNum++
	}
	if len(filters.Status) > 0 {
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", argNum))
		args = append(args, filters.Status)
		argNum++
	}
	if !filters.DateFrom.IsZero() {
		conditions = append(conditions, fmt.Sprintf("timestamp >= $%d", argNum))
		args = append(args, filters.DateFrom)
		argNum++
	}
	if !filters.DateTo.IsZero() {
		conditions = append(conditions, fmt.Sprintf("timestamp <= $%d", argNum))
		args = append(args, filters.DateTo)
		argNum++
	}
	if filters.Search != "" {
		conditions = append(conditions, fmt.Sprintf("rule_title ILIKE $%d", argNum))
		args = append(args, "%"+filters.Search+"%")
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Sort
	sortBy := "timestamp"
	if filters.SortBy != "" {
		sortBy = filters.SortBy
	}
	sortOrder := "DESC"
	if filters.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	// Limit/Offset
	limit := 50
	if filters.Limit > 0 {
		limit = filters.Limit
	}
	offset := filters.Offset

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM sigma_alerts %s", whereClause)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	// Data query
	dataQuery := fmt.Sprintf(`
		SELECT id, timestamp, agent_id, rule_id, rule_title, severity, category,
			event_count, event_ids, mitre_tactics, mitre_techniques,
			matched_fields, matched_selections, context_data,
			status, assigned_to, resolution_notes,
			confidence, false_positive_risk,
			match_count, related_rules, combined_confidence,
			severity_promoted, original_severity,
			created_at, updated_at
		FROM sigma_alerts %s
		ORDER BY %s %s
		LIMIT %d OFFSET %d`,
		whereClause, sortBy, sortOrder, limit, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		alert, err := r.scanAlertRow(rows)
		if err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, alert)
	}

	return alerts, total, nil
}

// Update updates an existing alert.
func (r *PostgresAlertRepository) Update(ctx context.Context, id string, alert *Alert) (*Alert, error) {
	matchedFieldsJSON, _ := json.Marshal(alert.MatchedFields)
	contextDataJSON, _ := json.Marshal(alert.ContextData)

	query := `
		UPDATE sigma_alerts SET
			rule_title = $2, severity = $3, category = $4,
			event_count = $5, event_ids = $6,
			matched_fields = $7, context_data = $8,
			status = $9, assigned_to = $10, resolution_notes = $11,
			confidence = $12, false_positive_risk = $13
		WHERE id = $1
		RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query,
		id, alert.RuleTitle, alert.Severity, alert.Category,
		alert.EventCount, alert.EventIDs,
		matchedFieldsJSON, contextDataJSON,
		alert.Status, alert.AssignedTo, alert.ResolutionNotes,
		alert.Confidence, alert.FalsePositiveRisk,
	).Scan(&alert.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to update alert: %w", err)
	}

	alert.ID = id
	return alert, nil
}

// UpdateStatus updates just the status of an alert.
func (r *PostgresAlertRepository) UpdateStatus(ctx context.Context, id string, status string, notes string) error {
	query := `UPDATE sigma_alerts SET status = $2, resolution_notes = $3 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status, notes)
	return err
}

// Delete removes an alert from the database.
func (r *PostgresAlertRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM sigma_alerts WHERE id = $1", id)
	return err
}

// GetStats retrieves aggregate alert statistics.
func (r *PostgresAlertRepository) GetStats(ctx context.Context) (*AlertStats, error) {
	stats := &AlertStats{
		BySeverity: make(map[string]int64),
		ByStatus:   make(map[string]int64),
		ByRule:     make(map[string]int64),
		ByAgent:    make(map[string]int64),
	}

	// Total
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM sigma_alerts").Scan(&stats.TotalAlerts)

	// Last 24h
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM sigma_alerts WHERE timestamp > NOW() - INTERVAL '24 hours'").Scan(&stats.Last24Hours)

	// Last 7d
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM sigma_alerts WHERE timestamp > NOW() - INTERVAL '7 days'").Scan(&stats.Last7Days)

	// By severity
	rows, _ := r.pool.Query(ctx, "SELECT severity, COUNT(*) FROM sigma_alerts GROUP BY severity")
	for rows.Next() {
		var sev string
		var count int64
		rows.Scan(&sev, &count)
		stats.BySeverity[sev] = count
	}
	rows.Close()

	// By status
	rows, _ = r.pool.Query(ctx, "SELECT status, COUNT(*) FROM sigma_alerts GROUP BY status")
	for rows.Next() {
		var status string
		var count int64
		rows.Scan(&status, &count)
		stats.ByStatus[status] = count
	}
	rows.Close()

	return stats, nil
}

// FindRecent finds recent similar alerts for deduplication.
func (r *PostgresAlertRepository) FindRecent(ctx context.Context, agentID, ruleID string, since time.Time) (*Alert, error) {
	query := `
		SELECT id, timestamp, agent_id, rule_id, rule_title, severity, category,
			event_count, event_ids, mitre_tactics, mitre_techniques,
			matched_fields, matched_selections, context_data,
			status, assigned_to, resolution_notes,
			confidence, false_positive_risk,
			match_count, related_rules, combined_confidence,
			severity_promoted, original_severity,
			created_at, updated_at
		FROM sigma_alerts
		WHERE agent_id = $1 AND rule_id = $2 AND timestamp >= $3
		ORDER BY timestamp DESC
		LIMIT 1`

	alert, err := r.scanAlert(r.pool.QueryRow(ctx, query, agentID, ruleID, since))
	if err == pgx.ErrNoRows {
		return nil, nil // Not found is ok for dedup
	}
	return alert, err
}

// IncrementEventCount increments the event count for an existing alert.
func (r *PostgresAlertRepository) IncrementEventCount(ctx context.Context, id string, eventIDs []string) error {
	query := `
		UPDATE sigma_alerts 
		SET event_count = event_count + $2,
		    event_ids = event_ids || $3
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, len(eventIDs), eventIDs)
	return err
}

// BulkUpdateStatus updates the status of multiple alerts at once.
func (r *PostgresAlertRepository) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	query := `UPDATE sigma_alerts SET status = $1, updated_at = NOW() WHERE id = ANY($2)`
	_, err := r.pool.Exec(ctx, query, status, ids)
	return err
}

// Close closes the repository.
func (r *PostgresAlertRepository) Close() error {
	return nil // Pool is managed externally
}

// scanAlert scans a single alert from a QueryRow.
func (r *PostgresAlertRepository) scanAlert(row pgx.Row) (*Alert, error) {
	var alert Alert
	var matchedFieldsJSON, contextDataJSON []byte
	var assignedTo, resolutionNotes, originalSeverity *string

	err := row.Scan(
		&alert.ID, &alert.Timestamp, &alert.AgentID, &alert.RuleID, &alert.RuleTitle,
		&alert.Severity, &alert.Category, &alert.EventCount, &alert.EventIDs,
		&alert.MitreTactics, &alert.MitreTechniques,
		&matchedFieldsJSON, &alert.MatchedSelections, &contextDataJSON,
		&alert.Status, &assignedTo, &resolutionNotes,
		&alert.Confidence, &alert.FalsePositiveRisk,
		&alert.MatchCount, &alert.RelatedRules, &alert.CombinedConfidence,
		&alert.SeverityPromoted, &originalSeverity,
		&alert.CreatedAt, &alert.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal(matchedFieldsJSON, &alert.MatchedFields)
	json.Unmarshal(contextDataJSON, &alert.ContextData)
	if assignedTo != nil {
		alert.AssignedTo = *assignedTo
	}
	if resolutionNotes != nil {
		alert.ResolutionNotes = *resolutionNotes
	}
	if originalSeverity != nil {
		alert.OriginalSeverity = *originalSeverity
	}

	return &alert, nil
}

// scanAlertRow scans a single alert from Rows.
func (r *PostgresAlertRepository) scanAlertRow(rows pgx.Rows) (*Alert, error) {
	var alert Alert
	var matchedFieldsJSON, contextDataJSON []byte
	var assignedTo, resolutionNotes, originalSeverity *string

	err := rows.Scan(
		&alert.ID, &alert.Timestamp, &alert.AgentID, &alert.RuleID, &alert.RuleTitle,
		&alert.Severity, &alert.Category, &alert.EventCount, &alert.EventIDs,
		&alert.MitreTactics, &alert.MitreTechniques,
		&matchedFieldsJSON, &alert.MatchedSelections, &contextDataJSON,
		&alert.Status, &assignedTo, &resolutionNotes,
		&alert.Confidence, &alert.FalsePositiveRisk,
		&alert.MatchCount, &alert.RelatedRules, &alert.CombinedConfidence,
		&alert.SeverityPromoted, &originalSeverity,
		&alert.CreatedAt, &alert.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal(matchedFieldsJSON, &alert.MatchedFields)
	json.Unmarshal(contextDataJSON, &alert.ContextData)
	if assignedTo != nil {
		alert.AssignedTo = *assignedTo
	}
	if resolutionNotes != nil {
		alert.ResolutionNotes = *resolutionNotes
	}
	if originalSeverity != nil {
		alert.OriginalSeverity = *originalSeverity
	}

	return &alert, nil
}

// Ensure interface compliance
var _ AlertRepository = (*PostgresAlertRepository)(nil)

func init() {
	_ = logger.Info // Reference logger to avoid unused import
}
