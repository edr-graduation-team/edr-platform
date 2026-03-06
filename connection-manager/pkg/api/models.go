// Package api provides API request/response models.
package api

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// COMMON MODELS
// ============================================================================

// PaginationRequest for paginated endpoints.
type PaginationRequest struct {
	Limit  int `query:"limit" validate:"min=1,max=1000"`
	Offset int `query:"offset" validate:"min=0"`
}

// PaginationResponse for paginated responses.
type PaginationResponse struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

// TimeRange for time-based queries.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// ResponseMeta for all responses.
type ResponseMeta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// ErrorResponse for error responses.
type ErrorResponse struct {
	Error     string      `json:"error"`
	ErrorCode string      `json:"error_code,omitempty"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
	Timestamp string      `json:"timestamp"`
}

// ============================================================================
// AUTH MODELS
// ============================================================================

// LoginRequest for user login.
type LoginRequest struct {
	Username string `json:"username" validate:"required,min=3"`
	Password string `json:"password" validate:"required,min=8"`
}

// LoginResponse after successful login.
type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         UserResponse `json:"user"`
}

// RefreshTokenRequest for token refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshTokenResponse after token refresh.
type RefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// ============================================================================
// AGENT MODELS
// ============================================================================

// AgentListRequest for listing agents.
type AgentListRequest struct {
	PaginationRequest
	Status    string `query:"status"`
	OSType    string `query:"os_type"`
	Search    string `query:"search"`
	SortBy    string `query:"sort_by" validate:"omitempty,oneof=hostname status last_seen health_score"`
	SortOrder string `query:"sort_order" validate:"omitempty,oneof=asc desc"`
}

// AgentListResponse for agent list.
type AgentListResponse struct {
	Data       []AgentSummary     `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	Meta       ResponseMeta       `json:"meta"`
}

// AgentSummary for list views.
type AgentSummary struct {
	ID              uuid.UUID  `json:"id"`
	Hostname        string     `json:"hostname"`
	Status          string     `json:"status"`
	OSType          string     `json:"os_type"`
	OSVersion       string     `json:"os_version"`
	AgentVersion    string     `json:"agent_version"`
	LastSeen        time.Time  `json:"last_seen"`
	HealthScore     float64    `json:"health_score"`
	EventsDelivered int64      `json:"events_delivered"`
	CertExpiresAt   *time.Time `json:"cert_expires_at,omitempty"`
}

// AgentDetailResponse for single agent.
type AgentDetailResponse struct {
	Data AgentDetail  `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

// AgentDetail for detailed view.
type AgentDetail struct {
	AgentSummary
	InstalledDate   time.Time         `json:"installed_date"`
	IPAddresses     []string          `json:"ip_addresses"`
	CPUCount        int               `json:"cpu_count"`
	MemoryMB        int64             `json:"memory_mb"`
	Tags            map[string]string `json:"tags"`
	CurrentCertID   *uuid.UUID        `json:"current_cert_id,omitempty"`
	EventsGenerated int64             `json:"events_generated"`
	EventsSent      int64             `json:"events_sent"`
	CPUUsage        float64           `json:"cpu_usage"`
	MemoryUsedMB    int64             `json:"memory_used_mb"`
	QueueDepth      int               `json:"queue_depth"`
}

// AgentUpdateRequest for updating agent.
type AgentUpdateRequest struct {
	Tags map[string]string `json:"tags"`
}

// AgentStatsResponse for agent statistics.
type AgentStatsResponse struct {
	Total     int            `json:"total"`
	Online    int            `json:"online"`
	Offline   int            `json:"offline"`
	Degraded  int            `json:"degraded"`
	Pending   int            `json:"pending"`
	Suspended int            `json:"suspended"`
	ByOSType  map[string]int `json:"by_os_type"`
	ByVersion map[string]int `json:"by_version"`
	AvgHealth float64        `json:"avg_health"`
	Meta      ResponseMeta   `json:"meta"`
}

// ============================================================================
// COMMAND MODELS
// ============================================================================

// CommandRequest for executing a command.
type CommandRequest struct {
	CommandType string            `json:"command_type" validate:"required,oneof=kill_process quarantine_file collect_logs update_policy restart_agent restart_machine shutdown_machine isolate_network restore_network scan_file scan_memory custom update_agent adjust_rate run_cmd"`
	Parameters  map[string]string `json:"parameters"`
	Timeout     int               `json:"timeout" validate:"min=0,max=3600"`
}

// CommandResponse for command execution.
type CommandResponse struct {
	CommandID string    `json:"command_id"`
	Status    string    `json:"status"`
	IssuedAt  time.Time `json:"issued_at"`
}

// CommandListResponse for command history.
type CommandListResponse struct {
	Data       []CommandSummary   `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	Meta       ResponseMeta       `json:"meta"`
}

// CommandSummary for command history.
type CommandSummary struct {
	ID          uuid.UUID         `json:"id"`
	CommandType string            `json:"command_type"`
	Parameters  map[string]string `json:"parameters"`
	Status      string            `json:"status"`
	IssuedAt    time.Time         `json:"issued_at"`
	IssuedBy    string            `json:"issued_by"`
	ExecutedAt  *time.Time        `json:"executed_at,omitempty"`
	Result      interface{}       `json:"result,omitempty"`
}

// ============================================================================
// ALERT MODELS
// ============================================================================

// AlertSearchRequest for searching alerts.
type AlertSearchRequest struct {
	Severity  []string  `json:"severity"`
	Status    []string  `json:"status"`
	AgentID   string    `json:"agent_id"`
	RuleID    string    `json:"rule_id"`
	TimeRange TimeRange `json:"time_range"`
	Query     string    `json:"query"`
	Limit     int       `json:"limit" validate:"max=1000"`
	Offset    int       `json:"offset"`
	SortBy    string    `json:"sort_by"`
	SortOrder string    `json:"sort_order"`
}

// AlertListResponse for alert list.
type AlertListResponse struct {
	Data       []AlertSummary     `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	Meta       ResponseMeta       `json:"meta"`
}

// AlertSummary for list views.
type AlertSummary struct {
	ID         uuid.UUID  `json:"id"`
	Severity   string     `json:"severity"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	AgentID    uuid.UUID  `json:"agent_id"`
	AgentName  string     `json:"agent_name"`
	EventCount int        `json:"event_count"`
	DetectedAt time.Time  `json:"detected_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	AssignedTo *string    `json:"assigned_to,omitempty"`
}

// AlertDetailResponse for single alert.
type AlertDetailResponse struct {
	Data AlertDetail  `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

// AlertDetail for detailed view.
type AlertDetail struct {
	AlertSummary
	Description string            `json:"description"`
	RuleID      *uuid.UUID        `json:"rule_id,omitempty"`
	RuleName    string            `json:"rule_name,omitempty"`
	EventIDs    []uuid.UUID       `json:"event_ids"`
	Resolution  string            `json:"resolution,omitempty"`
	Notes       string            `json:"notes,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Metadata    interface{}       `json:"metadata,omitempty"`
}

// AlertUpdateRequest for updating alert.
type AlertUpdateRequest struct {
	Status     string `json:"status" validate:"omitempty,oneof=open in_progress resolved"`
	AssignedTo string `json:"assigned_to"`
}

// AlertResolveRequest for resolving alert.
type AlertResolveRequest struct {
	Resolution string `json:"resolution" validate:"required,oneof=false_positive remediated escalated"`
	Notes      string `json:"notes"`
}

// AlertNoteRequest for adding note.
type AlertNoteRequest struct {
	Note string `json:"note" validate:"required,min=1"`
}

// AlertStatsResponse for alert statistics.
type AlertStatsResponse struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	InProgress int            `json:"in_progress"`
	Resolved   int            `json:"resolved"`
	BySeverity map[string]int `json:"by_severity"`
	Meta       ResponseMeta   `json:"meta"`
}

// ============================================================================
// USER MODELS
// ============================================================================

// UserResponse for user data.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	LastLogin time.Time `json:"last_login,omitempty"`
}

// UserCreateRequest for creating user.
type UserCreateRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	FullName string `json:"full_name" validate:"required"`
	Role     string `json:"role" validate:"required,oneof=admin security analyst operations viewer"`
}

// UserUpdateRequest for updating user.
type UserUpdateRequest struct {
	Email    string `json:"email" validate:"omitempty,email"`
	FullName string `json:"full_name"`
	Role     string `json:"role" validate:"omitempty,oneof=admin security analyst operations viewer"`
	Status   string `json:"status" validate:"omitempty,oneof=active inactive locked"`
}

// PasswordChangeRequest for changing password.
type PasswordChangeRequest struct {
	OldPassword string `json:"old_password" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=12"`
}

// ============================================================================
// POLICY MODELS
// ============================================================================

// PolicyListResponse for policy list.
type PolicyListResponse struct {
	Data       []PolicySummary    `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	Meta       ResponseMeta       `json:"meta"`
}

// PolicySummary for list views.
type PolicySummary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Priority    int       `json:"priority"`
	AgentCount  int       `json:"agent_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PolicyCreateRequest for creating policy.
type PolicyCreateRequest struct {
	Name        string        `json:"name" validate:"required,min=3"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Priority    int           `json:"priority"`
	Rules       []PolicyRule  `json:"rules" validate:"required,min=1"`
	Targets     PolicyTargets `json:"targets"`
}

// PolicyRule defines a single policy rule.
type PolicyRule struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Action     string                 `json:"action" validate:"oneof=allow block alert"`
	Conditions map[string]interface{} `json:"conditions"`
}

// PolicyTargets defines policy targets.
type PolicyTargets struct {
	Agents     []string `json:"agents"`
	Groups     []string `json:"groups"`
	ApplyToAll bool     `json:"apply_to_all"`
}

// ============================================================================
// EVENT MODELS
// ============================================================================

// EventSearchRequest for searching events.
type EventSearchRequest struct {
	Filters   []EventFilter `json:"filters"`
	Logic     string        `json:"logic" validate:"oneof=AND OR"`
	TimeRange TimeRange     `json:"time_range"`
	Limit     int           `json:"limit" validate:"max=10000"`
	Offset    int           `json:"offset"`
}

// EventFilter for event filtering.
type EventFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator" validate:"oneof=equals contains regex gt lt gte lte"`
	Value    interface{} `json:"value"`
}

// EventListResponse for event list.
type EventListResponse struct {
	Data       []EventSummary     `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	Meta       ResponseMeta       `json:"meta"`
}

// EventSummary for list views.
type EventSummary struct {
	ID        uuid.UUID `json:"id"`
	AgentID   uuid.UUID `json:"agent_id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	Summary   string    `json:"summary"`
}

// EventExportRequest for exporting events.
type EventExportRequest struct {
	EventSearchRequest
	Format string `json:"format" validate:"oneof=json csv"`
}
