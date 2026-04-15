// Package repository defines the interfaces for data access.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// AgentRepository defines the interface for agent data access.
type AgentRepository interface {
	// Create creates a new agent record.
	Create(ctx context.Context, agent *models.Agent) error

	// GetByID retrieves an agent by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.Agent, error)

	// GetByHostname retrieves an agent by its hostname.
	GetByHostname(ctx context.Context, hostname string) (*models.Agent, error)

	// Update updates an existing agent.
	Update(ctx context.Context, agent *models.Agent) error

	// UpdateStatus updates the agent's status and last_seen timestamp.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, lastSeen time.Time) error

	// UpdateMetrics updates the agent's metrics from a heartbeat.
	UpdateMetrics(ctx context.Context, id uuid.UUID, cpuUsage float64, memoryUsedMB int64,
		memoryTotalMB int64, queueDepth int, eventsGenerated, eventsSent, eventsDropped int64,
		agentVersion string, ipAddresses []string, cpuCount int, healthScore float64) error

	// Delete soft-deletes an agent (sets status to deleted).
	Delete(ctx context.Context, id uuid.UUID) error

	// List retrieves agents with optional filters.
	List(ctx context.Context, filter AgentFilter) ([]*models.Agent, error)

	// Count returns the number of agents matching the filter.
	Count(ctx context.Context, filter AgentFilter) (int64, error)

	// GetOnlineAgents retrieves all agents with status "online".
	GetOnlineAgents(ctx context.Context) ([]*models.Agent, error)

	// GetAgentsNeedingCertRenewal retrieves agents whose certs expire within the given duration.
	GetAgentsNeedingCertRenewal(ctx context.Context, within time.Duration) ([]*models.Agent, error)

	// MarkStaleOffline marks agents as offline if their last_seen timestamp is older than the threshold.
	// Returns the number of agents that were marked offline.
	MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error)

	// SetIsolation updates the is_isolated flag on an agent.
	SetIsolation(ctx context.Context, id uuid.UUID, isolated bool) error

	// UpsertByHostname performs an INSERT ... ON CONFLICT (hostname) DO UPDATE,
	// atomically creating or replacing the agent record for the given hostname.
	// On collision (re-install / re-image scenario), the old agent ID is replaced
	// by the new one in a single statement, preserving DB integrity without a
	// manual delete step. Returns the final agent as stored.
	UpsertByHostname(ctx context.Context, agent *models.Agent) error
}

// AgentFilter defines filters for listing agents.
type AgentFilter struct {
	Status    *string
	OSType    *string
	Search    *string // Search in hostname
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string // "asc" or "desc"
}

// CertificateRepository defines the interface for certificate data access.
type CertificateRepository interface {
	// Create creates a new certificate record.
	Create(ctx context.Context, cert *models.Certificate) error

	// GetByID retrieves a certificate by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.Certificate, error)

	// GetByFingerprint retrieves a certificate by its fingerprint.
	GetByFingerprint(ctx context.Context, fingerprint string) (*models.Certificate, error)

	// GetActiveByAgentID retrieves the active certificate for an agent.
	GetActiveByAgentID(ctx context.Context, agentID uuid.UUID) (*models.Certificate, error)

	// Update updates an existing certificate.
	Update(ctx context.Context, cert *models.Certificate) error

	// Revoke marks a certificate as revoked.
	Revoke(ctx context.Context, id uuid.UUID, revokedBy uuid.UUID, reason string) error

	// MarkSuperseded marks a certificate as superseded by a new one.
	MarkSuperseded(ctx context.Context, id uuid.UUID) error

	// GetExpiring retrieves certificates expiring within the given duration.
	GetExpiring(ctx context.Context, within time.Duration) ([]*models.Certificate, error)

	// List retrieves certificates with optional filters.
	List(ctx context.Context, agentID uuid.UUID, status *string) ([]*models.Certificate, error)
}

// CSRRepository defines the interface for CSR data access.
type CSRRepository interface {
	// Create creates a new CSR record.
	Create(ctx context.Context, csr *models.CSR) error

	// GetByID retrieves a CSR by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.CSR, error)

	// GetByAgentID retrieves the pending CSR for an agent.
	GetByAgentID(ctx context.Context, agentID uuid.UUID) (*models.CSR, error)

	// Approve marks a CSR as approved.
	Approve(ctx context.Context, id uuid.UUID, approvedBy uuid.UUID) error

	// Delete deletes a CSR.
	Delete(ctx context.Context, id uuid.UUID) error

	// GetPending retrieves all pending CSRs.
	GetPending(ctx context.Context) ([]*models.CSR, error)

	// DeleteExpired deletes CSRs that have expired.
	DeleteExpired(ctx context.Context) (int64, error)
}

// InstallationTokenRepository defines the interface for installation token data access.
type InstallationTokenRepository interface {
	// Create creates a new installation token.
	Create(ctx context.Context, token *models.InstallationToken) error

	// GetByValue retrieves a token by its value.
	GetByValue(ctx context.Context, value string) (*models.InstallationToken, error)

	// MarkUsed marks a token as used by an agent.
	MarkUsed(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error

	// Delete deletes a token.
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteExpired deletes tokens that have expired.
	DeleteExpired(ctx context.Context) (int64, error)
}

// EnrollmentTokenRepository defines the interface for dynamic enrollment token management.
type EnrollmentTokenRepository interface {
	// Create creates a new enrollment token.
	Create(ctx context.Context, token *models.EnrollmentToken) error

	// GetByID retrieves an enrollment token by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.EnrollmentToken, error)

	// GetByToken retrieves an enrollment token by its token string.
	GetByToken(ctx context.Context, token string) (*models.EnrollmentToken, error)

	// List retrieves all enrollment tokens ordered by creation date.
	List(ctx context.Context) ([]*models.EnrollmentToken, error)

	// IncrementUsage increments the use_count of a token.
	IncrementUsage(ctx context.Context, id uuid.UUID) error

	// Revoke deactivates an enrollment token.
	Revoke(ctx context.Context, id uuid.UUID) error

	// Delete deletes an enrollment token.
	Delete(ctx context.Context, id uuid.UUID) error
}

// UserRepository defines the interface for user data access.
type UserRepository interface {
	// Create creates a new user.
	Create(ctx context.Context, user *models.User) error

	// GetByID retrieves a user by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)

	// GetByUsername retrieves a user by username.
	GetByUsername(ctx context.Context, username string) (*models.User, error)

	// GetByEmail retrieves a user by email.
	GetByEmail(ctx context.Context, email string) (*models.User, error)

	// Update updates an existing user.
	Update(ctx context.Context, user *models.User) error

	// UpdatePassword updates the user's password hash.
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error

	// Delete soft-deletes a user.
	Delete(ctx context.Context, id uuid.UUID) error

	// List retrieves users with optional filters.
	List(ctx context.Context, filter UserFilter) ([]*models.User, error)
}

// UserFilter defines filters for listing users.
type UserFilter struct {
	Role   *string
	Status *string
	Search *string
	Limit  int
	Offset int
}

// RoleRepository defines the interface for role and permission data access.
type RoleRepository interface {
	// ListRoles retrieves all roles with their permissions.
	ListRoles(ctx context.Context) ([]*models.Role, error)

	// GetRoleByName retrieves a single role by name, including permissions.
	GetRoleByName(ctx context.Context, name string) (*models.Role, error)

	// CreateRole creates a new custom role.
	CreateRole(ctx context.Context, role *models.Role) error

	// UpdateRolePermissions replaces the permission set for a role.
	UpdateRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error

	// DeleteRole deletes a custom role (built-in roles cannot be deleted).
	DeleteRole(ctx context.Context, id uuid.UUID) error

	// ListPermissions retrieves all available permissions.
	ListPermissions(ctx context.Context) ([]*models.Permission, error)

	// GetPermissionsForRoleName retrieves the permission keys for a role name.
	// Returns slice of "resource:action" strings.
	GetPermissionsForRoleName(ctx context.Context, roleName string) ([]string, error)
}

// AuditLogRepository defines the interface for audit log data access.
type AuditLogRepository interface {
	// Create creates a new audit log entry.
	Create(ctx context.Context, log *models.AuditLog) error

	// GetByID retrieves an audit log by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.AuditLog, error)

	// List retrieves audit logs with optional filters.
	List(ctx context.Context, filter AuditLogFilter) ([]*models.AuditLog, error)

	// Count returns the number of audit logs matching the filter.
	Count(ctx context.Context, filter AuditLogFilter) (int64, error)
}

// AuditLogFilter defines filters for listing audit logs.
type AuditLogFilter struct {
	UserID       *uuid.UUID
	Action       *string
	ResourceType *string
	ResourceID   *uuid.UUID
	Result       *string
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}
