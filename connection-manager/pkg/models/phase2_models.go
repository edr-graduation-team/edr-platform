// Package models provides Alert, Policy, and Command models.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// ALERT MODEL
// ============================================================================

// AlertSeverity represents alert severity levels.
type AlertSeverity string

const (
	AlertSeverityCritical      AlertSeverity = "critical"
	AlertSeverityHigh          AlertSeverity = "high"
	AlertSeverityMedium        AlertSeverity = "medium"
	AlertSeverityLow           AlertSeverity = "low"
	AlertSeverityInformational AlertSeverity = "informational"
)

// AlertStatus represents alert status values.
type AlertStatus string

const (
	AlertStatusOpen          AlertStatus = "open"
	AlertStatusInProgress    AlertStatus = "in_progress"
	AlertStatusResolved      AlertStatus = "resolved"
	AlertStatusClosed        AlertStatus = "closed"
	AlertStatusFalsePositive AlertStatus = "false_positive"
)

// AlertResolution represents how an alert was resolved.
type AlertResolution string

const (
	AlertResolutionFalsePositive AlertResolution = "false_positive"
	AlertResolutionRemediated    AlertResolution = "remediated"
	AlertResolutionEscalated     AlertResolution = "escalated"
	AlertResolutionAcceptedRisk  AlertResolution = "accepted_risk"
	AlertResolutionDuplicate     AlertResolution = "duplicate"
)

// Alert represents a security alert.
type Alert struct {
	ID              uuid.UUID         `json:"id" db:"id"`
	Severity        AlertSeverity     `json:"severity" db:"severity"`
	Title           string            `json:"title" db:"title"`
	Description     string            `json:"description,omitempty" db:"description"`
	AgentID         uuid.UUID         `json:"agent_id" db:"agent_id"`
	RuleID          string            `json:"rule_id,omitempty" db:"rule_id"`
	RuleName        string            `json:"rule_name,omitempty" db:"rule_name"`
	Status          AlertStatus       `json:"status" db:"status"`
	AssignedTo      *uuid.UUID        `json:"assigned_to,omitempty" db:"assigned_to"`
	Resolution      *AlertResolution  `json:"resolution,omitempty" db:"resolution"`
	ResolutionNotes string            `json:"resolution_notes,omitempty" db:"resolution_notes"`
	EventCount      int               `json:"event_count" db:"event_count"`
	FirstEventAt    *time.Time        `json:"first_event_at,omitempty" db:"first_event_at"`
	LastEventAt     *time.Time        `json:"last_event_at,omitempty" db:"last_event_at"`
	DetectedAt      time.Time         `json:"detected_at" db:"detected_at"`
	AcknowledgedAt  *time.Time        `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	ResolvedAt      *time.Time        `json:"resolved_at,omitempty" db:"resolved_at"`
	CreatedAt       time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at" db:"updated_at"`
	Tags            map[string]string `json:"tags,omitempty" db:"tags"`
	Metadata        map[string]any    `json:"metadata,omitempty" db:"metadata"`
	Notes           string            `json:"notes,omitempty" db:"notes"`

	// Context-Aware Risk Scoring (Phase 1) — populated from alerts.risk_score / context_snapshot / score_breakdown.
	// These fields are written by the sigma_engine's RiskScorer and read by the connection-manager API.
	RiskScore          int              `json:"risk_score" db:"risk_score"`
	ContextSnapshot    json.RawMessage  `json:"context_snapshot,omitempty" db:"context_snapshot"`
	ScoreBreakdown     json.RawMessage  `json:"score_breakdown,omitempty" db:"score_breakdown"`
	FalsePositiveRisk  float64          `json:"false_positive_risk" db:"false_positive_risk"`
}

// IsOpen returns true if alert is open or in progress.

// ==========================================================================
// Phase 2 — Endpoint Risk Intelligence
// ==========================================================================

// EndpointRiskSummary aggregates risk scoring data per agent.
// Computed by a single GROUP BY query on the alerts table; no agent join needed.
// The dashboard merges this with agent metadata (hostname, OS, status) fetched
// from the /api/v1/agents endpoint that is already cached by React Query.
type EndpointRiskSummary struct {
	AgentID       string    `json:"agent_id"`
	TotalAlerts   int64     `json:"total_alerts"`
	PeakRiskScore int       `json:"peak_risk_score"`
	AvgRiskScore  float64   `json:"avg_risk_score"`
	CriticalCount int64     `json:"critical_count"` // risk_score >= 90
	HighCount     int64     `json:"high_count"`     // risk_score 70-89
	OpenCount     int64     `json:"open_count"`     // status = 'open'
	LastAlertAt   time.Time `json:"last_alert_at"`
}

// IsOpen returns true if alert is open or in progress.
func (a *Alert) IsOpen() bool {
	return a.Status == AlertStatusOpen || a.Status == AlertStatusInProgress
}

// IsCritical returns true if alert is critical severity.
func (a *Alert) IsCritical() bool {
	return a.Severity == AlertSeverityCritical
}

// ============================================================================
// POLICY MODEL
// ============================================================================

// Policy represents a security policy.
type Policy struct {
	ID          uuid.UUID         `json:"id" db:"id"`
	Name        string            `json:"name" db:"name"`
	Description string            `json:"description,omitempty" db:"description"`
	Rules       []PolicyRule      `json:"rules" db:"rules"`
	Targets     PolicyTargets     `json:"targets" db:"targets"`
	Enabled     bool              `json:"enabled" db:"enabled"`
	Priority    int               `json:"priority" db:"priority"`
	Version     int               `json:"version" db:"version"`
	CreatedBy   uuid.UUID         `json:"created_by" db:"created_by"`
	UpdatedBy   *uuid.UUID        `json:"updated_by,omitempty" db:"updated_by"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
	Tags        map[string]string `json:"tags,omitempty" db:"tags"`
	Metadata    map[string]any    `json:"metadata,omitempty" db:"metadata"`
}

// PolicyRule represents a single rule within a policy.
type PolicyRule struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Action     string         `json:"action"` // allow, block, alert
	Conditions map[string]any `json:"conditions"`
}

// PolicyTargets defines which agents a policy applies to.
type PolicyTargets struct {
	ApplyToAll bool     `json:"apply_to_all"`
	Agents     []string `json:"agents,omitempty"`
	Groups     []string `json:"groups,omitempty"`
}

// PolicyVersion represents a historical version of a policy.
type PolicyVersion struct {
	ID           uuid.UUID     `json:"id" db:"id"`
	PolicyID     uuid.UUID     `json:"policy_id" db:"policy_id"`
	Version      int           `json:"version" db:"version"`
	Name         string        `json:"name" db:"name"`
	Description  string        `json:"description,omitempty" db:"description"`
	Rules        []PolicyRule  `json:"rules" db:"rules"`
	Targets      PolicyTargets `json:"targets" db:"targets"`
	Enabled      bool          `json:"enabled" db:"enabled"`
	Priority     int           `json:"priority" db:"priority"`
	ChangedBy    *uuid.UUID    `json:"changed_by,omitempty" db:"changed_by"`
	ChangeReason string        `json:"change_reason,omitempty" db:"change_reason"`
	CreatedAt    time.Time     `json:"created_at" db:"created_at"`
}

// ============================================================================
// COMMAND MODEL
// ============================================================================

// CommandType represents types of remote commands.
type CommandType string

const (
	CommandTypeKillProcess    CommandType = "kill_process"
	CommandTypeQuarantineFile CommandType = "quarantine_file"
	CommandTypeCollectLogs    CommandType = "collect_logs"
	CommandTypeUpdatePolicy   CommandType = "update_policy"
	CommandTypeRestartAgent   CommandType = "restart_agent"
	CommandTypeIsolateNetwork CommandType = "isolate_network"
	CommandTypeRestoreNetwork CommandType = "restore_network"
	CommandTypeScanFile       CommandType = "scan_file"
	CommandTypeScanMemory     CommandType = "scan_memory"
	CommandTypeCustom         CommandType = "custom"
	CommandTypeBlockIP          CommandType = "block_ip"
	CommandTypeUnblockIP        CommandType = "unblock_ip"
	CommandTypeBlockDomain      CommandType = "block_domain"
	CommandTypeUnblockDomain    CommandType = "unblock_domain"
	CommandTypeUpdateSignatures CommandType = "update_signatures"
	CommandTypeRestoreQuarantineFile CommandType = "restore_quarantine_file"
	CommandTypeDeleteQuarantineFile  CommandType = "delete_quarantine_file"
)

// CommandStatus represents command execution status.
type CommandStatus string

const (
	CommandStatusPending      CommandStatus = "pending"
	CommandStatusSent         CommandStatus = "sent"
	CommandStatusAcknowledged CommandStatus = "acknowledged"
	CommandStatusExecuting    CommandStatus = "executing"
	CommandStatusCompleted    CommandStatus = "completed"
	CommandStatusFailed       CommandStatus = "failed"
	CommandStatusTimeout      CommandStatus = "timeout"
	CommandStatusCancelled    CommandStatus = "cancelled"
)

// Command represents a remote command to execute on an agent.
type Command struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	AgentID        uuid.UUID      `json:"agent_id" db:"agent_id"`
	CommandType    CommandType    `json:"command_type" db:"command_type"`
	Parameters     map[string]any `json:"parameters,omitempty" db:"parameters"`
	Priority       int            `json:"priority" db:"priority"`
	Status         CommandStatus  `json:"status" db:"status"`
	Result         map[string]any `json:"result,omitempty" db:"result"`
	ErrorMessage   string         `json:"error_message,omitempty" db:"error_message"`
	ExitCode       *int           `json:"exit_code,omitempty" db:"exit_code"`
	TimeoutSeconds int            `json:"timeout_seconds" db:"timeout_seconds"`
	IssuedAt       time.Time      `json:"issued_at" db:"issued_at"`
	SentAt         *time.Time     `json:"sent_at,omitempty" db:"sent_at"`
	AcknowledgedAt *time.Time     `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty" db:"completed_at"`
	ExpiresAt      time.Time      `json:"expires_at" db:"expires_at"`
	IssuedBy       *uuid.UUID     `json:"issued_by,omitempty" db:"issued_by"`
	Metadata       map[string]any `json:"metadata,omitempty" db:"metadata"`
}

// IsPending returns true if command is waiting to be executed.
func (c *Command) IsPending() bool {
	return c.Status == CommandStatusPending || c.Status == CommandStatusSent
}

// IsComplete returns true if command has finished (success or failure).
func (c *Command) IsComplete() bool {
	return c.Status == CommandStatusCompleted ||
		c.Status == CommandStatusFailed ||
		c.Status == CommandStatusTimeout ||
		c.Status == CommandStatusCancelled
}

// IsExpired returns true if command has exceeded its expiration time.
func (c *Command) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// QuarantineItemState is the analyst / lifecycle state for an inventoried quarantined file.
type QuarantineItemState string

const (
	QuarantineStateQuarantined   QuarantineItemState = "quarantined"
	QuarantineStateAcknowledged  QuarantineItemState = "acknowledged"
	QuarantineStateRestored      QuarantineItemState = "restored"
	QuarantineStateDeleted       QuarantineItemState = "deleted"
)

// QuarantineItem represents one file held in the agent quarantine folder (server-side inventory).
type QuarantineItem struct {
	ID              uuid.UUID           `json:"id" db:"id"`
	AgentID         uuid.UUID           `json:"agent_id" db:"agent_id"`
	EventID         string              `json:"event_id,omitempty" db:"event_id"`
	OriginalPath    string              `json:"original_path" db:"original_path"`
	QuarantinePath  string              `json:"quarantine_path" db:"quarantine_path"`
	SHA256          string              `json:"sha256,omitempty" db:"sha256"`
	ThreatName      string              `json:"threat_name,omitempty" db:"threat_name"`
	Source          string              `json:"source" db:"source"`
	State           QuarantineItemState `json:"state" db:"state"`
	CreatedAt       time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at" db:"updated_at"`
}
