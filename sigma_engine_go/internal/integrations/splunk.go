// Package integrations provides Splunk HEC integration.
package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// SplunkConfig holds Splunk HEC configuration.
type SplunkConfig struct {
	Enabled     bool   `json:"enabled"`
	HECEndpoint string `json:"hec_endpoint"` // https://splunk:8088
	HECToken    string `json:"hec_token"`
	Index       string `json:"index"`
	Source      string `json:"source"`
	SourceType  string `json:"sourcetype"`
	VerifySSL   bool   `json:"verify_ssl"`
	BatchSize   int    `json:"batch_size"`
	FlushSecs   int    `json:"flush_seconds"`
}

// DefaultSplunkConfig returns default settings.
func DefaultSplunkConfig() SplunkConfig {
	return SplunkConfig{
		Enabled:    false,
		Index:      "sigma",
		Source:     "sigma:engine",
		SourceType: "_json",
		VerifySSL:  true,
		BatchSize:  100,
		FlushSecs:  5,
	}
}

// SplunkEvent is the HEC event format.
type SplunkEvent struct {
	Event      interface{} `json:"event"`
	Time       int64       `json:"time,omitempty"`
	Source     string      `json:"source,omitempty"`
	SourceType string      `json:"sourcetype,omitempty"`
	Index      string      `json:"index,omitempty"`
	Host       string      `json:"host,omitempty"`
}

// SplunkAlertEvent is the alert payload for Splunk.
type SplunkAlertEvent struct {
	AlertID         string                 `json:"alert_id"`
	Timestamp       string                 `json:"timestamp"`
	Severity        string                 `json:"severity"`
	RuleID          string                 `json:"rule_id"`
	RuleName        string                 `json:"rule_name"`
	AgentID         string                 `json:"agent_id"`
	Category        string                 `json:"category"`
	Message         string                 `json:"message"`
	Status          string                 `json:"status"`
	Confidence      float64                `json:"confidence"`
	EventData       map[string]interface{} `json:"event_data,omitempty"`
	MITRETactics    []string               `json:"mitre_tactics,omitempty"`
	MITRETechniques []string               `json:"mitre_techniques,omitempty"`
}

// SplunkIntegration manages Splunk HEC delivery.
type SplunkIntegration struct {
	mu         sync.RWMutex
	config     SplunkConfig
	client     *http.Client
	buffer     []SplunkEvent
	lastFlush  time.Time
	connected  bool
	lastError  string
	eventsSent int64
}

// NewSplunkIntegration creates a new Splunk integration.
func NewSplunkIntegration() *SplunkIntegration {
	return &SplunkIntegration{
		config:    DefaultSplunkConfig(),
		buffer:    make([]SplunkEvent, 0, 100),
		lastFlush: time.Now(),
	}
}

// Configure updates the Splunk configuration.
func (s *SplunkIntegration) Configure(config SplunkConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create HTTP client with appropriate TLS settings
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: !config.VerifySSL,
		},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	s.client = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	s.config = config
	logger.Infof("Splunk integration configured: %s", config.HECEndpoint)
	return nil
}

// GetConfig returns current configuration.
func (s *SplunkIntegration) GetConfig() SplunkConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetStatus returns integration status.
func (s *SplunkIntegration) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"enabled":     s.config.Enabled,
		"connected":   s.connected,
		"last_error":  s.lastError,
		"events_sent": s.eventsSent,
		"last_flush":  s.lastFlush,
		"buffer_size": len(s.buffer),
	}
}

// TestConnection verifies Splunk connectivity.
func (s *SplunkIntegration) TestConnection(ctx context.Context) error {
	s.mu.RLock()
	config := s.config
	client := s.client
	s.mu.RUnlock()

	if config.HECEndpoint == "" || config.HECToken == "" {
		return fmt.Errorf("Splunk not configured")
	}

	// Send test event
	testEvent := SplunkEvent{
		Event: map[string]interface{}{
			"message": "Sigma Engine connection test",
			"test":    true,
		},
		Source:     config.Source,
		SourceType: config.SourceType,
		Index:      config.Index,
	}

	data, _ := json.Marshal(testEvent)
	req, err := http.NewRequestWithContext(ctx, "POST", config.HECEndpoint+"/services/collector", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Splunk "+config.HECToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		s.mu.Lock()
		s.connected = false
		s.lastError = err.Error()
		s.mu.Unlock()
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.mu.Lock()
		s.connected = false
		s.lastError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		s.mu.Unlock()
		return fmt.Errorf("HEC returned %d: %s", resp.StatusCode, string(body))
	}

	s.mu.Lock()
	s.connected = true
	s.lastError = ""
	s.mu.Unlock()

	logger.Info("Splunk connection test successful")
	return nil
}

// SendAlert queues an alert for delivery.
func (s *SplunkIntegration) SendAlert(alert *domain.Alert) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		return
	}

	event := s.transformAlert(alert)
	s.buffer = append(s.buffer, event)

	// Flush if buffer is full or time elapsed
	if len(s.buffer) >= s.config.BatchSize || time.Since(s.lastFlush) > time.Duration(s.config.FlushSecs)*time.Second {
		go s.flush()
	}
}

// transformAlert converts an alert to Splunk event format.
func (s *SplunkIntegration) transformAlert(alert *domain.Alert) SplunkEvent {
	alertEvent := SplunkAlertEvent{
		AlertID:         alert.ID,
		Timestamp:       alert.Timestamp.Format(time.RFC3339),
		Severity:        alert.Severity.String(),
		RuleID:          alert.RuleID,
		RuleName:        alert.RuleTitle,
		Category:        string(alert.EventCategory),
		Message:         alert.RuleTitle,
		Status:          "open",
		Confidence:      alert.Confidence,
		EventData:       alert.EventData,
		MITRETactics:    alert.MITRETactics,
		MITRETechniques: alert.MITRETechniques,
	}

	return SplunkEvent{
		Event:      alertEvent,
		Time:       alert.Timestamp.Unix(),
		Source:     s.config.Source,
		SourceType: s.config.SourceType,
		Index:      s.config.Index,
	}
}

// flush sends buffered events to Splunk.
func (s *SplunkIntegration) flush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}

	events := make([]SplunkEvent, len(s.buffer))
	copy(events, s.buffer)
	s.buffer = s.buffer[:0]
	s.lastFlush = time.Now()
	config := s.config
	client := s.client
	s.mu.Unlock()

	// Build batch payload (newline-delimited JSON)
	var payload bytes.Buffer
	for _, event := range events {
		data, _ := json.Marshal(event)
		payload.Write(data)
		payload.WriteByte('\n')
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", config.HECEndpoint+"/services/collector", &payload)
	if err != nil {
		logger.Errorf("Failed to create Splunk request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Splunk "+config.HECToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.connected = false
		s.mu.Unlock()
		logger.Errorf("Splunk delivery failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		s.mu.Lock()
		s.eventsSent += int64(len(events))
		s.connected = true
		s.lastError = ""
		s.mu.Unlock()
		logger.Debugf("Sent %d events to Splunk", len(events))
	} else {
		body, _ := io.ReadAll(resp.Body)
		s.mu.Lock()
		s.lastError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		s.mu.Unlock()
		logger.Warnf("Splunk returned %d: %s", resp.StatusCode, string(body))
	}
}

// StartBackgroundFlush starts periodic flushing.
func (s *SplunkIntegration) StartBackgroundFlush(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Duration(s.config.FlushSecs) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.flush() // Final flush
				return
			case <-ticker.C:
				s.flush()
			}
		}
	}()
}
