// Package integrations provides external system integrations for alert delivery.
package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// WebhookConfig defines a webhook configuration.
type WebhookConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Enabled     bool              `json:"enabled"`
	Filters     WebhookFilters    `json:"filters"`
	RetryPolicy RetryPolicy       `json:"retry_policy"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// WebhookFilters defines alert filtering for webhooks.
type WebhookFilters struct {
	Severities []string `json:"severities"` // critical, high, medium, low
	RuleIDs    []string `json:"rule_ids"`
	AgentIDs   []string `json:"agent_ids"`
	Categories []string `json:"categories"`
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	TimeoutSeconds int           `json:"timeout_seconds"`
}

// DefaultRetryPolicy returns standard retry settings.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     5,
		InitialBackoff: time.Second,
		MaxBackoff:     16 * time.Second,
		TimeoutSeconds: 10,
	}
}

// WebhookPayload is the JSON payload sent to webhooks.
type WebhookPayload struct {
	AlertID   string                 `json:"alert_id"`
	Timestamp time.Time              `json:"timestamp"`
	Severity  string                 `json:"severity"`
	RuleID    string                 `json:"rule_id"`
	RuleName  string                 `json:"rule_name"`
	AgentID   string                 `json:"agent_id"`
	Message   string                 `json:"message"`
	Status    string                 `json:"status"`
	Details   map[string]interface{} `json:"details"`
}

// DeliveryStatus represents webhook delivery state.
type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

// DeliveryLog tracks webhook delivery attempts.
type DeliveryLog struct {
	ID          string         `json:"id"`
	WebhookID   string         `json:"webhook_id"`
	AlertID     string         `json:"alert_id"`
	Status      DeliveryStatus `json:"status"`
	StatusCode  int            `json:"status_code"`
	Attempts    int            `json:"attempts"`
	Error       string         `json:"error,omitempty"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// WebhookManager manages webhook configurations and delivery.
type WebhookManager struct {
	mu       sync.RWMutex
	webhooks map[string]*WebhookConfig
	logs     []DeliveryLog
	client   *http.Client
}

// NewWebhookManager creates a new webhook manager.
func NewWebhookManager() *WebhookManager {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &WebhookManager{
		webhooks: make(map[string]*WebhookConfig),
		logs:     make([]DeliveryLog, 0),
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// CreateWebhook adds a new webhook configuration.
func (m *WebhookManager) CreateWebhook(config WebhookConfig) (*WebhookConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if config.ID == "" {
		config.ID = uuid.New().String()
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	if config.RetryPolicy.MaxRetries == 0 {
		config.RetryPolicy = DefaultRetryPolicy()
	}

	m.webhooks[config.ID] = &config
	logger.Infof("Created webhook: %s (%s)", config.Name, config.ID)
	return &config, nil
}

// GetWebhook retrieves a webhook by ID.
func (m *WebhookManager) GetWebhook(id string) (*WebhookConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	webhook, exists := m.webhooks[id]
	if !exists {
		return nil, fmt.Errorf("webhook not found: %s", id)
	}
	return webhook, nil
}

// ListWebhooks returns all webhooks.
func (m *WebhookManager) ListWebhooks() []*WebhookConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*WebhookConfig, 0, len(m.webhooks))
	for _, w := range m.webhooks {
		list = append(list, w)
	}
	return list
}

// UpdateWebhook updates an existing webhook.
func (m *WebhookManager) UpdateWebhook(id string, config WebhookConfig) (*WebhookConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.webhooks[id]
	if !exists {
		return nil, fmt.Errorf("webhook not found: %s", id)
	}

	config.ID = id
	config.CreatedAt = existing.CreatedAt
	config.UpdatedAt = time.Now()
	m.webhooks[id] = &config
	return &config, nil
}

// DeleteWebhook removes a webhook.
func (m *WebhookManager) DeleteWebhook(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.webhooks[id]; !exists {
		return fmt.Errorf("webhook not found: %s", id)
	}
	delete(m.webhooks, id)
	logger.Infof("Deleted webhook: %s", id)
	return nil
}

// DeliverAlert sends an alert to all matching webhooks.
func (m *WebhookManager) DeliverAlert(ctx context.Context, alert *domain.Alert) {
	m.mu.RLock()
	webhooks := make([]*WebhookConfig, 0)
	for _, w := range m.webhooks {
		if w.Enabled && m.matchesFilters(alert, w.Filters) {
			webhooks = append(webhooks, w)
		}
	}
	m.mu.RUnlock()

	// Deliver to all matching webhooks concurrently
	for _, webhook := range webhooks {
		go m.deliverToWebhook(ctx, webhook, alert)
	}
}

// matchesFilters checks if an alert matches webhook filters.
func (m *WebhookManager) matchesFilters(alert *domain.Alert, filters WebhookFilters) bool {
	// Check severity filter
	if len(filters.Severities) > 0 {
		matched := false
		for _, s := range filters.Severities {
			if s == alert.Severity.String() {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check rule ID filter
	if len(filters.RuleIDs) > 0 {
		matched := false
		for _, r := range filters.RuleIDs {
			if r == alert.RuleID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// deliverToWebhook sends an alert to a specific webhook with retry.
func (m *WebhookManager) deliverToWebhook(ctx context.Context, webhook *WebhookConfig, alert *domain.Alert) {
	payload := m.buildPayload(alert)
	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("Failed to marshal webhook payload: %v", err)
		return
	}

	log := DeliveryLog{
		ID:        uuid.New().String(),
		WebhookID: webhook.ID,
		AlertID:   alert.ID,
		Status:    DeliveryPending,
		CreatedAt: time.Now(),
	}

	var lastErr error
	for attempt := 0; attempt <= webhook.RetryPolicy.MaxRetries; attempt++ {
		log.Attempts = attempt + 1

		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * webhook.RetryPolicy.InitialBackoff
			if backoff > webhook.RetryPolicy.MaxBackoff {
				backoff = webhook.RetryPolicy.MaxBackoff
			}
			time.Sleep(backoff)
		}

		statusCode, err := m.sendRequest(ctx, webhook, jsonData)
		log.StatusCode = statusCode

		if err == nil && statusCode >= 200 && statusCode < 300 {
			log.Status = DeliveryDelivered
			now := time.Now()
			log.DeliveredAt = &now
			m.saveLog(log)
			logger.Debugf("Webhook delivered: %s to %s", alert.ID, webhook.URL)
			return
		}

		lastErr = err
		if err != nil {
			log.Error = err.Error()
		} else {
			log.Error = fmt.Sprintf("HTTP %d", statusCode)
		}

		// Don't retry on 4xx errors (except 429)
		if statusCode >= 400 && statusCode < 500 && statusCode != 429 {
			break
		}
	}

	log.Status = DeliveryFailed
	if lastErr != nil {
		log.Error = lastErr.Error()
	}
	m.saveLog(log)
	logger.Warnf("Webhook delivery failed after %d attempts: %s", log.Attempts, log.Error)
}

// sendRequest sends the HTTP request.
func (m *WebhookManager) sendRequest(ctx context.Context, webhook *WebhookConfig, body []byte) (int, error) {
	timeout := time.Duration(webhook.RetryPolicy.TimeoutSeconds) * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", webhook.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sigma-Engine/1.0")
	for k, v := range webhook.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

// buildPayload creates the webhook payload from an alert.
func (m *WebhookManager) buildPayload(alert *domain.Alert) WebhookPayload {
	return WebhookPayload{
		AlertID:   alert.ID,
		Timestamp: alert.Timestamp,
		Severity:  alert.Severity.String(),
		RuleID:    alert.RuleID,
		RuleName:  alert.RuleTitle,
		AgentID:   "", // Set from context
		Message:   alert.RuleTitle,
		Status:    "open",
		Details:   alert.EventData,
	}
}

// saveLog stores a delivery log.
func (m *WebhookManager) saveLog(log DeliveryLog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, log)

	// Keep only last 1000 logs
	if len(m.logs) > 1000 {
		m.logs = m.logs[len(m.logs)-1000:]
	}
}

// GetLogs retrieves delivery logs for a webhook.
func (m *WebhookManager) GetLogs(webhookID string, limit int) []DeliveryLog {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]DeliveryLog, 0)
	for i := len(m.logs) - 1; i >= 0 && len(result) < limit; i-- {
		if m.logs[i].WebhookID == webhookID {
			result = append(result, m.logs[i])
		}
	}
	return result
}

// TestWebhook sends a test payload to verify connectivity.
func (m *WebhookManager) TestWebhook(ctx context.Context, webhook *WebhookConfig) error {
	testPayload := WebhookPayload{
		AlertID:   "test-" + uuid.New().String(),
		Timestamp: time.Now(),
		Severity:  "low",
		RuleID:    "test-rule",
		RuleName:  "Webhook Connectivity Test",
		AgentID:   "test-agent",
		Message:   "This is a test message from Sigma Engine",
		Status:    "test",
		Details:   map[string]interface{}{"test": true},
	}

	jsonData, _ := json.Marshal(testPayload)
	statusCode, err := m.sendRequest(ctx, webhook, jsonData)
	if err != nil {
		return fmt.Errorf("test delivery failed: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("test delivery returned HTTP %d", statusCode)
	}
	return nil
}
