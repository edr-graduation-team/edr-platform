package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAgentPatchProfileRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAgentPatchProfileRepository(pool *pgxpool.Pool) *PostgresAgentPatchProfileRepository {
	return &PostgresAgentPatchProfileRepository{pool: pool}
}

func (r *PostgresAgentPatchProfileRepository) Get(ctx context.Context, agentID uuid.UUID) (map[string]any, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT profile FROM agent_patch_profiles WHERE agent_id=$1
	`, agentID).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (r *PostgresAgentPatchProfileRepository) Upsert(ctx context.Context, agentID uuid.UUID, profile map[string]any) error {
	if profile == nil {
		profile = map[string]any{}
	}
	b, _ := json.Marshal(profile)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO agent_patch_profiles (agent_id, profile, updated_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (agent_id) DO UPDATE SET profile=EXCLUDED.profile, updated_at=EXCLUDED.updated_at
	`, agentID, b, time.Now())
	return err
}

