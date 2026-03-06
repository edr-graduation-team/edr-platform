// Package models provides unit tests for domain models.
package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCertificate_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "active and not expired",
			status:    CertStatusActive,
			expiresAt: time.Now().Add(30 * 24 * time.Hour),
			want:      true,
		},
		{
			name:      "active but expired",
			status:    CertStatusActive,
			expiresAt: time.Now().Add(-24 * time.Hour),
			want:      false,
		},
		{
			name:      "revoked",
			status:    CertStatusRevoked,
			expiresAt: time.Now().Add(30 * 24 * time.Hour),
			want:      false,
		},
		{
			name:      "superseded",
			status:    CertStatusSuperseded,
			expiresAt: time.Now().Add(30 * 24 * time.Hour),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &Certificate{
				Status:    tt.status,
				ExpiresAt: tt.expiresAt,
			}
			assert.Equal(t, tt.want, cert.IsValid())
		})
	}
}

func TestCertificate_IsExpiringSoon(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		within    time.Duration
		want      bool
	}{
		{
			name:      "expires in 5 days, check 7 days",
			expiresAt: time.Now().Add(5 * 24 * time.Hour),
			within:    7 * 24 * time.Hour,
			want:      true,
		},
		{
			name:      "expires in 30 days, check 7 days",
			expiresAt: time.Now().Add(30 * 24 * time.Hour),
			within:    7 * 24 * time.Hour,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &Certificate{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.want, cert.IsExpiringSoon(tt.within))
		})
	}
}

func TestGenerateFingerprint(t *testing.T) {
	certBytes := []byte("test certificate content")
	fingerprint := GenerateFingerprint(certBytes)

	// SHA256 produces 64 character hex string
	assert.Len(t, fingerprint, 64)

	// Same input should produce same output
	fingerprint2 := GenerateFingerprint(certBytes)
	assert.Equal(t, fingerprint, fingerprint2)

	// Different input should produce different output
	fingerprint3 := GenerateFingerprint([]byte("different content"))
	assert.NotEqual(t, fingerprint, fingerprint3)
}

func TestInstallationToken_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		used      bool
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "unused and not expired",
			used:      false,
			expiresAt: time.Now().Add(24 * time.Hour),
			want:      true,
		},
		{
			name:      "unused but expired",
			used:      false,
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      false,
		},
		{
			name:      "used",
			used:      true,
			expiresAt: time.Now().Add(24 * time.Hour),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &InstallationToken{
				Used:      tt.used,
				ExpiresAt: tt.expiresAt,
			}
			assert.Equal(t, tt.want, token.IsValid())
		})
	}
}
