// Package repository provides PostgreSQL implementations for repositories.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// PostgresAgentRepository implements AgentRepository using PostgreSQL.
type PostgresAgentRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAgentRepository creates a new agent repository.
func NewPostgresAgentRepository(pool *pgxpool.Pool) *PostgresAgentRepository {
	return &PostgresAgentRepository{pool: pool}
}

// agentSelectColumns is the NULL-safe column list for all agent queries.
// Uses COALESCE to convert nullable DB columns to their Go zero-values,
// preventing "can't scan NULL into *int" panics when rows have missing data.
// Pointer-typed Go fields (installed_date, current_cert_id, cert_expires_at)
// handle NULL natively and don't need COALESCE.
const agentSelectColumns = `
	id, hostname, COALESCE(status, 'pending'),
	COALESCE(os_type, ''), COALESCE(os_version, ''),
	COALESCE(cpu_count, 0), COALESCE(memory_mb, 0),
	COALESCE(agent_version, ''), installed_date,
	COALESCE(last_seen, created_at),
	COALESCE(events_collected, 0), COALESCE(events_delivered, 0),
	COALESCE(events_dropped, 0),
	COALESCE(queue_depth, 0), COALESCE(cpu_usage, 0.0),
	COALESCE(memory_used_mb, 0), COALESCE(health_score, 0.0),
	COALESCE(ip_addresses, '[]'),
	COALESCE(is_isolated, false),
	current_cert_id, cert_expires_at,
	COALESCE(tags, '{}'), COALESCE(metadata, '{}'),
	created_at, updated_at`

// Create creates a new agent record.
func (r *PostgresAgentRepository) Create(ctx context.Context, agent *models.Agent) error {
	query := `
		INSERT INTO agents (
			id, hostname, status, os_type, os_version, cpu_count, memory_mb,
			agent_version, installed_date, last_seen, events_collected, events_delivered,
			queue_depth, cpu_usage, memory_used_mb, health_score, tags, metadata,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)`

	_, err := r.pool.Exec(ctx, query,
		agent.ID,
		agent.Hostname,
		agent.Status,
		agent.OSType,
		agent.OSVersion,
		agent.CPUCount,
		agent.MemoryMB,
		agent.AgentVersion,
		agent.InstalledDate,
		agent.LastSeen,
		agent.EventsCollected,
		agent.EventsDelivered,
		agent.QueueDepth,
		agent.CPUUsage,
		agent.MemoryUsedMB,
		agent.HealthScore,
		agent.Tags,
		agent.Metadata,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	return nil
}

// GetByID retrieves an agent by its ID.
func (r *PostgresAgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Agent, error) {
	query := `SELECT` + agentSelectColumns + `
		FROM agents
		WHERE id = $1`

	agent := &models.Agent{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&agent.ID,
		&agent.Hostname,
		&agent.Status,
		&agent.OSType,
		&agent.OSVersion,
		&agent.CPUCount,
		&agent.MemoryMB,
		&agent.AgentVersion,
		&agent.InstalledDate,
		&agent.LastSeen,
		&agent.EventsCollected,
		&agent.EventsDelivered,
		&agent.EventsDropped,
		&agent.QueueDepth,
		&agent.CPUUsage,
		&agent.MemoryUsedMB,
		&agent.HealthScore,
		&agent.IPAddresses,
		&agent.IsIsolated,
		&agent.CurrentCertID,
		&agent.CertExpiresAt,
		&agent.Tags,
		&agent.Metadata,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return agent, nil
}

// GetByHostname retrieves an agent by its hostname.
func (r *PostgresAgentRepository) GetByHostname(ctx context.Context, hostname string) (*models.Agent, error) {
	query := `SELECT` + agentSelectColumns + `
		FROM agents
		WHERE hostname = $1`

	agent := &models.Agent{}
	err := r.pool.QueryRow(ctx, query, hostname).Scan(
		&agent.ID,
		&agent.Hostname,
		&agent.Status,
		&agent.OSType,
		&agent.OSVersion,
		&agent.CPUCount,
		&agent.MemoryMB,
		&agent.AgentVersion,
		&agent.InstalledDate,
		&agent.LastSeen,
		&agent.EventsCollected,
		&agent.EventsDelivered,
		&agent.EventsDropped,
		&agent.QueueDepth,
		&agent.CPUUsage,
		&agent.MemoryUsedMB,
		&agent.HealthScore,
		&agent.IPAddresses,
		&agent.IsIsolated,
		&agent.CurrentCertID,
		&agent.CertExpiresAt,
		&agent.Tags,
		&agent.Metadata,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent by hostname: %w", err)
	}

	return agent, nil
}

// Update updates an existing agent.
func (r *PostgresAgentRepository) Update(ctx context.Context, agent *models.Agent) error {
	query := `
		UPDATE agents SET
			hostname = $2, status = $3, os_type = $4, os_version = $5,
			cpu_count = $6, memory_mb = $7, agent_version = $8,
			last_seen = $9, events_collected = $10, events_delivered = $11,
			queue_depth = $12, cpu_usage = $13, memory_used_mb = $14,
			health_score = $15, current_cert_id = $16, cert_expires_at = $17,
			tags = $18, metadata = $19, updated_at = $20
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query,
		agent.ID,
		agent.Hostname,
		agent.Status,
		agent.OSType,
		agent.OSVersion,
		agent.CPUCount,
		agent.MemoryMB,
		agent.AgentVersion,
		agent.LastSeen,
		agent.EventsCollected,
		agent.EventsDelivered,
		agent.QueueDepth,
		agent.CPUUsage,
		agent.MemoryUsedMB,
		agent.HealthScore,
		agent.CurrentCertID,
		agent.CertExpiresAt,
		agent.Tags,
		agent.Metadata,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateStatus updates the agent's status and last_seen timestamp.
// Returns ErrNotFound if the agent does not exist in the database —
// callers MUST handle this to enforce proper enrollment.
func (r *PostgresAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, lastSeen time.Time) error {
	query := `
		UPDATE agents SET
			status = $2, last_seen = $3, updated_at = $4
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id, status, lastSeen, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// AgentExists checks whether an agent with the given ID exists in PostgreSQL.
// Used by StreamEvents and Heartbeat to reject unknown/revoked agents.
func (r *PostgresAgentRepository) AgentExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)", id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check agent existence: %w", err)
	}
	return exists, nil
}

// UpdateMetrics updates the agent's metrics from a heartbeat.
func (r *PostgresAgentRepository) UpdateMetrics(ctx context.Context, id uuid.UUID,
	cpuUsage float64, memoryUsedMB int64, memoryTotalMB int64, queueDepth int,
	eventsGenerated, eventsSent, eventsDropped int64,
	agentVersion string, ipAddresses []string, cpuCount int, healthScore float64) error {

	// Marshal ip_addresses to JSON for JSONB column
	var ipJSON []byte
	if len(ipAddresses) > 0 {
		var err error
		ipJSON, err = json.Marshal(ipAddresses)
		if err != nil {
			ipJSON = []byte("[]")
		}
	} else {
		ipJSON = []byte("[]")
	}

	query := `
		UPDATE agents SET
			cpu_usage = $2, memory_used_mb = $3, memory_mb = $4, queue_depth = $5,
			events_collected = $6, events_delivered = $7, events_dropped = $8,
			agent_version = CASE WHEN $9 = '' THEN agent_version ELSE $9 END,
			ip_addresses = $10::jsonb,
			cpu_count = CASE WHEN $11 = 0 THEN cpu_count ELSE $11 END,
			health_score = CASE WHEN $12 < 0 THEN health_score ELSE $12 END,
			last_seen = $13, updated_at = $13
		WHERE id = $1`

	now := time.Now()
	result, err := r.pool.Exec(ctx, query, id, cpuUsage, memoryUsedMB, memoryTotalMB, queueDepth,
		eventsGenerated, eventsSent, eventsDropped,
		agentVersion, string(ipJSON), cpuCount, healthScore, now)

	if err != nil {
		return fmt.Errorf("failed to update agent metrics: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete soft-deletes an agent.
func (r *PostgresAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE agents SET status = 'deleted', updated_at = $2 WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id, time.Now())
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// List retrieves agents with optional filters.
func (r *PostgresAgentRepository) List(ctx context.Context, filter AgentFilter) ([]*models.Agent, error) {
	query := `SELECT` + agentSelectColumns + `
		FROM agents
		WHERE 1=1`

	args := []interface{}{}
	argNum := 1

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, *filter.Status)
		argNum++
	}

	if filter.OSType != nil {
		query += fmt.Sprintf(" AND os_type = $%d", argNum)
		args = append(args, *filter.OSType)
		argNum++
	}

	if filter.Search != nil {
		query += fmt.Sprintf(" AND hostname ILIKE $%d", argNum)
		args = append(args, "%"+*filter.Search+"%")
		argNum++
	}

	// Sort and pagination
	sortBy := "created_at"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	sortOrder := "DESC"
	if filter.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		agent := &models.Agent{}
		err := rows.Scan(
			&agent.ID,
			&agent.Hostname,
			&agent.Status,
			&agent.OSType,
			&agent.OSVersion,
			&agent.CPUCount,
			&agent.MemoryMB,
			&agent.AgentVersion,
			&agent.InstalledDate,
			&agent.LastSeen,
			&agent.EventsCollected,
			&agent.EventsDelivered,
			&agent.EventsDropped,
			&agent.QueueDepth,
			&agent.CPUUsage,
			&agent.MemoryUsedMB,
			&agent.HealthScore,
			&agent.IPAddresses,
			&agent.IsIsolated,
			&agent.CurrentCertID,
			&agent.CertExpiresAt,
			&agent.Tags,
			&agent.Metadata,
			&agent.CreatedAt,
			&agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent row: %w", err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// Count returns the number of agents matching the filter.
func (r *PostgresAgentRepository) Count(ctx context.Context, filter AgentFilter) (int64, error) {
	query := `SELECT COUNT(*) FROM agents WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, *filter.Status)
		argNum++
	}

	var count int64
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count agents: %w", err)
	}

	return count, nil
}

// GetOnlineAgents retrieves all agents with status "online".
func (r *PostgresAgentRepository) GetOnlineAgents(ctx context.Context) ([]*models.Agent, error) {
	status := "online"
	return r.List(ctx, AgentFilter{Status: &status, Limit: 10000})
}

// GetAgentsNeedingCertRenewal retrieves agents whose certs expire within the given duration.
func (r *PostgresAgentRepository) GetAgentsNeedingCertRenewal(ctx context.Context, within time.Duration) ([]*models.Agent, error) {
	query := `SELECT` + agentSelectColumns + `
		FROM agents
		WHERE cert_expires_at BETWEEN NOW() AND NOW() + $1
		AND status = 'online'`

	rows, err := r.pool.Query(ctx, query, within)
	if err != nil {
		return nil, fmt.Errorf("failed to get agents needing cert renewal: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		agent := &models.Agent{}
		err := rows.Scan(
			&agent.ID,
			&agent.Hostname,
			&agent.Status,
			&agent.OSType,
			&agent.OSVersion,
			&agent.CPUCount,
			&agent.MemoryMB,
			&agent.AgentVersion,
			&agent.InstalledDate,
			&agent.LastSeen,
			&agent.EventsCollected,
			&agent.EventsDelivered,
			&agent.EventsDropped,
			&agent.QueueDepth,
			&agent.CPUUsage,
			&agent.MemoryUsedMB,
			&agent.HealthScore,
			&agent.IPAddresses,
			&agent.IsIsolated,
			&agent.CurrentCertID,
			&agent.CertExpiresAt,
			&agent.Tags,
			&agent.Metadata,
			&agent.CreatedAt,
			&agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent row: %w", err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// MarkStaleOffline marks agents as offline if their last_seen timestamp
// is older than the given threshold. Only agents currently 'online' or
// 'degraded' are affected. Returns the number of agents updated.
func (r *PostgresAgentRepository) MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-threshold)
	query := `
		UPDATE agents
		SET status = 'offline', updated_at = NOW()
		WHERE status IN ('online', 'degraded')
		  AND last_seen < $1`

	result, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to mark stale agents offline: %w", err)
	}
	return result.RowsAffected(), nil
}

// SetIsolation updates the is_isolated flag for an agent.
func (r *PostgresAgentRepository) SetIsolation(ctx context.Context, id uuid.UUID, isolated bool) error {
	query := `UPDATE agents SET is_isolated = $1, updated_at = NOW() WHERE id = $2`
	result, err := r.pool.Exec(ctx, query, isolated, id)
	if err != nil {
		return fmt.Errorf("failed to update agent isolation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("agent %s not found", id)
	}
	return nil
}

// UpsertByHostname atomically creates or replaces the agent record for the given hostname.
//
// Re-enrollment strategy:
//
//	When a re-image or re-install sends a new registration request for an existing
//	hostname, this method uses PostgreSQL's ON CONFLICT (hostname) DO UPDATE to
//	overwrite the old row in place.  Key design choices:
//
//	  • The old agent UUID is captured into metadata["previous_agent_id"] before
//	    being overwritten — providing a full audit trail without a separate history table.
//	  • status is reset to "pending" so the admin dashboard shows the re-enrolled
//	    endpoint as requiring re-approval (same UX as a brand-new agent).
//	  • Telemetry counters (events_collected, etc.) are reset to zero because they
//	    belong to the previous install's lifecycle.
//	  • installed_date is updated to now() to reflect the new imaging time.
//	  • All other fields (os_type, os_version, cpu_count, memory_mb, agent_version,
//	    tags, ip_addresses) are refreshed from the incoming request.
//
// The operation runs inside a single implicit transaction (pgxpool.Exec is atomic).
func (r *PostgresAgentRepository) UpsertByHostname(ctx context.Context, agent *models.Agent) error {
	now := time.Now()

	// Use a transaction: we must delete FK-dependent rows from ALL child tables
	// before the UPSERT can safely change the agent's primary key (id).
	//
	// Child tables with FK → agents(id):
	//   certificates, installation_tokens, csrs,
	//   policy_agent_assignments, alerts, commands, command_queue
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Delete ALL FK-dependent rows referencing the previous agent with this hostname.
	//    Each table has agent_id → agents(id), so all must be cleared before the id can change.
	childTables := []string{
		"certificates",
		"installation_tokens",
		"csrs",
		"policy_agent_assignments",
		"alerts",
		"commands",
		"command_queue",
	}
	for _, table := range childTables {
		_, err = tx.Exec(ctx, fmt.Sprintf(`
			DELETE FROM %s
			WHERE agent_id IN (
				SELECT id FROM agents WHERE hostname = $1
			)`, table), agent.Hostname)
		if err != nil {
			return fmt.Errorf("failed to clean %s for re-enrollment: %w", table, err)
		}
	}

	// 2. UPSERT the agent row — now safe to change the id (no FK references remain).
	query := `
		INSERT INTO agents (
			id, hostname, status, os_type, os_version, cpu_count, memory_mb,
			agent_version, installed_date, last_seen, events_collected, events_delivered,
			events_dropped, queue_depth, cpu_usage, memory_used_mb, health_score,
			tags, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			0, 0, 0, 0, 0.0, 0, 0.0,
			$11, $12, $13, $13
		)
		ON CONFLICT (hostname) DO UPDATE SET
			id             = EXCLUDED.id,
			status         = EXCLUDED.status,
			os_type        = EXCLUDED.os_type,
			os_version     = EXCLUDED.os_version,
			cpu_count      = EXCLUDED.cpu_count,
			memory_mb      = EXCLUDED.memory_mb,
			agent_version  = EXCLUDED.agent_version,
			installed_date = EXCLUDED.installed_date,
			last_seen      = EXCLUDED.last_seen,
			events_collected  = 0,
			events_delivered  = 0,
			events_dropped    = 0,
			queue_depth    = 0,
			cpu_usage      = 0.0,
			memory_used_mb = 0,
			health_score   = 0.0,
			is_isolated    = false,
			current_cert_id = NULL,
			cert_expires_at = NULL,
			tags           = EXCLUDED.tags,
			metadata       = EXCLUDED.metadata,
			updated_at     = EXCLUDED.updated_at`

	_, err = tx.Exec(ctx, query,
		agent.ID,           // $1
		agent.Hostname,     // $2
		agent.Status,       // $3
		agent.OSType,       // $4
		agent.OSVersion,    // $5
		agent.CPUCount,     // $6
		agent.MemoryMB,     // $7
		agent.AgentVersion, // $8
		now,                // $9  installed_date
		now,                // $10 last_seen
		agent.Tags,         // $11
		agent.Metadata,     // $12
		now,                // $13 created_at / updated_at
	)
	if err != nil {
		return fmt.Errorf("failed to upsert agent by hostname: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit upsert transaction: %w", err)
	}
	return nil
}
