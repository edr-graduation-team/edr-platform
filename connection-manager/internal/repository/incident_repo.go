package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain types
// ─────────────────────────────────────────────────────────────────────────────

type PlaybookRun struct {
	ID         int64          `json:"id"`
	AgentID    uuid.UUID      `json:"agent_id"`
	Playbook   string         `json:"playbook"`
	Trigger    string         `json:"trigger"`
	Status     string         `json:"status"` // running | completed | partial | failed
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Summary    map[string]any `json:"summary,omitempty"`
}

type PlaybookStep struct {
	ID          int64      `json:"id"`
	RunID       int64      `json:"run_id"`
	StepName    string     `json:"step_name"`
	CommandType string     `json:"command_type"`
	Status      string     `json:"status"` // pending | running | success | failed | skipped
	CommandID   *uuid.UUID `json:"command_id,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type TriageSnapshot struct {
	ID        int64          `json:"id"`
	AgentID   uuid.UUID      `json:"agent_id"`
	RunID     *int64         `json:"run_id,omitempty"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type IocEnrichment struct {
	ID        int64          `json:"id"`
	AgentID   *uuid.UUID     `json:"agent_id,omitempty"`
	RunID     *int64         `json:"run_id,omitempty"`
	IocType   string         `json:"ioc_type"`  // hash | ip | domain
	IocValue  string         `json:"ioc_value"`
	Provider  string         `json:"provider"`  // virustotal | abuseipdb | otx
	Verdict   string         `json:"verdict"`   // clean | suspicious | malicious | unknown
	Score     int            `json:"score"`
	Raw       map[string]any `json:"raw,omitempty"`
	FetchedAt time.Time      `json:"fetched_at"`
}

// IncidentSummary aggregates everything for a single agent's active incident.
type IncidentSummary struct {
	Run       *PlaybookRun    `json:"run"`
	Steps     []PlaybookStep  `json:"steps"`
	Snapshots []TriageSnapshot `json:"snapshots"`
	Iocs      []IocEnrichment `json:"iocs"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Repository interface
// ─────────────────────────────────────────────────────────────────────────────

type IncidentRepository interface {
	// Playbook runs
	CreateRun(ctx context.Context, run *PlaybookRun) (int64, error)
	GetLatestRun(ctx context.Context, agentID uuid.UUID) (*PlaybookRun, error)
	ListRuns(ctx context.Context, agentID uuid.UUID, limit int) ([]PlaybookRun, error)
	UpdateRunStatus(ctx context.Context, id int64, status string, summary map[string]any) error
	FinishRun(ctx context.Context, id int64, status string) error

	// Playbook steps
	CreateStep(ctx context.Context, step *PlaybookStep) (int64, error)
	UpdateStep(ctx context.Context, id int64, status string, commandID *uuid.UUID, errMsg string) error
	ListSteps(ctx context.Context, runID int64) ([]PlaybookStep, error)
	GetStepByCommandID(ctx context.Context, commandID uuid.UUID) (*PlaybookStep, error)

	// Triage snapshots
	UpsertSnapshot(ctx context.Context, snap *TriageSnapshot) error
	ListSnapshots(ctx context.Context, agentID uuid.UUID, kinds []string) ([]TriageSnapshot, error)

	// IOC enrichment
	UpsertIoc(ctx context.Context, ioc *IocEnrichment) error
	ListIocs(ctx context.Context, agentID uuid.UUID, limit int) ([]IocEnrichment, error)

	// Aggregate
	GetIncidentSummary(ctx context.Context, agentID uuid.UUID) (*IncidentSummary, error)

	// Incident lifecycle actions
	MarkFalsePositive(ctx context.Context, agentID uuid.UUID) error
	EscalateRun(ctx context.Context, agentID uuid.UUID) error
}

// ─────────────────────────────────────────────────────────────────────────────
// PostgreSQL implementation
// ─────────────────────────────────────────────────────────────────────────────

type PostgresIncidentRepository struct {
	db *pgxpool.Pool
}

func NewPostgresIncidentRepository(db *pgxpool.Pool) *PostgresIncidentRepository {
	return &PostgresIncidentRepository{db: db}
}

// ── Playbook Runs ────────────────────────────────────────────────────────────

func (r *PostgresIncidentRepository) CreateRun(ctx context.Context, run *PlaybookRun) (int64, error) {
	summaryJSON, _ := json.Marshal(run.Summary)
	if summaryJSON == nil {
		summaryJSON = []byte("{}")
	}
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO playbook_runs (agent_id, playbook, trigger, status, started_at, summary)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		run.AgentID, run.Playbook, run.Trigger, run.Status, run.StartedAt, summaryJSON,
	).Scan(&id)
	return id, err
}

func (r *PostgresIncidentRepository) GetLatestRun(ctx context.Context, agentID uuid.UUID) (*PlaybookRun, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, agent_id, playbook, trigger, status, started_at, finished_at, summary
		 FROM playbook_runs WHERE agent_id=$1 ORDER BY started_at DESC LIMIT 1`,
		agentID,
	)
	return scanRun(row)
}

func (r *PostgresIncidentRepository) ListRuns(ctx context.Context, agentID uuid.UUID, limit int) ([]PlaybookRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, agent_id, playbook, trigger, status, started_at, finished_at, summary
		 FROM playbook_runs WHERE agent_id=$1 ORDER BY started_at DESC LIMIT $2`,
		agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlaybookRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *PostgresIncidentRepository) UpdateRunStatus(ctx context.Context, id int64, status string, summary map[string]any) error {
	summaryJSON, _ := json.Marshal(summary)
	if summaryJSON == nil {
		summaryJSON = []byte("{}")
	}
	_, err := r.db.Exec(ctx,
		`UPDATE playbook_runs SET status=$1, summary=$2 WHERE id=$3`,
		status, summaryJSON, id,
	)
	return err
}

func (r *PostgresIncidentRepository) FinishRun(ctx context.Context, id int64, status string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE playbook_runs SET status=$1, finished_at=now() WHERE id=$2`,
		status, id,
	)
	return err
}

// ── Playbook Steps ───────────────────────────────────────────────────────────

func (r *PostgresIncidentRepository) CreateStep(ctx context.Context, step *PlaybookStep) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO playbook_steps (run_id, step_name, command_type, status)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		step.RunID, step.StepName, step.CommandType, step.Status,
	).Scan(&id)
	return id, err
}

func (r *PostgresIncidentRepository) UpdateStep(ctx context.Context, id int64, status string, commandID *uuid.UUID, errMsg string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE playbook_steps SET status=$1, command_id=$2, error=$3,
		 started_at=CASE WHEN status='running' THEN now() ELSE started_at END,
		 finished_at=CASE WHEN $1 IN ('success','failed','skipped') THEN now() ELSE finished_at END
		 WHERE id=$4`,
		status, commandID, errMsg, id,
	)
	return err
}

func (r *PostgresIncidentRepository) ListSteps(ctx context.Context, runID int64) ([]PlaybookStep, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, run_id, step_name, command_type, status, command_id, started_at, finished_at, error
		 FROM playbook_steps WHERE run_id=$1 ORDER BY id`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlaybookStep
	for rows.Next() {
		var s PlaybookStep
		if err := rows.Scan(&s.ID, &s.RunID, &s.StepName, &s.CommandType, &s.Status,
			&s.CommandID, &s.StartedAt, &s.FinishedAt, &s.Error); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresIncidentRepository) GetStepByCommandID(ctx context.Context, commandID uuid.UUID) (*PlaybookStep, error) {
	var s PlaybookStep
	err := r.db.QueryRow(ctx,
		`SELECT id, run_id, step_name, command_type, status, command_id, started_at, finished_at, error
		 FROM playbook_steps WHERE command_id=$1 LIMIT 1`,
		commandID,
	).Scan(&s.ID, &s.RunID, &s.StepName, &s.CommandType, &s.Status,
		&s.CommandID, &s.StartedAt, &s.FinishedAt, &s.Error)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ── Triage Snapshots ─────────────────────────────────────────────────────────

func (r *PostgresIncidentRepository) UpsertSnapshot(ctx context.Context, snap *TriageSnapshot) error {
	payloadJSON, _ := json.Marshal(snap.Payload)
	if payloadJSON == nil {
		payloadJSON = []byte("{}")
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO triage_snapshots (agent_id, run_id, kind, payload)
		 VALUES ($1, $2, $3, $4)`,
		snap.AgentID, snap.RunID, snap.Kind, payloadJSON,
	)
	return err
}

func (r *PostgresIncidentRepository) ListSnapshots(ctx context.Context, agentID uuid.UUID, kinds []string) ([]TriageSnapshot, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close(); Err() error }
	var err error

	if len(kinds) == 0 {
		rows, err = r.db.Query(ctx,
			`SELECT id, agent_id, run_id, kind, payload, created_at
			 FROM triage_snapshots WHERE agent_id=$1 ORDER BY created_at DESC LIMIT 50`,
			agentID,
		)
	} else {
		rows, err = r.db.Query(ctx,
			`SELECT id, agent_id, run_id, kind, payload, created_at
			 FROM triage_snapshots WHERE agent_id=$1 AND kind=ANY($2) ORDER BY created_at DESC LIMIT 50`,
			agentID, kinds,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TriageSnapshot
	for rows.Next() {
		var s TriageSnapshot
		var payloadJSON []byte
		if err := rows.Scan(&s.ID, &s.AgentID, &s.RunID, &s.Kind, &payloadJSON, &s.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payloadJSON, &s.Payload)
		if s.Payload == nil {
			s.Payload = map[string]any{}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ── IOC Enrichment ───────────────────────────────────────────────────────────

func (r *PostgresIncidentRepository) UpsertIoc(ctx context.Context, ioc *IocEnrichment) error {
	rawJSON, _ := json.Marshal(ioc.Raw)
	if rawJSON == nil {
		rawJSON = []byte("null")
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO ioc_enrichment (agent_id, run_id, ioc_type, ioc_value, provider, verdict, score, raw, fetched_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 ON CONFLICT (ioc_type, ioc_value, provider)
		 DO UPDATE SET verdict=EXCLUDED.verdict, score=EXCLUDED.score, raw=EXCLUDED.raw, fetched_at=now()`,
		ioc.AgentID, ioc.RunID, ioc.IocType, ioc.IocValue, ioc.Provider,
		ioc.Verdict, ioc.Score, rawJSON,
	)
	return err
}

func (r *PostgresIncidentRepository) ListIocs(ctx context.Context, agentID uuid.UUID, limit int) ([]IocEnrichment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, agent_id, run_id, ioc_type, ioc_value, provider, verdict, score, raw, fetched_at
		 FROM ioc_enrichment WHERE agent_id=$1 ORDER BY fetched_at DESC LIMIT $2`,
		agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IocEnrichment
	for rows.Next() {
		var e IocEnrichment
		var rawJSON []byte
		if err := rows.Scan(&e.ID, &e.AgentID, &e.RunID, &e.IocType, &e.IocValue,
			&e.Provider, &e.Verdict, &e.Score, &rawJSON, &e.FetchedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(rawJSON, &e.Raw)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Aggregate ────────────────────────────────────────────────────────────────

func (r *PostgresIncidentRepository) GetIncidentSummary(ctx context.Context, agentID uuid.UUID) (*IncidentSummary, error) {
	summary := &IncidentSummary{}

	run, err := r.GetLatestRun(ctx, agentID)
	if err == nil {
		summary.Run = run
		if steps, err := r.ListSteps(ctx, run.ID); err == nil {
			summary.Steps = steps
		}
	}

	if snaps, err := r.ListSnapshots(ctx, agentID, nil); err == nil {
		summary.Snapshots = snaps
	}

	if iocs, err := r.ListIocs(ctx, agentID, 100); err == nil {
		summary.Iocs = iocs
	}

	return summary, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...any) error
}

func scanRun(row scannable) (*PlaybookRun, error) {
	var run PlaybookRun
	var summaryJSON []byte
	if err := row.Scan(&run.ID, &run.AgentID, &run.Playbook, &run.Trigger,
		&run.Status, &run.StartedAt, &run.FinishedAt, &summaryJSON); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(summaryJSON, &run.Summary)
	if run.Summary == nil {
		run.Summary = map[string]any{}
	}
	return &run, nil
}

// ── Incident Lifecycle ───────────────────────────────────────────────────────

// MarkFalsePositive sets the latest run's status to 'false_positive'.
func (r *PostgresIncidentRepository) MarkFalsePositive(ctx context.Context, agentID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE playbook_runs
		SET    status = 'false_positive', finished_at = now()
		WHERE  id = (
			SELECT id FROM playbook_runs
			WHERE  agent_id = $1
			ORDER  BY started_at DESC
			LIMIT  1
		)`, agentID)
	return err
}

// EscalateRun merges escalated=true into the latest run's summary JSONB.
func (r *PostgresIncidentRepository) EscalateRun(ctx context.Context, agentID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE playbook_runs
		SET    summary = summary || '{"escalated": true}'::jsonb
		WHERE  id = (
			SELECT id FROM playbook_runs
			WHERE  agent_id = $1
			ORDER  BY started_at DESC
			LIMIT  1
		)`, agentID)
	return err
}
