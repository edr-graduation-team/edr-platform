package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ForensicCollectionSummary struct {
	CommandID    uuid.UUID              `json:"command_id"`
	AgentID      uuid.UUID              `json:"agent_id"`
	CommandType  string                 `json:"command_type"`
	IssuedAt     time.Time              `json:"issued_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	TimeRange    string                 `json:"time_range,omitempty"`
	LogTypes     string                 `json:"log_types,omitempty"`
	Summary      map[string]any         `json:"summary"`
}

type ForensicEventRow struct {
	ID       int64           `json:"id"`
	Timestamp *time.Time     `json:"timestamp,omitempty"`
	LogType  string          `json:"log_type"`
	EventID  string          `json:"event_id,omitempty"`
	Level    string          `json:"level,omitempty"`
	Provider string          `json:"provider,omitempty"`
	Message  string          `json:"message,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

type PostgresForensicRepository struct {
	db *pgxpool.Pool
}

func NewPostgresForensicRepository(db *pgxpool.Pool) *PostgresForensicRepository {
	return &PostgresForensicRepository{db: db}
}

func (r *PostgresForensicRepository) ListCollectionsByAgent(ctx context.Context, agentID uuid.UUID, limit int) ([]ForensicCollectionSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
		SELECT command_id, agent_id, command_type, issued_at, completed_at, time_range, log_types, summary
		FROM forensic_collections
		WHERE agent_id = $1
		ORDER BY issued_at DESC
		LIMIT $2`
	rows, err := r.db.Query(ctx, q, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ForensicCollectionSummary
	for rows.Next() {
		var c ForensicCollectionSummary
		var summaryJSON []byte
		if err := rows.Scan(&c.CommandID, &c.AgentID, &c.CommandType, &c.IssuedAt, &c.CompletedAt, &c.TimeRange, &c.LogTypes, &summaryJSON); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(summaryJSON, &c.Summary)
		if c.Summary == nil {
			c.Summary = map[string]any{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresForensicRepository) UpsertCollection(ctx context.Context, c ForensicCollectionSummary) error {
	summaryJSON, _ := json.Marshal(c.Summary)
	if summaryJSON == nil {
		summaryJSON = []byte("{}")
	}
	const q = `
		INSERT INTO forensic_collections (command_id, agent_id, command_type, issued_at, completed_at, time_range, log_types, summary)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
		ON CONFLICT (command_id) DO UPDATE SET
			completed_at = EXCLUDED.completed_at,
			time_range = EXCLUDED.time_range,
			log_types = EXCLUDED.log_types,
			summary = EXCLUDED.summary`
	_, err := r.db.Exec(ctx, q, c.CommandID, c.AgentID, c.CommandType, c.IssuedAt, c.CompletedAt, c.TimeRange, c.LogTypes, summaryJSON)
	return err
}

func (r *PostgresForensicRepository) ReplaceEvents(ctx context.Context, agentID, commandID uuid.UUID, logType string, events []ForensicEventRow) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM forensic_events WHERE command_id=$1 AND log_type=$2`, commandID, logType); err != nil {
		return err
	}

	const ins = `
		INSERT INTO forensic_events (command_id, agent_id, log_type, ts, event_id, level, provider, message, raw)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`

	for _, e := range events {
		raw := []byte(nil)
		if len(e.Raw) > 0 {
			raw = e.Raw
		} else {
			raw = []byte("null")
		}
		if _, err := tx.Exec(ctx, ins, commandID, agentID, logType, e.Timestamp, nullIfEmpty(e.EventID), nullIfEmpty(e.Level), nullIfEmpty(e.Provider), nullIfEmpty(e.Message), raw); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresForensicRepository) ListEvents(ctx context.Context, agentID, commandID uuid.UUID, logType string, limit int, cursorID *int64) ([]ForensicEventRow, *int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{agentID, commandID, logType}
	whereCursor := ""
	if cursorID != nil && *cursorID > 0 {
		whereCursor = "AND id > $4"
		args = append(args, *cursorID)
	}
	args = append(args, limit+1)

	q := fmt.Sprintf(`
		SELECT id, ts, log_type, event_id, level, provider, message, raw
		FROM forensic_events
		WHERE agent_id=$1 AND command_id=$2 AND log_type=$3
		%s
		ORDER BY id
		LIMIT $%d`, whereCursor, len(args))

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []ForensicEventRow
	for rows.Next() {
		var e ForensicEventRow
		var raw []byte
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.LogType, &e.EventID, &e.Level, &e.Provider, &e.Message, &raw); err != nil {
			return nil, nil, err
		}
		e.Raw = raw
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *int64
	if len(out) > limit {
		last := out[limit-1].ID
		next = &last
		out = out[:limit]
	}
	return out, next, nil
}

func nullIfEmpty(s string) *string {
	ss := s
	if ss == "" {
		return nil
	}
	return &ss
}

