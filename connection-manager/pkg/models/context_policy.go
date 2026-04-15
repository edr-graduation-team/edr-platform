package models

import "time"

// ContextPolicy controls context-aware risk weighting inputs.
// Scope model:
//   - global: applies platform-wide (scope_value should be "*")
//   - agent:  applies to a specific agent_id
//   - user:   applies to a specific username
type ContextPolicy struct {
	ID                      int64     `json:"id" db:"id"`
	Name                    string    `json:"name" db:"name"`
	ScopeType               string    `json:"scope_type" db:"scope_type"`
	ScopeValue              string    `json:"scope_value" db:"scope_value"`
	Enabled                 bool      `json:"enabled" db:"enabled"`
	UserRoleWeight          float64   `json:"user_role_weight" db:"user_role_weight"`
	DeviceCriticalityWeight float64   `json:"device_criticality_weight" db:"device_criticality_weight"`
	NetworkAnomalyFactor    float64   `json:"network_anomaly_factor" db:"network_anomaly_factor"`
	TrustedNetworks         []string  `json:"trusted_networks" db:"trusted_networks"`
	Notes                   string    `json:"notes,omitempty" db:"notes"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}
