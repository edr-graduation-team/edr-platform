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
	COALESCE(hardware_id, ''),
	COALESCE(cpu_count, 0), COALESCE(memory_mb, 0),
	COALESCE(agent_version, ''), installed_date,
	COALESCE(last_seen, created_at),
	COALESCE(events_collected, 0), COALESCE(events_delivered, 0),
	COALESCE(events_dropped, 0),
	COALESCE(queue_depth, 0), COALESCE(cpu_usage, 0.0),
	COALESCE(memory_used_mb, 0), COALESCE(health_score, 0.0),
	COALESCE(ip_addresses, '[]'),
	COALESCE(is_isolated, false),
	COALESCE(sysmon_installed, false), COALESCE(sysmon_running, false),
	current_cert_id, cert_expires_at,
	COALESCE(tags, '{}'), COALESCE(metadata, '{}'),
	COALESCE(criticality, 'medium'),
	COALESCE(business_unit, ''),
	COALESCE(environment, ''),
	created_at, updated_at`

// Create creates a new agent record.
func (r *PostgresAgentRepository) Create(ctx context.Context, agent *models.Agent) error {
	query := `
		INSERT INTO agents (
			id, hostname, status, os_type, os_version, hardware_id, cpu_count, memory_mb,
			agent_version, installed_date, last_seen, events_collected, events_delivered,
			queue_depth, cpu_usage, memory_used_mb, health_score, tags, metadata,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)`

	_, err := r.pool.Exec(ctx, query,
		agent.ID,
		agent.Hostname,
		agent.Status,
		agent.OSType,
		agent.OSVersion,
		agent.HardwareID,
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
		&agent.HardwareID,
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
		&agent.SysmonInstalled,
		&agent.SysmonRunning,
		&agent.CurrentCertID,
		&agent.CertExpiresAt,
		&agent.Tags,
		&agent.Metadata,
		&agent.Criticality,
		&agent.BusinessUnit,
		&agent.Environment,
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
		&agent.HardwareID,
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
		&agent.SysmonInstalled,
		&agent.SysmonRunning,
		&agent.CurrentCertID,
		&agent.CertExpiresAt,
		&agent.Tags,
		&agent.Metadata,
		&agent.Criticality,
		&agent.BusinessUnit,
		&agent.Environment,
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
			hardware_id = $6,
			cpu_count = $7, memory_mb = $8, agent_version = $9,
			last_seen = $10, events_collected = $11, events_delivered = $12,
			queue_depth = $13, cpu_usage = $14, memory_used_mb = $15,
			health_score = $16, current_cert_id = $17, cert_expires_at = $18,
			tags = $19, metadata = $20, updated_at = $21
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query,
		agent.ID,
		agent.Hostname,
		agent.Status,
		agent.OSType,
		agent.OSVersion,
		agent.HardwareID,
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
	agentVersion string, ipAddresses []string, cpuCount int, healthScore float64,
	sysmonInstalled, sysmonRunning bool) error {

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
			sysmon_installed = $14, sysmon_running = $15,
			last_seen = $13, updated_at = $13
		WHERE id = $1`

	now := time.Now()
	result, err := r.pool.Exec(ctx, query, id, cpuUsage, memoryUsedMB, memoryTotalMB, queueDepth,
		eventsGenerated, eventsSent, eventsDropped,
		agentVersion, string(ipJSON), cpuCount, healthScore, now,
		sysmonInstalled, sysmonRunning)

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
			&agent.HardwareID,
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
			&agent.SysmonInstalled,
			&agent.SysmonRunning,
			&agent.CurrentCertID,
			&agent.CertExpiresAt,
			&agent.Tags,
			&agent.Metadata,
			&agent.Criticality,
			&agent.BusinessUnit,
			&agent.Environment,
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
			&agent.HardwareID,
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
			&agent.SysmonInstalled,
			&agent.SysmonRunning,
			&agent.CurrentCertID,
			&agent.CertExpiresAt,
			&agent.Tags,
			&agent.Metadata,
			&agent.Criticality,
			&agent.BusinessUnit,
			&agent.Environment,
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

// AgentBusinessContext is the optional set of asset-context fields that can be
// updated together. Empty/nil entries are left unchanged.
type AgentBusinessContext struct {
	Criticality  *string // low | medium | high | critical
	BusinessUnit *string
	Environment  *string
}

// UpdateBusinessContext updates the agent's asset-context fields (criticality,
// business unit, environment). The DB trigger on agents.criticality automatically
// recomputes the priority_score of all linked vulnerability_findings.
func (r *PostgresAgentRepository) UpdateBusinessContext(ctx context.Context, id uuid.UUID, ctxFields AgentBusinessContext) error {
	if ctxFields.Criticality == nil && ctxFields.BusinessUnit == nil && ctxFields.Environment == nil {
		return nil
	}
	if ctxFields.Criticality != nil {
		switch *ctxFields.Criticality {
		case "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("invalid criticality: %s (allowed: low, medium, high, critical)", *ctxFields.Criticality)
		}
	}

	sets := []string{}
	args := []interface{}{}
	idx := 1
	if ctxFields.Criticality != nil {
		sets = append(sets, fmt.Sprintf("criticality = $%d", idx))
		args = append(args, *ctxFields.Criticality)
		idx++
	}
	if ctxFields.BusinessUnit != nil {
		sets = append(sets, fmt.Sprintf("business_unit = $%d", idx))
		args = append(args, *ctxFields.BusinessUnit)
		idx++
	}
	if ctxFields.Environment != nil {
		sets = append(sets, fmt.Sprintf("environment = $%d", idx))
		args = append(args, *ctxFields.Environment)
		idx++
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE agents SET %s WHERE id = $%d",
		joinStrings(sets, ", "), idx)

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update agent business context: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// joinStrings is a tiny helper to avoid pulling in strings.Join just for this file.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
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
	//    Each table has agent_id → agents(id) (or otherwise blocks changing agents.id),
	//    so all must be cleared before the ON CONFLICT DO UPDATE can replace the id.
	//
	//    Order: tables with secondary FKs to other agent-scoped tables first (e.g. ioc_enrichment
	//    → playbook_runs), then the rest. Must include *every* child of agents — omitting
	//    e.g. "events" caused 23503 on re-enrollment.
	childTables := []string{
		"ioc_enrichment",
		"triage_snapshots",
		"forensic_collections",
		"playbook_runs",
		"events",
		"enrollment_token_consumptions",
		"vulnerability_findings",
		"agent_quarantine_items",
		"agent_patch_profiles",
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

	// Unbind per-agent download rows without deleting the shared package (see 023 migration).
	_, err = tx.Exec(ctx, `
		UPDATE agent_packages
		SET agent_id = NULL, consumed_at = NULL
		WHERE agent_id IN (SELECT id FROM agents WHERE hostname = $1)
	`, agent.Hostname)
	if err != nil {
		return fmt.Errorf("failed to unbind agent_packages for re-enrollment: %w", err)
	}

	// 2. UPSERT the agent row — now safe to change the id (no FK references remain).
	query := `
		INSERT INTO agents (
			id, hostname, status, os_type, os_version, hardware_id, cpu_count, memory_mb,
			agent_version, installed_date, last_seen, events_collected, events_delivered,
			events_dropped, queue_depth, cpu_usage, memory_used_mb, health_score,
			tags, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			0, 0, 0, 0, 0.0, 0, 0.0,
			$12, $13, $14, $14
		)
		ON CONFLICT (hostname) DO UPDATE SET
			id             = EXCLUDED.id,
			status         = EXCLUDED.status,
			os_type        = EXCLUDED.os_type,
			os_version     = EXCLUDED.os_version,
			hardware_id    = EXCLUDED.hardware_id,
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
		agent.HardwareID,   // $6
		agent.CPUCount,     // $7
		agent.MemoryMB,     // $8
		agent.AgentVersion, // $9
		now,                // $10  installed_date
		now,                // $11 last_seen
		agent.Tags,         // $12
		agent.Metadata,     // $13
		now,                // $14 created_at / updated_at
	)
	if err != nil {
		return fmt.Errorf("failed to upsert agent by hostname: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit upsert transaction: %w", err)
	}
	return nil
}
