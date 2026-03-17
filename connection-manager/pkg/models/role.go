// Package models defines the domain models for the connection-manager.
package models

import (
	"time"

	"github.com/google/uuid"
)

// Role represents an RBAC role in the system.
type Role struct {
	ID          uuid.UUID    `db:"id" json:"id"`
	Name        string       `db:"name" json:"name"`
	Description string       `db:"description" json:"description"`
	IsBuiltIn   bool         `db:"is_built_in" json:"is_built_in"`
	Permissions []Permission `json:"permissions,omitempty"` // populated by join query
	CreatedAt   time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time    `db:"updated_at" json:"updated_at"`
}

// Built-in role names — must match DB seed.
const (
	RoleNameAdmin      = "admin"
	RoleNameSecurity   = "security"
	RoleNameAnalyst    = "analyst"
	RoleNameOperations = "operations"
	RoleNameViewer     = "viewer"
)

// ValidRoleNames is the authoritative list of known role names.
var ValidRoleNames = []string{
	RoleNameAdmin,
	RoleNameSecurity,
	RoleNameAnalyst,
	RoleNameOperations,
	RoleNameViewer,
}

// Permission represents a granular permission (resource:action).
type Permission struct {
	ID          uuid.UUID `db:"id" json:"id"`
	Resource    string    `db:"resource" json:"resource"`
	Action      string    `db:"action" json:"action"`
	Description string    `db:"description" json:"description"`
}

// Key returns the "resource:action" string for comparison.
func (p Permission) Key() string {
	return p.Resource + ":" + p.Action
}
