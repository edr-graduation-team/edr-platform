package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLog represents a system audit event.
type AuditLog struct {
	ID           uuid.UUID       `json:"id"`
	UserID       uuid.UUID       `json:"user_id"`
	Username     string          `json:"username"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   uuid.UUID       `json:"resource_id,omitempty"`
	OldValue     json.RawMessage `json:"old_value,omitempty"`
	NewValue     json.RawMessage `json:"new_value,omitempty"`
	Result       string          `json:"result"`
	ErrorMessage string          `json:"error_message,omitempty"`
	IPAddress    string          `json:"ip_address,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

// AuditLogger handles writing audit logs to the shared DB.
type AuditLogger struct {
	pool *pgxpool.Pool
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

// Log writes an audit log to the database.
func (l *AuditLogger) Log(ctx context.Context, action string, resType string, resID string, username string, userID string, ip string, result string, details string) error {
	id := uuid.New()
	
	// Parse IDs
	var uID uuid.UUID
	if userID != "" {
		if parsed, err := uuid.Parse(userID); err == nil {
			uID = parsed
		}
	}
	
	var rID uuid.UUID
	if resID != "" {
		if parsed, err := uuid.Parse(resID); err == nil {
			rID = parsed
		}
	}

	query := `
		INSERT INTO audit_logs (
			id, user_id, username, action, resource_type, resource_id,
			result, error_message, ip_address, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := l.pool.Exec(ctx, query,
		id,
		uID,
		username,
		action,
		resType,
		rID,
		result,
		details,
		ip,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}
