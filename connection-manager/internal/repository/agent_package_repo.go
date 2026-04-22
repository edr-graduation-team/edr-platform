package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type PostgresAgentPackageRepository struct {
	db *sql.DB
}

func NewPostgresAgentPackageRepository(db *sql.DB) *PostgresAgentPackageRepository {
	return &PostgresAgentPackageRepository{db: db}
}

func (r *PostgresAgentPackageRepository) Create(ctx context.Context, row AgentPackageRow) error {
	if row.ID == uuid.Nil {
		return fmt.Errorf("agent package id is required")
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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_packages (id, sha256, filename, storage_path, build_params, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, row.ID, row.SHA256, row.Filename, row.StoragePath, b, row.ExpiresAt)
	return err
}

func (r *PostgresAgentPackageRepository) Get(ctx context.Context, id uuid.UUID) (*AgentPackageRow, error) {
	var row AgentPackageRow
	var buildParamsRaw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, sha256, filename, storage_path, build_params, created_at, expires_at
		FROM agent_packages
		WHERE id=$1
	`, id).Scan(&row.ID, &row.SHA256, &row.Filename, &row.StoragePath, &buildParamsRaw, &row.CreatedAt, &row.ExpiresAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(buildParamsRaw, &row.BuildParams)
	return &row, nil
}

