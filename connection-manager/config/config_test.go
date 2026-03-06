// Package config provides unit tests for configuration.
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_port: 50051
  http_port: 8090
  tls_cert_path: "/path/to/cert.pem"
  tls_key_path: "/path/to/key.pem"
  ca_cert_path: "/path/to/ca.pem"

database:
  host: "localhost"
  port: 5432
  user: "edr"
  password: "secret"
  name: "edr"
  ssl_mode: "disable"

redis:
  addr: "localhost:6379"
  db: 0

jwt:
  private_key_path: "/path/to/private.pem"
  public_key_path: "/path/to/public.pem"
  issuer: "edr-server"
  audience: "edr-agents"

rate_limit:
  enabled: true
  events_per_second: 10000

logging:
  level: "info"
  format: "json"

monitoring:
  enabled: true
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Verify values
	assert.Equal(t, 50051, cfg.Server.GRPCPort)
	assert.Equal(t, 8090, cfg.Server.HTTPPort)
	assert.Equal(t, "/path/to/cert.pem", cfg.Server.TLSCertPath)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "edr", cfg.Database.User)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "edr-server", cfg.JWT.Issuer)
	assert.True(t, cfg.RateLimit.Enabled)
	assert.Equal(t, 10000, cfg.RateLimit.EventsPerSecond)
	assert.Equal(t, "info", cfg.Logging.Level)
}

func TestLoadWithEnvOverrides(t *testing.T) {
	// Create minimal config file with all required fields
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	minimalConfig := `
server:
  grpc_port: 50051
  tls_cert_path: "/path/to/cert.pem"
  tls_key_path: "/path/to/key.pem"
  ca_cert_path: "/path/to/ca.pem"
database:
  host: "localhost"
  user: "edr"
  password: "secret"
  name: "edr"
jwt:
  private_key_path: "/path/to/private.pem"
  public_key_path: "/path/to/public.pem"
logging:
  level: "info"
`
	err := os.WriteFile(configPath, []byte(minimalConfig), 0644)
	require.NoError(t, err)

	// Set environment variables
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Environment variables should override file values
	assert.Equal(t, "debug", cfg.Logging.Level)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}
