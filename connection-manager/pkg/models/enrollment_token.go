package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EnrollmentToken represents a dynamic, reusable token for agent enrollment.
// Unlike InstallationToken (one-time-use), enrollment tokens can be used
// multiple times (optionally capped by MaxUses), have descriptions, and
// can be revoked via the Dashboard.
type EnrollmentToken struct {
	ID          uuid.UUID  `db:"id"          json:"id"`
	Token       string     `db:"token"       json:"token"`
	Description string     `db:"description" json:"description"`
	IsActive    bool       `db:"is_active"   json:"is_active"`
	ExpiresAt   *time.Time `db:"expires_at"  json:"expires_at"` // nil = never expires
	UseCount    int        `db:"use_count"   json:"use_count"`
	MaxUses     *int       `db:"max_uses"    json:"max_uses"` // nil = unlimited
	CreatedBy   string     `db:"created_by"  json:"created_by"`
	CreatedAt   time.Time  `db:"created_at"  json:"created_at"`
	RevokedAt   *time.Time `db:"revoked_at"  json:"revoked_at"`
	UpdatedAt   time.Time  `db:"updated_at"  json:"updated_at"`
}

// IsValid returns true if the token can be used for enrollment:
// - Must be active (not revoked)
// - Must not be expired
// - Must not have exceeded max uses
func (t *EnrollmentToken) IsValid() bool {
	if !t.IsActive {
		return false
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return false
	}
	if t.MaxUses != nil && t.UseCount >= *t.MaxUses {
		return false
	}
	return true
}

// GenerateSecureToken creates a cryptographically secure random token string.
// Returns a 64-character hex string (32 bytes of entropy).
func GenerateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}
