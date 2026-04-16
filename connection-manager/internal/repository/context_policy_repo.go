package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

type PostgresContextPolicyRepository struct {
	db *pgxpool.Pool
}

func NewPostgresContextPolicyRepository(db *pgxpool.Pool) *PostgresContextPolicyRepository {
	return &PostgresContextPolicyRepository{db: db}
}

func (r *PostgresContextPolicyRepository) List(ctx context.Context) ([]*models.ContextPolicy, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, scope_type, scope_value, enabled,
		       user_role_weight, device_criticality_weight, network_anomaly_factor,
		       trusted_networks, COALESCE(notes, ''), created_at, updated_at
		FROM context_policies
		ORDER BY CASE WHEN scope_type = 'global' THEN 0 ELSE 1 END, name ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list context policies: %w", err)
	}
	defer rows.Close()

	var out []*models.ContextPolicy
	for rows.Next() {
		p := &models.ContextPolicy{}
		var trustedJSON []byte
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ScopeType, &p.ScopeValue, &p.Enabled,
			&p.UserRoleWeight, &p.DeviceCriticalityWeight, &p.NetworkAnomalyFactor,
			&trustedJSON, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan context policy: %w", err)
		}
		if len(trustedJSON) > 0 {
			_ = json.Unmarshal(trustedJSON, &p.TrustedNetworks)
		}
		out = append(out, p)
	}
	if out == nil {
		out = []*models.ContextPolicy{}
	}
	return out, nil
}

func (r *PostgresContextPolicyRepository) GetByID(ctx context.Context, id int64) (*models.ContextPolicy, error) {
	p := &models.ContextPolicy{}
	var trustedJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, name, scope_type, scope_value, enabled,
		       user_role_weight, device_criticality_weight, network_anomaly_factor,
		       trusted_networks, COALESCE(notes, ''), created_at, updated_at
		FROM context_policies
		WHERE id = $1`, id).
		Scan(
			&p.ID, &p.Name, &p.ScopeType, &p.ScopeValue, &p.Enabled,
			&p.UserRoleWeight, &p.DeviceCriticalityWeight, &p.NetworkAnomalyFactor,
			&trustedJSON, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get context policy: %w", err)
	}
	if len(trustedJSON) > 0 {
		_ = json.Unmarshal(trustedJSON, &p.TrustedNetworks)
	}
	return p, nil
}

func (r *PostgresContextPolicyRepository) Create(ctx context.Context, policy *models.ContextPolicy) error {
	trusted, _ := json.Marshal(policy.TrustedNetworks)
	// One-policy-per-scope hardening:
	// If a policy already exists for (scope_type, scope_value), we REPLACE it by
	// updating fields. This matches the UNIQUE constraint created by migration 017.
	//
	// This makes Create idempotent for clients and avoids ambiguous “stacking”
	// semantics that would multiply factors in unexpected ways.
	return r.db.QueryRow(ctx, `
		INSERT INTO context_policies (
			name, scope_type, scope_value, enabled,
			user_role_weight, device_criticality_weight, network_anomaly_factor,
			trusted_networks, notes
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8::jsonb, $9
		)
		ON CONFLICT (scope_type, scope_value) DO UPDATE SET
		    name = EXCLUDED.name,
		    enabled = EXCLUDED.enabled,
		    user_role_weight = EXCLUDED.user_role_weight,
		    device_criticality_weight = EXCLUDED.device_criticality_weight,
		    network_anomaly_factor = EXCLUDED.network_anomaly_factor,
		    trusted_networks = EXCLUDED.trusted_networks,
		    notes = EXCLUDED.notes,
		    updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		policy.Name, policy.ScopeType, policy.ScopeValue, policy.Enabled,
		policy.UserRoleWeight, policy.DeviceCriticalityWeight, policy.NetworkAnomalyFactor,
		string(trusted), policy.Notes,
	).Scan(&policy.ID, &policy.CreatedAt, &policy.UpdatedAt)
}

func (r *PostgresContextPolicyRepository) Update(ctx context.Context, policy *models.ContextPolicy) error {
	trusted, _ := json.Marshal(policy.TrustedNetworks)
	res, err := r.db.Exec(ctx, `
		UPDATE context_policies
		SET name = $2,
		    scope_type = $3,
		    scope_value = $4,
		    enabled = $5,
		    user_role_weight = $6,
		    device_criticality_weight = $7,
		    network_anomaly_factor = $8,
		    trusted_networks = $9::jsonb,
		    notes = $10,
		    updated_at = NOW()
		WHERE id = $1`,
		policy.ID, policy.Name, policy.ScopeType, policy.ScopeValue, policy.Enabled,
		policy.UserRoleWeight, policy.DeviceCriticalityWeight, policy.NetworkAnomalyFactor,
		string(trusted), policy.Notes,
	)
	if err != nil {
		return fmt.Errorf("failed to update context policy: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresContextPolicyRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.Exec(ctx, `DELETE FROM context_policies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete context policy: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
