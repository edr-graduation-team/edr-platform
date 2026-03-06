// Package repository provides Alert repository implementation.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// AlertRepository defines the interface for alert storage.
type AlertRepository interface {
	Create(ctx context.Context, alert *models.Alert) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error)
	Update(ctx context.Context, alert *models.Alert) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter AlertFilter) ([]*models.Alert, int, error)
	GetStats(ctx context.Context) (*AlertStats, error)
}

// AlertFilter for querying alerts.
type AlertFilter struct {
	AgentID    *uuid.UUID
	Status     []models.AlertStatus
	Severity   []models.AlertSeverity
	AssignedTo *uuid.UUID
	RuleID     string
	FromTime   *time.Time
	ToTime     *time.Time
	Search     string
	Limit      int
	Offset     int
	SortBy     string
	SortOrder  string
}

// AlertStats contains alert statistics.
type AlertStats struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	InProgress int            `json:"in_progress"`
	Resolved   int            `json:"resolved"`
	BySeverity map[string]int `json:"by_severity"`
}

// PostgresAlertRepository implements AlertRepository using PostgreSQL.
type PostgresAlertRepository struct {
	db *pgxpool.Pool
}

// NewPostgresAlertRepository creates a new alert repository.
func NewPostgresAlertRepository(db *pgxpool.Pool) *PostgresAlertRepository {
	return &PostgresAlertRepository{db: db}
}

// Create inserts a new alert.
func (r *PostgresAlertRepository) Create(ctx context.Context, alert *models.Alert) error {
	if alert.ID == uuid.Nil {
		alert.ID = uuid.New()
	}
	if alert.DetectedAt.IsZero() {
		alert.DetectedAt = time.Now()
	}

	query := `
		INSERT INTO alerts (
			id, severity, title, description, agent_id, rule_id, rule_name,
			status, assigned_to, event_count, first_event_at, last_event_at,
			detected_at, tags, metadata, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		alert.ID, alert.Severity, alert.Title, alert.Description,
		alert.AgentID, alert.RuleID, alert.RuleName, alert.Status,
		alert.AssignedTo, alert.EventCount, alert.FirstEventAt,
		alert.LastEventAt, alert.DetectedAt, alert.Tags, alert.Metadata, alert.Notes,
	).Scan(&alert.CreatedAt, &alert.UpdatedAt)
}

// GetByID retrieves an alert by ID.
func (r *PostgresAlertRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	query := `
		SELECT id, severity, title, description, agent_id, rule_id, rule_name,
			status, assigned_to, resolution, resolution_notes, event_count,
			first_event_at, last_event_at, detected_at, acknowledged_at,
			resolved_at, created_at, updated_at, tags, metadata, notes
		FROM alerts WHERE id = $1`

	alert := &models.Alert{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&alert.ID, &alert.Severity, &alert.Title, &alert.Description,
		&alert.AgentID, &alert.RuleID, &alert.RuleName, &alert.Status,
		&alert.AssignedTo, &alert.Resolution, &alert.ResolutionNotes,
		&alert.EventCount, &alert.FirstEventAt, &alert.LastEventAt,
		&alert.DetectedAt, &alert.AcknowledgedAt, &alert.ResolvedAt,
		&alert.CreatedAt, &alert.UpdatedAt, &alert.Tags, &alert.Metadata, &alert.Notes,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return alert, err
}

// Update updates an existing alert.
func (r *PostgresAlertRepository) Update(ctx context.Context, alert *models.Alert) error {
	query := `
		UPDATE alerts SET
			severity = $2, title = $3, description = $4, status = $5,
			assigned_to = $6, resolution = $7, resolution_notes = $8,
			event_count = $9, acknowledged_at = $10, resolved_at = $11,
			tags = $12, metadata = $13, notes = $14
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		alert.ID, alert.Severity, alert.Title, alert.Description, alert.Status,
		alert.AssignedTo, alert.Resolution, alert.ResolutionNotes,
		alert.EventCount, alert.AcknowledgedAt, alert.ResolvedAt,
		alert.Tags, alert.Metadata, alert.Notes,
	).Scan(&alert.UpdatedAt)

	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	return err
}

// Delete removes an alert.
func (r *PostgresAlertRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, "DELETE FROM alerts WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List retrieves alerts with filtering.
func (r *PostgresAlertRepository) List(ctx context.Context, filter AlertFilter) ([]*models.Alert, int, error) {
	// Build query
	baseQuery := "FROM alerts WHERE 1=1"
	var args []interface{}
	argNum := 1

	if filter.AgentID != nil {
		baseQuery += fmt.Sprintf(" AND agent_id = $%d", argNum)
		args = append(args, *filter.AgentID)
		argNum++
	}

	if len(filter.Status) > 0 {
		baseQuery += fmt.Sprintf(" AND status = ANY($%d)", argNum)
		args = append(args, filter.Status)
		argNum++
	}

	if len(filter.Severity) > 0 {
		baseQuery += fmt.Sprintf(" AND severity = ANY($%d)", argNum)
		args = append(args, filter.Severity)
		argNum++
	}

	if filter.Search != "" {
		baseQuery += fmt.Sprintf(" AND (title ILIKE $%d OR description ILIKE $%d)", argNum, argNum)
		args = append(args, "%"+filter.Search+"%")
		argNum++
	}

	// Count total
	var total int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Add sorting and pagination
	sortBy := "detected_at"
	sortOrder := "DESC"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	if filter.SortOrder != "" {
		sortOrder = filter.SortOrder
	}

	query := fmt.Sprintf(`
		SELECT id, severity, title, description, agent_id, rule_id, rule_name,
			status, assigned_to, event_count, detected_at, created_at, updated_at
		%s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		baseQuery, sortBy, sortOrder, argNum, argNum+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var alerts []*models.Alert
	for rows.Next() {
		alert := &models.Alert{}
		if err := rows.Scan(
			&alert.ID, &alert.Severity, &alert.Title, &alert.Description,
			&alert.AgentID, &alert.RuleID, &alert.RuleName, &alert.Status,
			&alert.AssignedTo, &alert.EventCount, &alert.DetectedAt,
			&alert.CreatedAt, &alert.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, alert)
	}

	return alerts, total, nil
}

// GetStats returns alert statistics.
func (r *PostgresAlertRepository) GetStats(ctx context.Context) (*AlertStats, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'open') as open,
			COUNT(*) FILTER (WHERE status = 'in_progress') as in_progress,
			COUNT(*) FILTER (WHERE status = 'resolved') as resolved
		FROM alerts`

	stats := &AlertStats{BySeverity: make(map[string]int)}
	err := r.db.QueryRow(ctx, query).Scan(
		&stats.Total, &stats.Open, &stats.InProgress, &stats.Resolved,
	)
	if err != nil {
		return nil, err
	}

	// Get by severity
	rows, err := r.db.Query(ctx, "SELECT severity, COUNT(*) FROM alerts GROUP BY severity")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sev string
		var count int
		if err := rows.Scan(&sev, &count); err != nil {
			return nil, err
		}
		stats.BySeverity[sev] = count
	}

	return stats, nil
}
