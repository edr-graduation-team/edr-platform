// Package api provides HTTP client for connection-manager communication.
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// ClientConfig configures the connection-manager API client.
type ClientConfig struct {
	BaseURL        string        `yaml:"base_url"`
	ClientCertPath string        `yaml:"client_cert_path"`
	ClientKeyPath  string        `yaml:"client_key_path"`
	CACertPath     string        `yaml:"ca_cert_path"`
	Timeout        time.Duration `yaml:"timeout"`
	RetryAttempts  int           `yaml:"retry_attempts"`
	RetryDelay     time.Duration `yaml:"retry_delay"`
}

// DefaultClientConfig returns default API client configuration.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		BaseURL:       "https://localhost:8443",
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
	}
}

// LoadFromEnv loads client configuration from environment variables.
func LoadFromEnv() ClientConfig {
	cfg := DefaultClientConfig()

	if v := os.Getenv("CONNECTION_MANAGER_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("SIGMA_CLIENT_CERT"); v != "" {
		cfg.ClientCertPath = v
	}
	if v := os.Getenv("SIGMA_CLIENT_KEY"); v != "" {
		cfg.ClientKeyPath = v
	}
	if v := os.Getenv("SIGMA_CA_CERT"); v != "" {
		cfg.CACertPath = v
	}

	return cfg
}

// TokenManager manages JWT tokens for API authentication.
type TokenManager struct {
	accessToken  string
	refreshToken string
	expiresAt    time.Time
	mu           sync.RWMutex
}

// NewTokenManager creates a new token manager.
func NewTokenManager() *TokenManager {
	return &TokenManager{}
}

// SetTokens updates the stored tokens.
func (tm *TokenManager) SetTokens(access, refresh string, expiresAt time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.accessToken = access
	tm.refreshToken = refresh
	tm.expiresAt = expiresAt
}

// GetAccessToken returns the current access token.
func (tm *TokenManager) GetAccessToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.accessToken
}

// GetRefreshToken returns the current refresh token.
func (tm *TokenManager) GetRefreshToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.refreshToken
}

// IsExpired returns whether the token is expired or about to expire.
func (tm *TokenManager) IsExpired() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	// Consider expired 30 seconds before actual expiry
	return time.Now().Add(30 * time.Second).After(tm.expiresAt)
}

// Client provides HTTP client for connection-manager API.
type Client struct {
	config       ClientConfig
	httpClient   *http.Client
	tokenManager *TokenManager
}

// NewClient creates a new connection-manager API client.
func NewClient(config ClientConfig) (*Client, error) {
	// Build TLS config
	tlsConfig, err := buildTLSConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	logger.Infof("Created connection-manager API client: %s", config.BaseURL)

	return &Client{
		config:       config,
		httpClient:   httpClient,
		tokenManager: NewTokenManager(),
	}, nil
}

// buildTLSConfig creates TLS configuration for mTLS.
func buildTLSConfig(config ClientConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load client certificate if provided
	if config.ClientCertPath != "" && config.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		logger.Info("Loaded mTLS client certificate")
	}

	// Load CA certificate if provided
	if config.CACertPath != "" {
		caCert, err := os.ReadFile(config.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
		logger.Info("Loaded CA certificate")
	} else {
		// Allow insecure for development (no CA cert)
		tlsConfig.InsecureSkipVerify = true
	}

	return tlsConfig, nil
}

// doRequest executes an HTTP request with retry logic.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.config.BaseURL + path

	var lastErr error
	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(c.config.RetryDelay * time.Duration(attempt))
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		// Add authorization if we have a token
		if token := c.tokenManager.GetAccessToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			logger.Warnf("Request failed (attempt %d): %v", attempt+1, err)
			continue
		}

		// Retry on 5xx errors
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// HealthCheck checks connection-manager health.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}

	return nil
}

// Close closes the client.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// =============================================================================
// Alert API Methods
// =============================================================================

// AlertResponse represents an alert response from the API.
type AlertResponse struct {
	ID              string                 `json:"id"`
	Timestamp       time.Time              `json:"timestamp"`
	AgentID         string                 `json:"agent_id"`
	RuleID          string                 `json:"rule_id"`
	RuleTitle       string                 `json:"rule_title"`
	Severity        string                 `json:"severity"`
	Category        string                 `json:"category"`
	EventCount      int                    `json:"event_count"`
	MitreTactics    []string               `json:"mitre_tactics"`
	MitreTechniques []string               `json:"mitre_techniques"`
	MatchedFields   map[string]interface{} `json:"matched_fields"`
	Status          string                 `json:"status"`
	Confidence      float64                `json:"confidence"`
	CreatedAt       time.Time              `json:"created_at"`
}

// GetAlert retrieves an alert by ID.
func (c *Client) GetAlert(ctx context.Context, id string) (*AlertResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/alerts/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get alert failed: %d", resp.StatusCode)
	}

	var alert AlertResponse
	if err := json.NewDecoder(resp.Body).Decode(&alert); err != nil {
		return nil, fmt.Errorf("failed to decode alert: %w", err)
	}

	return &alert, nil
}

// =============================================================================
// Rule API Methods
// =============================================================================

// RuleResponse represents a rule response from the API.
type RuleResponse struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Content         string    `json:"content"`
	Enabled         bool      `json:"enabled"`
	Status          string    `json:"status"`
	Product         string    `json:"product"`
	Category        string    `json:"category"`
	Severity        string    `json:"severity"`
	MitreTactics    []string  `json:"mitre_tactics"`
	MitreTechniques []string  `json:"mitre_techniques"`
	Source          string    `json:"source"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GetRules retrieves all rules from connection-manager.
func (c *Client) GetRules(ctx context.Context) ([]*RuleResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/sigma/rules", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get rules failed: %d", resp.StatusCode)
	}

	var rules []*RuleResponse
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode rules: %w", err)
	}

	return rules, nil
}

// GetRule retrieves a rule by ID.
func (c *Client) GetRule(ctx context.Context, id string) (*RuleResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/sigma/rules/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get rule failed: %d", resp.StatusCode)
	}

	var rule RuleResponse
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("failed to decode rule: %w", err)
	}

	return &rule, nil
}

// Ensure interface compliance
var _ io.Closer = (*Client)(nil)
