// Package repository provides PostgreSQL implementations for repositories.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// PostgresCertificateRepository implements CertificateRepository using PostgreSQL.
type PostgresCertificateRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresCertificateRepository creates a new certificate repository.
func NewPostgresCertificateRepository(pool *pgxpool.Pool) *PostgresCertificateRepository {
	return &PostgresCertificateRepository{pool: pool}
}

// Create creates a new certificate record.
func (r *PostgresCertificateRepository) Create(ctx context.Context, cert *models.Certificate) error {
	query := `
		INSERT INTO certificates (
			id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.pool.Exec(ctx, query,
		cert.ID,
		cert.AgentID,
		cert.CertFingerprint,
		cert.PublicKey,
		cert.SerialNumber,
		cert.Status,
		cert.IssuedAt,
		cert.ExpiresAt,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	return nil
}

// GetByID retrieves a certificate by its ID.
func (r *PostgresCertificateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Certificate, error) {
	query := `
		SELECT id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, revoked_at, revoked_by, revoke_reason,
			created_at
		FROM certificates
		WHERE id = $1`

	cert := &models.Certificate{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&cert.ID,
		&cert.AgentID,
		&cert.CertFingerprint,
		&cert.PublicKey,
		&cert.SerialNumber,
		&cert.Status,
		&cert.IssuedAt,
		&cert.ExpiresAt,
		&cert.RevokedAt,
		&cert.RevokedBy,
		&cert.RevokeReason,
		&cert.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate: %w", err)
	}

	return cert, nil
}

// GetByFingerprint retrieves a certificate by its fingerprint.
func (r *PostgresCertificateRepository) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Certificate, error) {
	query := `
		SELECT id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, revoked_at, revoked_by, revoke_reason,
			created_at
		FROM certificates
		WHERE cert_fingerprint = $1`

	cert := &models.Certificate{}
	err := r.pool.QueryRow(ctx, query, fingerprint).Scan(
		&cert.ID,
		&cert.AgentID,
		&cert.CertFingerprint,
		&cert.PublicKey,
		&cert.SerialNumber,
		&cert.Status,
		&cert.IssuedAt,
		&cert.ExpiresAt,
		&cert.RevokedAt,
		&cert.RevokedBy,
		&cert.RevokeReason,
		&cert.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate by fingerprint: %w", err)
	}

	return cert, nil
}

// GetActiveByAgentID retrieves the active certificate for an agent.
func (r *PostgresCertificateRepository) GetActiveByAgentID(ctx context.Context, agentID uuid.UUID) (*models.Certificate, error) {
	query := `
		SELECT id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, revoked_at, revoked_by, revoke_reason,
			created_at
		FROM certificates
		WHERE agent_id = $1 AND status = 'active'
		ORDER BY issued_at DESC
		LIMIT 1`

	cert := &models.Certificate{}
	err := r.pool.QueryRow(ctx, query, agentID).Scan(
		&cert.ID,
		&cert.AgentID,
		&cert.CertFingerprint,
		&cert.PublicKey,
		&cert.SerialNumber,
		&cert.Status,
		&cert.IssuedAt,
		&cert.ExpiresAt,
		&cert.RevokedAt,
		&cert.RevokedBy,
		&cert.RevokeReason,
		&cert.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active certificate: %w", err)
	}

	return cert, nil
}

// Update updates an existing certificate.
func (r *PostgresCertificateRepository) Update(ctx context.Context, cert *models.Certificate) error {
	query := `
		UPDATE certificates SET
			status = $2, revoked_at = $3, revoked_by = $4, revoke_reason = $5
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query,
		cert.ID,
		cert.Status,
		cert.RevokedAt,
		cert.RevokedBy,
		cert.RevokeReason,
	)

	if err != nil {
		return fmt.Errorf("failed to update certificate: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Revoke marks a certificate as revoked.
func (r *PostgresCertificateRepository) Revoke(ctx context.Context, id uuid.UUID, revokedBy uuid.UUID, reason string) error {
	query := `
		UPDATE certificates SET
			status = 'revoked', revoked_at = $2, revoked_by = $3, revoke_reason = $4
		WHERE id = $1 AND status = 'active'`

	result, err := r.pool.Exec(ctx, query, id, time.Now(), revokedBy, reason)
	if err != nil {
		return fmt.Errorf("failed to revoke certificate: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkSuperseded marks a certificate as superseded by a new one.
func (r *PostgresCertificateRepository) MarkSuperseded(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE certificates SET status = 'superseded' WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark certificate as superseded: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetExpiring retrieves certificates expiring within the given duration.
func (r *PostgresCertificateRepository) GetExpiring(ctx context.Context, within time.Duration) ([]*models.Certificate, error) {
	query := `
		SELECT id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, revoked_at, revoked_by, revoke_reason,
			created_at
		FROM certificates
		WHERE status = 'active' AND expires_at BETWEEN NOW() AND NOW() + $1`

	rows, err := r.pool.Query(ctx, query, within)
	if err != nil {
		return nil, fmt.Errorf("failed to get expiring certificates: %w", err)
	}
	defer rows.Close()

	var certs []*models.Certificate
	for rows.Next() {
		cert := &models.Certificate{}
		err := rows.Scan(
			&cert.ID,
			&cert.AgentID,
			&cert.CertFingerprint,
			&cert.PublicKey,
			&cert.SerialNumber,
			&cert.Status,
			&cert.IssuedAt,
			&cert.ExpiresAt,
			&cert.RevokedAt,
			&cert.RevokedBy,
			&cert.RevokeReason,
			&cert.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan certificate row: %w", err)
		}
		certs = append(certs, cert)
	}

	return certs, nil
}

// List retrieves certificates with optional filters.
func (r *PostgresCertificateRepository) List(ctx context.Context, agentID uuid.UUID, status *string) ([]*models.Certificate, error) {
	query := `
		SELECT id, agent_id, cert_fingerprint, public_key, serial_number,
			status, issued_at, expires_at, revoked_at, revoked_by, revoke_reason,
			created_at
		FROM certificates
		WHERE agent_id = $1`

	args := []interface{}{agentID}

	if status != nil {
		query += " AND status = $2"
		args = append(args, *status)
	}

	query += " ORDER BY issued_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates: %w", err)
	}
	defer rows.Close()

	var certs []*models.Certificate
	for rows.Next() {
		cert := &models.Certificate{}
		err := rows.Scan(
			&cert.ID,
			&cert.AgentID,
			&cert.CertFingerprint,
			&cert.PublicKey,
			&cert.SerialNumber,
			&cert.Status,
			&cert.IssuedAt,
			&cert.ExpiresAt,
			&cert.RevokedAt,
			&cert.RevokedBy,
			&cert.RevokeReason,
			&cert.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan certificate row: %w", err)
		}
		certs = append(certs, cert)
	}

	return certs, nil
}
