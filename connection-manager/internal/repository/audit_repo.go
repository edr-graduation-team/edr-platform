// Package repository provides PostgreSQL implementations for repositories.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// PostgresAuditLogRepository implements AuditLogRepository using PostgreSQL.
type PostgresAuditLogRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAuditLogRepository creates a new audit log repository.
func NewPostgresAuditLogRepository(pool *pgxpool.Pool) *PostgresAuditLogRepository {
	return &PostgresAuditLogRepository{pool: pool}
}

// Create creates a new audit log entry.
func (r *PostgresAuditLogRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := `
		INSERT INTO audit_logs (
			id, user_id, username, action, resource_type, resource_id,
			old_value, new_value, result, error_message,
			ip_address, user_agent, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := r.pool.Exec(ctx, query,
		log.ID,
		log.UserID,
		log.Username,
		log.Action,
		log.ResourceType,
		log.ResourceID,
		log.OldValue,
		log.NewValue,
		log.Result,
		log.ErrorMessage,
		log.IPAddress,
		log.UserAgent,
		log.Timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// GetByID retrieves an audit log by its ID.
func (r *PostgresAuditLogRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.AuditLog, error) {
	query := `
		SELECT id, user_id, username, action, resource_type, resource_id,
			old_value, new_value, result, error_message,
			ip_address, user_agent, created_at
		FROM audit_logs
		WHERE id = $1`

	log := &models.AuditLog{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&log.ID,
		&log.UserID,
		&log.Username,
		&log.Action,
		&log.ResourceType,
		&log.ResourceID,
		&log.OldValue,
		&log.NewValue,
		&log.Result,
		&log.ErrorMessage,
		&log.IPAddress,
		&log.UserAgent,
		&log.Timestamp,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return log, nil
}

// List retrieves audit logs with optional filters.
func (r *PostgresAuditLogRepository) List(ctx context.Context, filter AuditLogFilter) ([]*models.AuditLog, error) {
	query := `
		SELECT id, user_id, username, action, resource_type, resource_id,
			old_value, new_value, result, error_message,
			ip_address, user_agent, created_at
		FROM audit_logs
		WHERE 1=1`

	args := []interface{}{}
	argNum := 1

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argNum)
		args = append(args, *filter.UserID)
		argNum++
	}

	if filter.Action != nil {
		query += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, *filter.Action)
		argNum++
	}

	if filter.ResourceType != nil {
		query += fmt.Sprintf(" AND resource_type = $%d", argNum)
		args = append(args, *filter.ResourceType)
		argNum++
	}

	if filter.ResourceID != nil {
		query += fmt.Sprintf(" AND resource_id = $%d", argNum)
		args = append(args, *filter.ResourceID)
		argNum++
	}

	if filter.Result != nil {
		query += fmt.Sprintf(" AND result = $%d", argNum)
		args = append(args, *filter.Result)
		argNum++
	}

	if filter.StartTime != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filter.StartTime)
		argNum++
	}

	if filter.EndTime != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filter.EndTime)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*models.AuditLog
	for rows.Next() {
		log := &models.AuditLog{}
		err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.Username,
			&log.Action,
			&log.ResourceType,
			&log.ResourceID,
			&log.OldValue,
			&log.NewValue,
			&log.Result,
			&log.ErrorMessage,
			&log.IPAddress,
			&log.UserAgent,
			&log.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log row: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// Count returns the number of audit logs matching the filter.
func (r *PostgresAuditLogRepository) Count(ctx context.Context, filter AuditLogFilter) (int64, error) {
	query := `SELECT COUNT(*) FROM audit_logs WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argNum)
		args = append(args, *filter.UserID)
		argNum++
	}

	if filter.Action != nil {
		query += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, *filter.Action)
		argNum++
	}

	if filter.StartTime != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filter.StartTime)
		argNum++
	}

	if filter.EndTime != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filter.EndTime)
		argNum++
	}

	var count int64
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}

// PostgresInstallationTokenRepository implements InstallationTokenRepository.
type PostgresInstallationTokenRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresInstallationTokenRepository creates a new token repository.
func NewPostgresInstallationTokenRepository(pool *pgxpool.Pool) *PostgresInstallationTokenRepository {
	return &PostgresInstallationTokenRepository{pool: pool}
}

// Create creates a new installation token.
func (r *PostgresInstallationTokenRepository) Create(ctx context.Context, token *models.InstallationToken) error {
	query := `
		INSERT INTO installation_tokens (id, token_value, created_at, expires_at)
		VALUES ($1, $2, $3, $4)`

	_, err := r.pool.Exec(ctx, query,
		token.ID,
		token.TokenValue,
		time.Now(),
		token.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create installation token: %w", err)
	}

	return nil
}

// GetByValue retrieves a token by its value.
// Uses nullable types for agent_id and used_at so SQL NULLs scan correctly
// (unused tokens have NULL in those columns).
func (r *PostgresInstallationTokenRepository) GetByValue(ctx context.Context, value string) (*models.InstallationToken, error) {
	query := `
		SELECT id, token_value, agent_id, used, used_at, created_at, expires_at
		FROM installation_tokens
		WHERE token_value = $1`

	var (
		id        uuid.UUID
		tokenVal  string
		agentID   pgtype.UUID
		used      bool
		usedAt    sql.NullTime
		createdAt time.Time
		expiresAt time.Time
	)
	err := r.pool.QueryRow(ctx, query, value).Scan(
		&id,
		&tokenVal,
		&agentID,
		&used,
		&usedAt,
		&createdAt,
		&expiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	token := &models.InstallationToken{
		ID:         id,
		TokenValue: tokenVal,
		Used:       used,
		CreatedAt:  createdAt,
		ExpiresAt:  expiresAt,
	}
	if agentID.Valid {
		token.AgentID = uuid.UUID(agentID.Bytes)
	}
	if usedAt.Valid {
		token.UsedAt = usedAt.Time
	}
	return token, nil
}

// MarkUsed marks a token as used by an agent.
func (r *PostgresInstallationTokenRepository) MarkUsed(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	query := `
		UPDATE installation_tokens SET used = TRUE, used_at = $2, agent_id = $3
		WHERE id = $1 AND used = FALSE`

	result, err := r.pool.Exec(ctx, query, id, time.Now(), agentID)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete deletes a token.
func (r *PostgresInstallationTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM installation_tokens WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete installation token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteExpired deletes tokens that have expired.
func (r *PostgresInstallationTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM installation_tokens WHERE expires_at < NOW() AND used = FALSE`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	return result.RowsAffected(), nil
}
