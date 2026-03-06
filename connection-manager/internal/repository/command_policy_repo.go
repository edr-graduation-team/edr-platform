// Package repository provides Command repository implementation.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// CommandRepository defines the interface for command storage.
type CommandRepository interface {
	Create(ctx context.Context, cmd *models.Command) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Command, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.CommandStatus, result map[string]any, errorMsg string) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByAgent(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]*models.Command, int, error)
	GetPendingForAgent(ctx context.Context, agentID uuid.UUID) ([]*models.Command, error)
}

// PostgresCommandRepository implements CommandRepository using PostgreSQL.
type PostgresCommandRepository struct {
	db *pgxpool.Pool
}

// NewPostgresCommandRepository creates a new command repository.
func NewPostgresCommandRepository(db *pgxpool.Pool) *PostgresCommandRepository {
	return &PostgresCommandRepository{db: db}
}

// Create inserts a new command.
func (r *PostgresCommandRepository) Create(ctx context.Context, cmd *models.Command) error {
	if cmd.ID == uuid.Nil {
		cmd.ID = uuid.New()
	}
	if cmd.IssuedAt.IsZero() {
		cmd.IssuedAt = time.Now()
	}
	if cmd.TimeoutSeconds == 0 {
		cmd.TimeoutSeconds = 300
	}
	cmd.ExpiresAt = cmd.IssuedAt.Add(time.Duration(cmd.TimeoutSeconds) * time.Second)

	query := `
		INSERT INTO commands (
			id, agent_id, command_type, parameters, priority, status,
			timeout_seconds, issued_at, expires_at, issued_by, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.db.Exec(ctx, query,
		cmd.ID, cmd.AgentID, cmd.CommandType, cmd.Parameters, cmd.Priority,
		cmd.Status, cmd.TimeoutSeconds, cmd.IssuedAt, cmd.ExpiresAt,
		cmd.IssuedBy, cmd.Metadata,
	)
	return err
}

// GetByID retrieves a command by ID.
func (r *PostgresCommandRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Command, error) {
	query := `
		SELECT id, agent_id, command_type, parameters, priority, status,
			result, error_message, exit_code, timeout_seconds, issued_at,
			sent_at, acknowledged_at, started_at, completed_at, expires_at,
			issued_by, metadata
		FROM commands WHERE id = $1`

	cmd := &models.Command{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&cmd.ID, &cmd.AgentID, &cmd.CommandType, &cmd.Parameters,
		&cmd.Priority, &cmd.Status, &cmd.Result, &cmd.ErrorMessage,
		&cmd.ExitCode, &cmd.TimeoutSeconds, &cmd.IssuedAt, &cmd.SentAt,
		&cmd.AcknowledgedAt, &cmd.StartedAt, &cmd.CompletedAt,
		&cmd.ExpiresAt, &cmd.IssuedBy, &cmd.Metadata,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return cmd, err
}

// UpdateStatus updates the command status and result.
func (r *PostgresCommandRepository) UpdateStatus(
	ctx context.Context, id uuid.UUID,
	status models.CommandStatus, result map[string]any, errorMsg string,
) error {
	now := time.Now()

	query := `
		UPDATE commands SET
			status = $2, result = $3, error_message = $4,
			completed_at = CASE WHEN $2 IN ('completed', 'failed', 'timeout', 'cancelled') THEN $5 ELSE completed_at END,
			started_at = CASE WHEN $2 = 'executing' AND started_at IS NULL THEN $5 ELSE started_at END,
			sent_at = CASE WHEN $2 = 'sent' AND sent_at IS NULL THEN $5 ELSE sent_at END,
			acknowledged_at = CASE WHEN $2 = 'acknowledged' AND acknowledged_at IS NULL THEN $5 ELSE acknowledged_at END
		WHERE id = $1`

	result2, err := r.db.Exec(ctx, query, id, status, result, errorMsg, now)
	if err != nil {
		return err
	}
	if result2.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a command.
func (r *PostgresCommandRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, "DELETE FROM commands WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByAgent retrieves commands for an agent.
func (r *PostgresCommandRepository) ListByAgent(
	ctx context.Context, agentID uuid.UUID, limit, offset int,
) ([]*models.Command, int, error) {
	// Count total
	var total int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM commands WHERE agent_id = $1", agentID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch commands
	query := `
		SELECT id, agent_id, command_type, parameters, priority, status,
			result, error_message, issued_at, completed_at, issued_by
		FROM commands
		WHERE agent_id = $1
		ORDER BY issued_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, agentID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var commands []*models.Command
	for rows.Next() {
		cmd := &models.Command{}
		if err := rows.Scan(
			&cmd.ID, &cmd.AgentID, &cmd.CommandType, &cmd.Parameters,
			&cmd.Priority, &cmd.Status, &cmd.Result, &cmd.ErrorMessage,
			&cmd.IssuedAt, &cmd.CompletedAt, &cmd.IssuedBy,
		); err != nil {
			return nil, 0, err
		}
		commands = append(commands, cmd)
	}

	return commands, total, nil
}

// GetPendingForAgent retrieves pending commands for an agent.
func (r *PostgresCommandRepository) GetPendingForAgent(
	ctx context.Context, agentID uuid.UUID,
) ([]*models.Command, error) {
	query := `
		SELECT id, agent_id, command_type, parameters, priority, status,
			timeout_seconds, issued_at, expires_at, issued_by
		FROM commands
		WHERE agent_id = $1 AND status = 'pending' AND expires_at > NOW()
		ORDER BY priority DESC, issued_at ASC`

	rows, err := r.db.Query(ctx, query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []*models.Command
	for rows.Next() {
		cmd := &models.Command{}
		if err := rows.Scan(
			&cmd.ID, &cmd.AgentID, &cmd.CommandType, &cmd.Parameters,
			&cmd.Priority, &cmd.Status, &cmd.TimeoutSeconds,
			&cmd.IssuedAt, &cmd.ExpiresAt, &cmd.IssuedBy,
		); err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}

	return commands, nil
}

// PolicyRepository - simplified version
type PolicyRepository interface {
	Create(ctx context.Context, policy *models.Policy) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Policy, error)
	Update(ctx context.Context, policy *models.Policy) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, limit, offset int) ([]*models.Policy, int, error)
	GetEnabledForAgent(ctx context.Context, agentID uuid.UUID) ([]*models.Policy, error)
}

// PostgresPolicyRepository implements PolicyRepository.
type PostgresPolicyRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPolicyRepository creates a new policy repository.
func NewPostgresPolicyRepository(db *pgxpool.Pool) *PostgresPolicyRepository {
	return &PostgresPolicyRepository{db: db}
}

// Create inserts a new policy.
func (r *PostgresPolicyRepository) Create(ctx context.Context, policy *models.Policy) error {
	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}

	query := `
		INSERT INTO policies (
			id, name, description, rules, targets, enabled, priority, created_by, tags, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING version, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		policy.ID, policy.Name, policy.Description, policy.Rules,
		policy.Targets, policy.Enabled, policy.Priority, policy.CreatedBy,
		policy.Tags, policy.Metadata,
	).Scan(&policy.Version, &policy.CreatedAt, &policy.UpdatedAt)
}

// GetByID retrieves a policy by ID.
func (r *PostgresPolicyRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Policy, error) {
	query := `
		SELECT id, name, description, rules, targets, enabled, priority,
			version, created_by, updated_by, created_at, updated_at, tags, metadata
		FROM policies WHERE id = $1`

	policy := &models.Policy{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&policy.ID, &policy.Name, &policy.Description, &policy.Rules,
		&policy.Targets, &policy.Enabled, &policy.Priority, &policy.Version,
		&policy.CreatedBy, &policy.UpdatedBy, &policy.CreatedAt, &policy.UpdatedAt,
		&policy.Tags, &policy.Metadata,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return policy, err
}

// Update updates a policy.
func (r *PostgresPolicyRepository) Update(ctx context.Context, policy *models.Policy) error {
	query := `
		UPDATE policies SET
			name = $2, description = $3, rules = $4, targets = $5,
			enabled = $6, priority = $7, updated_by = $8, tags = $9, metadata = $10
		WHERE id = $1
		RETURNING version, updated_at`

	err := r.db.QueryRow(ctx, query,
		policy.ID, policy.Name, policy.Description, policy.Rules,
		policy.Targets, policy.Enabled, policy.Priority, policy.UpdatedBy,
		policy.Tags, policy.Metadata,
	).Scan(&policy.Version, &policy.UpdatedAt)

	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	return err
}

// Delete removes a policy.
func (r *PostgresPolicyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, "DELETE FROM policies WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List retrieves policies with pagination.
func (r *PostgresPolicyRepository) List(ctx context.Context, limit, offset int) ([]*models.Policy, int, error) {
	var total int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM policies").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, name, description, enabled, priority, version, created_at, updated_at
		FROM policies ORDER BY priority DESC, name LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var policies []*models.Policy
	for rows.Next() {
		p := &models.Policy{}
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.Enabled,
			&p.Priority, &p.Version, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		policies = append(policies, p)
	}

	return policies, total, nil
}

// GetEnabledForAgent returns enabled policies that apply to an agent.
func (r *PostgresPolicyRepository) GetEnabledForAgent(ctx context.Context, agentID uuid.UUID) ([]*models.Policy, error) {
	query := `
		SELECT p.id, p.name, p.description, p.rules, p.targets, p.enabled, p.priority, p.version
		FROM policies p
		WHERE p.enabled = true
		AND (
			p.targets->>'apply_to_all' = 'true'
			OR p.targets->'agents' ? $1::text
			OR EXISTS (
				SELECT 1 FROM policy_agent_assignments pa
				WHERE pa.policy_id = p.id AND pa.agent_id = $1
			)
		)
		ORDER BY p.priority DESC`

	rows, err := r.db.Query(ctx, query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*models.Policy
	for rows.Next() {
		p := &models.Policy{}
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.Rules,
			&p.Targets, &p.Enabled, &p.Priority, &p.Version,
		); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}

	return policies, nil
}
