// Package models provides unit tests for domain models.
package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUser_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"active user", UserStatusActive, true},
		{"inactive user", UserStatusInactive, false},
		{"locked user", UserStatusLocked, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Status: tt.status}
			assert.Equal(t, tt.want, user.IsActive())
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestUser_IsLocked(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		lockedUntil *time.Time
		want        bool
	}{
		{
			name:        "status locked",
			status:      UserStatusLocked,
			lockedUntil: nil,
			want:        true,
		},
		{
			name:        "temporarily locked",
			status:      UserStatusActive,
			lockedUntil: timePtr(time.Now().Add(1 * time.Hour)),
			want:        true,
		},
		{
			name:        "lock expired",
			status:      UserStatusActive,
			lockedUntil: timePtr(time.Now().Add(-1 * time.Hour)),
			want:        false,
		},
		{
			name:        "active and not locked",
			status:      UserStatusActive,
			lockedUntil: nil,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				Status:      tt.status,
				LockedUntil: tt.lockedUntil,
			}
			assert.Equal(t, tt.want, user.IsLocked())
		})
	}
}

func TestUser_HasRole(t *testing.T) {
	user := &User{Role: UserRoleAdmin}

	assert.True(t, user.HasRole(UserRoleAdmin))
	assert.False(t, user.HasRole(UserRoleAnalyst))
	assert.False(t, user.HasRole(UserRoleViewer))
}

func TestUser_IsAdmin(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{"admin user", UserRoleAdmin, true},
		{"analyst user", UserRoleAnalyst, false},
		{"viewer user", UserRoleViewer, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Role: tt.role}
			assert.Equal(t, tt.want, user.IsAdmin())
		})
	}
}

func TestUser_RecordFailedLogin(t *testing.T) {
	user := &User{
		LoginAttempts: 0,
		Status:        UserStatusActive,
	}

	// First few attempts don't lock
	user.RecordFailedLogin(5, 15*time.Minute)
	assert.Equal(t, 1, user.LoginAttempts)
	assert.Nil(t, user.LockedUntil)

	// Second attempt
	user.RecordFailedLogin(5, 15*time.Minute)
	assert.Equal(t, 2, user.LoginAttempts)
	assert.Nil(t, user.LockedUntil)

	// Third, fourth attempt
	user.RecordFailedLogin(5, 15*time.Minute)
	user.RecordFailedLogin(5, 15*time.Minute)
	assert.Equal(t, 4, user.LoginAttempts)
	assert.Nil(t, user.LockedUntil)

	// Fifth attempt should lock
	user.RecordFailedLogin(5, 15*time.Minute)
	assert.Equal(t, 5, user.LoginAttempts)
	assert.NotNil(t, user.LockedUntil)
	assert.True(t, user.LockedUntil.After(time.Now()))
}

func TestUser_RecordSuccessfulLogin(t *testing.T) {
	user := &User{
		LoginAttempts: 3,
		LockedUntil:   timePtr(time.Now().Add(1 * time.Hour)),
	}

	before := time.Now()
	user.RecordSuccessfulLogin()
	after := time.Now()

	assert.Equal(t, 0, user.LoginAttempts)
	assert.Nil(t, user.LockedUntil)
	assert.NotNil(t, user.LastLogin)
	assert.True(t, user.LastLogin.After(before) || user.LastLogin.Equal(before))
	assert.True(t, user.LastLogin.Before(after) || user.LastLogin.Equal(after))
}
