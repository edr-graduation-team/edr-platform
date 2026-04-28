package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventRow is the event row returned by search/list APIs.
// Raw is included so callers can extract typed fields (action, name, pid, etc.).
type EventRow struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	EventType string
	Severity  string
	Timestamp time.Time
	Summary   string
	Raw       json.RawMessage
}

// EventInsert is an insert payload (parsed/normalized from ingestion).
type EventInsert struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	BatchID   *uuid.UUID
	EventType string
	Severity  string
	Timestamp time.Time
	Summary   string
	Raw       json.RawMessage
}

// EventSearchFilter matches API EventFilter shape.
type EventSearchFilter struct {
	Field    string
	Operator string
	Value    interface{}
}

// EventSearchRequest matches API EventSearchRequest (without importing api package).
type EventSearchRequest struct {
	Filters  []EventSearchFilter
	Logic    string // AND/OR
	TimeFrom time.Time
	TimeTo   time.Time
	Limit    int
	Offset   int
}

// EventDetail is one row from `events` including JSONB raw.
type EventDetail struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	EventType string
	Severity  string
	Timestamp time.Time
	Summary   string
	Raw       json.RawMessage
}

type EventRepository interface {
	InsertMany(ctx context.Context, rows []EventInsert) error
	Search(ctx context.Context, req EventSearchRequest) ([]EventRow, int, error)
	ListByAgent(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]EventRow, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*EventDetail, error)
}

type PostgresEventRepository struct {
	db *pgxpool.Pool
}

func NewPostgresEventRepository(db *pgxpool.Pool) *PostgresEventRepository {
	return &PostgresEventRepository{db: db}
}

func (r *PostgresEventRepository) InsertMany(ctx context.Context, rows []EventInsert) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	b := &strings.Builder{}
	args := make([]interface{}, 0, len(rows)*8)
	b.WriteString("INSERT INTO events (id, agent_id, batch_id, event_type, severity, ts, summary, raw) VALUES ")
	for i, e := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		// 8 cols per row
		p := i*8 + 1
		fmt.Fprintf(b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", p, p+1, p+2, p+3, p+4, p+5, p+6, p+7)
		args = append(args, e.ID, e.AgentID, e.BatchID, e.EventType, e.Severity, e.Timestamp, e.Summary, e.Raw)
	}
	b.WriteString(" ON CONFLICT (id) DO NOTHING")

	if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
		return fmt.Errorf("insert events: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *PostgresEventRepository) ListByAgent(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]EventRow, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM events WHERE agent_id=$1", agentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	q := `
		SELECT id, agent_id, event_type, severity, ts, summary, raw
		FROM events
		WHERE agent_id = $1
		ORDER BY ts DESC
		LIMIT $2 OFFSET $3`
	rs, err := r.db.Query(ctx, q, agentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}
	defer rs.Close()

	out := []EventRow{}
	for rs.Next() {
		var e EventRow
		if err := rs.Scan(&e.ID, &e.AgentID, &e.EventType, &e.Severity, &e.Timestamp, &e.Summary, &e.Raw); err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, total, nil
}

func (r *PostgresEventRepository) Search(ctx context.Context, req EventSearchRequest) ([]EventRow, int, error) {
	limit := req.Limit
	offset := req.Offset
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		limit = 10000
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"ts >= $1", "ts <= $2"}
	args := []interface{}{req.TimeFrom, req.TimeTo}

	add := func(clause string, v interface{}) {
		where = append(where, clause)
		args = append(args, v)
	}

	for _, f := range req.Filters {
		field := strings.TrimSpace(strings.ToLower(f.Field))
		op := strings.TrimSpace(strings.ToLower(f.Operator))
		argPos := len(args) + 1

		switch field {
		// ── Indexed top-level columns ─────────────────────────────────────
		case "agent_id", "event_type", "severity":
			switch op {
			case "equals":
				add(fmt.Sprintf("%s = $%d", field, argPos), f.Value)
			case "contains":
				add(fmt.Sprintf("%s ILIKE $%d", field, argPos), "%"+fmt.Sprint(f.Value)+"%")
			case "regex":
				add(fmt.Sprintf("%s ~* $%d", field, argPos), fmt.Sprint(f.Value))
			}

		// ── JSONB raw fields (data.*) ─────────────────────────────────────
		// Supported keys: data.autonomous, data.action, data.name, data.matched_rule_id, etc.
		default:
			if !strings.HasPrefix(field, "data.") {
				continue
			}
			key := strings.TrimPrefix(field, "data.")
			// Whitelist keys we allow for JSONB extraction (safety guard)
			allowedJSONB := map[string]bool{
				"autonomous": true, "action": true, "name": true,
				"matched_rule_id": true, "matched_rule_title": true,
				"response_action": true, "decision_mode": true,
				"pid": true, "user_name": true,
			}
			if !allowedJSONB[key] {
				continue
			}
			jsonPath := fmt.Sprintf("raw->>'%s'", key)
			switch op {
			case "equals":
				val := fmt.Sprint(f.Value)
				if b, ok := f.Value.(bool); ok {
					// boolean stored as JSON "true"/"false" string in JSONB text extraction
					if b {
						val = "true"
					} else {
						val = "false"
					}
				}
				add(fmt.Sprintf("%s = $%d", jsonPath, argPos), val)
			case "contains":
				add(fmt.Sprintf("%s ILIKE $%d", jsonPath, argPos), "%"+fmt.Sprint(f.Value)+"%")
			case "in":
				// Value should be a []string or []interface{}
				// Render as: raw->>'action' = ANY($n::text[])
				var vals []string
				switch v := f.Value.(type) {
				case []string:
					vals = v
				case []interface{}:
					for _, vi := range v {
						vals = append(vals, fmt.Sprint(vi))
					}
				}
				if len(vals) > 0 {
					add(fmt.Sprintf("%s = ANY($%d::text[])", jsonPath, argPos), vals)
				}
			}
		}
	}

	joiner := " AND "
	if strings.EqualFold(req.Logic, "OR") {
		if len(where) > 2 {
			head := where[:2]
			user := where[2:]
			where = []string{strings.Join(head, " AND "), "(" + strings.Join(user, " OR ") + ")"}
			joiner = " AND "
		}
	}

	w := strings.Join(where, joiner)

	countQ := "SELECT COUNT(1) FROM events WHERE " + w
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search: %w", err)
	}

	dataQ := `
		SELECT id, agent_id, event_type, severity, ts, summary, raw
		FROM events
		WHERE ` + w + `
		ORDER BY ts DESC
		LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, limit, offset)

	rs, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search events: %w", err)
	}
	defer rs.Close()

	out := []EventRow{}
	for rs.Next() {
		var e EventRow
		if err := rs.Scan(&e.ID, &e.AgentID, &e.EventType, &e.Severity, &e.Timestamp, &e.Summary, &e.Raw); err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, total, nil
}

func (r *PostgresEventRepository) GetByID(ctx context.Context, id uuid.UUID) (*EventDetail, error) {
	const q = `
		SELECT id, agent_id, event_type, severity, ts, summary, raw
		FROM events
		WHERE id = $1`
	var d EventDetail
	err := r.db.QueryRow(ctx, q, id).Scan(
		&d.ID, &d.AgentID, &d.EventType, &d.Severity, &d.Timestamp, &d.Summary, &d.Raw,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get event: %w", err)
	}
	return &d, nil
}


