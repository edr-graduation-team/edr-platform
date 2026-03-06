// Package models defines the domain models for the connection-manager.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AuditLog represents an immutable audit log entry.
type AuditLog struct {
	ID uuid.UUID `db:"id" json:"id"`

	// Who performed the action
	UserID   uuid.UUID `db:"user_id" json:"user_id"`
	Username string    `db:"username" json:"username"`

	// What action was performed
	Action       string    `db:"action" json:"action"`               // e.g., "agent_approved", "token_revoked"
	ResourceType string    `db:"resource_type" json:"resource_type"` // e.g., "agent", "certificate", "user"
	ResourceID   uuid.UUID `db:"resource_id" json:"resource_id"`

	// Change details
	OldValue json.RawMessage `db:"old_value" json:"old_value"`
	NewValue json.RawMessage `db:"new_value" json:"new_value"`
	Details  string          `db:"details" json:"details"`

	// Outcome
	Result       string `db:"result" json:"result"` // success, failure
	ErrorMessage string `db:"error_message" json:"error_message"`

	// Context
	IPAddress string `db:"ip_address" json:"ip_address"`
	UserAgent string `db:"user_agent" json:"user_agent"`

	Timestamp time.Time `db:"timestamp" json:"timestamp"`
}

// Audit action constants
const (
	// Agent actions
	AuditActionAgentRegistered = "agent_registered"
	AuditActionAgentApproved   = "agent_approved"
	AuditActionAgentSuspended  = "agent_suspended"
	AuditActionAgentDeleted    = "agent_deleted"

	// Certificate actions
	AuditActionCertIssued  = "cert_issued"
	AuditActionCertRenewed = "cert_renewed"
	AuditActionCertRevoked = "cert_revoked"

	// Token actions
	AuditActionTokenCreated = "token_created"
	AuditActionTokenRevoked = "token_revoked"

	// User actions
	AuditActionUserLogin       = "user_login"
	AuditActionUserLoginFailed = "user_login_failed"
	AuditActionUserLogout      = "user_logout"
	AuditActionUserCreated     = "user_created"
	AuditActionUserUpdated     = "user_updated"
	AuditActionUserDeleted     = "user_deleted"
	AuditActionUserLocked      = "user_locked"
	AuditActionPasswordChanged = "password_changed"

	// Aliases for convenience
	AuditActionLoginSuccess = "user_login"
	AuditActionLoginFailed  = "user_login_failed"

	// System actions
	AuditActionRateLimitTriggered = "rate_limit_triggered"
	AuditActionAuthFailed         = "auth_failed"
)

// Audit result constants
const (
	AuditResultSuccess = "success"
	AuditResultFailure = "failure"
)

// NewAuditLog creates a new audit log entry.
func NewAuditLog(userID uuid.UUID, username, action, resourceType string, resourceID uuid.UUID) *AuditLog {
	return &AuditLog{
		ID:           uuid.New(),
		UserID:       userID,
		Username:     username,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       AuditResultSuccess,
		Timestamp:    time.Now(),
	}
}

// WithDetails adds details to the audit log.
func (a *AuditLog) WithDetails(details string) *AuditLog {
	a.Details = details
	return a
}

// WithDetail adds a key-value detail (appends to Details string).
func (a *AuditLog) WithDetail(key, value string) *AuditLog {
	if a.Details != "" {
		a.Details += ", "
	}
	a.Details += key + "=" + value
	return a
}

// WithOldValue adds the old value to the audit log.
func (a *AuditLog) WithOldValue(value interface{}) *AuditLog {
	if data, err := json.Marshal(value); err == nil {
		a.OldValue = data
	}
	return a
}

// WithNewValue adds the new value to the audit log.
func (a *AuditLog) WithNewValue(value interface{}) *AuditLog {
	if data, err := json.Marshal(value); err == nil {
		a.NewValue = data
	}
	return a
}

// WithContext adds request context to the audit log.
func (a *AuditLog) WithContext(ipAddress, userAgent string) *AuditLog {
	a.IPAddress = ipAddress
	a.UserAgent = userAgent
	return a
}

// MarkFailed marks the audit log as a failure.
func (a *AuditLog) MarkFailed(errorMessage string) *AuditLog {
	a.Result = AuditResultFailure
	a.ErrorMessage = errorMessage
	return a
}
