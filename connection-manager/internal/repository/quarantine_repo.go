package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// PostgresQuarantineRepository implements QuarantineRepository.
type PostgresQuarantineRepository struct {
	db *pgxpool.Pool
}

// NewPostgresQuarantineRepository creates a quarantine inventory repository.
func NewPostgresQuarantineRepository(db *pgxpool.Pool) *PostgresQuarantineRepository {
	return &PostgresQuarantineRepository{db: db}
}

// Upsert inserts or updates a quarantine row keyed by (agent_id, quarantine_path).
func (r *PostgresQuarantineRepository) Upsert(ctx context.Context, row *models.QuarantineItem) error {
	if row == nil {
		return fmt.Errorf("nil row")
	}
	if row.State == "" {
		row.State = models.QuarantineStateQuarantined
	}
	if row.Source == "" {
		row.Source = "auto_responder"
	}
	q := `
INSERT INTO agent_quarantine_items (
  agent_id, event_id, original_path, quarantine_path, sha256, threat_name, source, state
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (agent_id, quarantine_path) DO UPDATE SET
  original_path = EXCLUDED.original_path,
  event_id = COALESCE(NULLIF(EXCLUDED.event_id,''), agent_quarantine_items.event_id),
  sha256 = COALESCE(NULLIF(EXCLUDED.sha256,''), agent_quarantine_items.sha256),
  threat_name = COALESCE(NULLIF(EXCLUDED.threat_name,''), agent_quarantine_items.threat_name),
  source = CASE WHEN agent_quarantine_items.source = 'manual_c2' THEN agent_quarantine_items.source ELSE EXCLUDED.source END,
  state = agent_quarantine_items.state,
  updated_at = NOW()
`
	_, err := r.db.Exec(ctx, q,
		row.AgentID, row.EventID, row.OriginalPath, row.QuarantinePath,
		row.SHA256, row.ThreatName, row.Source, string(row.State),
	)
	return err
}

// ListByAgent returns inventory rows for an endpoint.
func (r *PostgresQuarantineRepository) ListByAgent(ctx context.Context, agentID uuid.UUID, includeResolved bool) ([]*models.QuarantineItem, error) {
	var q string
	if includeResolved {
		q = `
SELECT id, agent_id, event_id, original_path, quarantine_path, sha256, threat_name, source, state, created_at, updated_at
FROM agent_quarantine_items WHERE agent_id = $1 ORDER BY updated_at DESC`
	} else {
		q = `
SELECT id, agent_id, event_id, original_path, quarantine_path, sha256, threat_name, source, state, created_at, updated_at
FROM agent_quarantine_items
WHERE agent_id = $1 AND state NOT IN ('restored','deleted')
ORDER BY updated_at DESC`
	}
	rows, err := r.db.Query(ctx, q, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.QuarantineItem
	for rows.Next() {
		var it models.QuarantineItem
		if err := rows.Scan(
			&it.ID, &it.AgentID, &it.EventID, &it.OriginalPath, &it.QuarantinePath,
			&it.SHA256, &it.ThreatName, &it.Source, &it.State, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}

// GetByID returns one row.
func (r *PostgresQuarantineRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.QuarantineItem, error) {
	q := `
SELECT id, agent_id, event_id, original_path, quarantine_path, sha256, threat_name, source, state, created_at, updated_at
FROM agent_quarantine_items WHERE id = $1`
	var it models.QuarantineItem
	err := r.db.QueryRow(ctx, q, id).Scan(
		&it.ID, &it.AgentID, &it.EventID, &it.OriginalPath, &it.QuarantinePath,
		&it.SHA256, &it.ThreatName, &it.Source, &it.State, &it.CreatedAt, &it.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// SetState updates analyst/lifecycle state.
func (r *PostgresQuarantineRepository) SetState(ctx context.Context, id uuid.UUID, state models.QuarantineItemState) error {
	_, err := r.db.Exec(ctx, `
UPDATE agent_quarantine_items SET state = $1, updated_at = NOW() WHERE id = $2`,
		string(state), id,
	)
	return err
}
