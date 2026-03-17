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

// PostgresRoleRepository implements RoleRepository using PostgreSQL.
type PostgresRoleRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRoleRepository creates a new role repository.
func NewPostgresRoleRepository(pool *pgxpool.Pool) *PostgresRoleRepository {
	return &PostgresRoleRepository{pool: pool}
}

// ListRoles retrieves all roles with their granted permissions.
func (r *PostgresRoleRepository) ListRoles(ctx context.Context) ([]*models.Role, error) {
	roleQuery := `SELECT id, name, description, is_built_in, created_at, updated_at FROM roles ORDER BY
		CASE name
			WHEN 'admin'      THEN 1
			WHEN 'security'   THEN 2
			WHEN 'analyst'    THEN 3
			WHEN 'operations' THEN 4
			WHEN 'viewer'     THEN 5
			ELSE 6
		END, name`

	rows, err := r.pool.Query(ctx, roleQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	defer rows.Close()

	var roles []*models.Role
	for rows.Next() {
		role := &models.Role{}
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsBuiltIn, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, role)
	}

	// Populate permissions for each role
	for _, role := range roles {
		perms, err := r.getPermissionsForRole(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = perms
	}

	return roles, nil
}

// GetRoleByName retrieves a single role by name, including its permissions.
func (r *PostgresRoleRepository) GetRoleByName(ctx context.Context, name string) (*models.Role, error) {
	query := `SELECT id, name, description, is_built_in, created_at, updated_at FROM roles WHERE name = $1`

	role := &models.Role{}
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&role.ID, &role.Name, &role.Description, &role.IsBuiltIn, &role.CreatedAt, &role.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	perms, err := r.getPermissionsForRole(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	role.Permissions = perms

	return role, nil
}

// CreateRole creates a new custom role (is_built_in = false).
func (r *PostgresRoleRepository) CreateRole(ctx context.Context, role *models.Role) error {
	role.ID = uuid.New()
	role.IsBuiltIn = false
	now := time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO roles (id, name, description, is_built_in, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		role.ID, role.Name, role.Description, role.IsBuiltIn, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}
	return nil
}

// UpdateRolePermissions replaces the permission set for a given role.
func (r *PostgresRoleRepository) UpdateRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Remove existing permissions
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("failed to clear role permissions: %w", err)
	}

	// Insert new permissions
	for _, pid := range permissionIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			roleID, pid,
		); err != nil {
			return fmt.Errorf("failed to assign permission: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// DeleteRole deletes a custom role. Built-in roles cannot be deleted.
func (r *PostgresRoleRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx,
		`DELETE FROM roles WHERE id = $1 AND is_built_in = FALSE`, id,
	)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPermissions retrieves all available permissions.
func (r *PostgresRoleRepository) ListPermissions(ctx context.Context) ([]*models.Permission, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, resource, action, description FROM permissions ORDER BY resource, action`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list permissions: %w", err)
	}
	defer rows.Close()

	var perms []*models.Permission
	for rows.Next() {
		p := &models.Permission{}
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, nil
}

// GetPermissionsForRoleName returns "resource:action" keys for a named role.
func (r *PostgresRoleRepository) GetPermissionsForRoleName(ctx context.Context, roleName string) ([]string, error) {
	query := `
		SELECT p.resource, p.action
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN roles ro ON ro.id = rp.role_id
		WHERE ro.name = $1
		ORDER BY p.resource, p.action`

	rows, err := r.pool.Query(ctx, query, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions for role %q: %w", roleName, err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var resource, action string
		if err := rows.Scan(&resource, &action); err != nil {
			return nil, fmt.Errorf("failed to scan permission key: %w", err)
		}
		keys = append(keys, resource+":"+action)
	}
	return keys, nil
}

// getPermissionsForRole fetches Permission objects granted to a role by UUID.
func (r *PostgresRoleRepository) getPermissionsForRole(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	query := `
		SELECT p.id, p.resource, p.action, p.description
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = $1
		ORDER BY p.resource, p.action`

	rows, err := r.pool.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions: %w", err)
	}
	defer rows.Close()

	var perms []models.Permission
	for rows.Next() {
		p := models.Permission{}
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, nil
}
