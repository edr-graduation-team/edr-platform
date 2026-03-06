// Package api provides unit tests for the API client.
package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	assert.Equal(t, "https://localhost:8443", cfg.BaseURL)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 3, cfg.RetryAttempts)
	assert.Equal(t, 1*time.Second, cfg.RetryDelay)
}

func TestTokenManager(t *testing.T) {
	tm := NewTokenManager()

	// Initially empty
	assert.Empty(t, tm.GetAccessToken())
	assert.Empty(t, tm.GetRefreshToken())
	assert.True(t, tm.IsExpired())

	// Set tokens
	expiry := time.Now().Add(1 * time.Hour)
	tm.SetTokens("access123", "refresh456", expiry)

	assert.Equal(t, "access123", tm.GetAccessToken())
	assert.Equal(t, "refresh456", tm.GetRefreshToken())
	assert.False(t, tm.IsExpired())
}

func TestTokenManagerExpired(t *testing.T) {
	tm := NewTokenManager()

	// Set expired token
	expiry := time.Now().Add(-1 * time.Hour)
	tm.SetTokens("expired", "refresh", expiry)

	assert.True(t, tm.IsExpired())
}

func TestTokenManagerExpiringSoon(t *testing.T) {
	tm := NewTokenManager()

	// Set token expiring in 10 seconds (should be considered expired)
	expiry := time.Now().Add(10 * time.Second)
	tm.SetTokens("expiring", "refresh", expiry)

	// Should be expired because we check 30 seconds before
	assert.True(t, tm.IsExpired())
}

func TestNewClientWithoutTLS(t *testing.T) {
	cfg := ClientConfig{
		BaseURL:       "https://localhost:8443",
		Timeout:       10 * time.Second,
		RetryAttempts: 2,
		RetryDelay:    500 * time.Millisecond,
	}

	client, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	err = client.Close()
	assert.NoError(t, err)
}

func TestAlertResponse(t *testing.T) {
	alert := AlertResponse{
		ID:           "alert-123",
		Timestamp:    time.Now(),
		AgentID:      "agent-456",
		RuleID:       "rule-789",
		RuleTitle:    "Suspicious Process",
		Severity:     "high",
		Category:     "process_creation",
		EventCount:   3,
		MitreTactics: []string{"Execution"},
		Status:       "open",
		Confidence:   0.85,
	}

	assert.Equal(t, "alert-123", alert.ID)
	assert.Equal(t, "high", alert.Severity)
	assert.Equal(t, 3, alert.EventCount)
	assert.Equal(t, 0.85, alert.Confidence)
}

func TestRuleResponse(t *testing.T) {
	rule := RuleResponse{
		ID:          "rule-123",
		Title:       "Suspicious PowerShell",
		Description: "Detects suspicious PowerShell usage",
		Content:     "title: Test\nstatus: stable",
		Enabled:     true,
		Status:      "stable",
		Product:     "windows",
		Category:    "process_creation",
		Severity:    "high",
		Source:      "official",
	}

	assert.Equal(t, "rule-123", rule.ID)
	assert.True(t, rule.Enabled)
	assert.Equal(t, "stable", rule.Status)
	assert.Equal(t, "official", rule.Source)
}

func TestLoadFromEnv(t *testing.T) {
	// Just test that it doesn't panic
	cfg := LoadFromEnv()
	assert.NotEmpty(t, cfg.BaseURL)
	assert.Greater(t, cfg.Timeout, time.Duration(0))
}
