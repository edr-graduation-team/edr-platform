package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAgentPackageRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAgentPackageRepository(pool *pgxpool.Pool) *PostgresAgentPackageRepository {
	return &PostgresAgentPackageRepository{pool: pool}
}

func (r *PostgresAgentPackageRepository) Create(ctx context.Context, row AgentPackageRow) error {
	if row.ID == uuid.Nil {
		return fmt.Errorf("agent package id is required")
	}
	if row.AgentID == uuid.Nil {
		return fmt.Errorf("agent package must be bound to an agent_id")
	}
	if row.SHA256 == "" || row.StoragePath == "" {
		return fmt.Errorf("sha256 and storage_path are required")
	}
	if row.Filename == "" {
		row.Filename = "edr-agent.exe"
	}
	if row.ExpiresAt.IsZero() {
		row.ExpiresAt = time.Now().Add(15 * time.Minute)
	}
	if row.BuildParams == nil {
		row.BuildParams = map[string]any{}
	}
	b, _ := json.Marshal(row.BuildParams)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO agent_packages (id, agent_id, sha256, filename, storage_path, build_params, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, row.ID, row.AgentID, row.SHA256, row.Filename, row.StoragePath, b, row.ExpiresAt)
	return err
}

func (r *PostgresAgentPackageRepository) Get(ctx context.Context, id uuid.UUID) (*AgentPackageRow, error) {
	var row AgentPackageRow
	var buildParamsRaw []byte
	var agentID *uuid.UUID
	var consumedAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, agent_id, sha256, filename, storage_path, build_params, created_at, expires_at, consumed_at
		FROM agent_packages
		WHERE id=$1
	`, id).Scan(&row.ID, &agentID, &row.SHA256, &row.Filename, &row.StoragePath, &buildParamsRaw, &row.CreatedAt, &row.ExpiresAt, &consumedAt)
	if err != nil {
		return nil, err
	}
	if agentID != nil {
		row.AgentID = *agentID
	}
	if consumedAt != nil {
		row.ConsumedAt = consumedAt
	}
	_ = json.Unmarshal(buildParamsRaw, &row.BuildParams)
	return &row, nil
}

func (r *PostgresAgentPackageRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE agent_packages SET consumed_at = NOW() WHERE id=$1 AND consumed_at IS NULL
	`, id)
	return err
}

func (r *PostgresAgentPackageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM agent_packages WHERE id=$1`, id)
	return err
}

func (r *PostgresAgentPackageRepository) ListExpired(ctx context.Context, before time.Time) ([]*AgentPackageRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, sha256, filename, storage_path, build_params, created_at, expires_at, consumed_at
		FROM agent_packages
		WHERE expires_at < $1
	`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AgentPackageRow
	for rows.Next() {
		var row AgentPackageRow
		var buildParamsRaw []byte
		var agentID *uuid.UUID
		var consumedAt *time.Time
		if err := rows.Scan(&row.ID, &agentID, &row.SHA256, &row.Filename, &row.StoragePath, &buildParamsRaw, &row.CreatedAt, &row.ExpiresAt, &consumedAt); err != nil {
			return nil, err
		}
		if agentID != nil {
			row.AgentID = *agentID
		}
		if consumedAt != nil {
			row.ConsumedAt = consumedAt
		}
		_ = json.Unmarshal(buildParamsRaw, &row.BuildParams)
		out = append(out, &row)
	}
	return out, rows.Err()
}
