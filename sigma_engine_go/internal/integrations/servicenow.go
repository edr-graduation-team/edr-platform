// Package integrations provides ServiceNow integration.
package integrations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// ServiceNowConfig holds ServiceNow API configuration.
type ServiceNowConfig struct {
	Enabled         bool           `json:"enabled"`
	InstanceURL     string         `json:"instance_url"` // https://dev123456.service-now.com
	Username        string         `json:"username"`
	Password        string         `json:"password"`
	AssignmentGroup string         `json:"assignment_group"`
	TableName       string         `json:"table_name"` // incident
	AutoCreate      bool           `json:"auto_create"`
	SeverityMapping map[string]int `json:"severity_mapping"`
}

// DefaultServiceNowConfig returns default settings.
func DefaultServiceNowConfig() ServiceNowConfig {
	return ServiceNowConfig{
		Enabled:    false,
		TableName:  "incident",
		AutoCreate: true,
		SeverityMapping: map[string]int{
			"critical": 1,
			"high":     2,
			"medium":   3,
			"low":      4,
		},
	}
}

// ServiceNowIncident represents an incident.
type ServiceNowIncident struct {
	SysID            string `json:"sys_id,omitempty"`
	Number           string `json:"number,omitempty"`
	ShortDescription string `json:"short_description"`
	Description      string `json:"description"`
	Severity         int    `json:"severity,omitempty"`
	Priority         int    `json:"priority,omitempty"`
	Urgency          int    `json:"urgency,omitempty"`
	Impact           int    `json:"impact,omitempty"`
	AssignmentGroup  string `json:"assignment_group,omitempty"`
	State            int    `json:"state,omitempty"`
	Category         string `json:"category,omitempty"`
	Subcategory      string `json:"subcategory,omitempty"`
	CorrelationID    string `json:"correlation_id,omitempty"`
}

// IncidentState represents ServiceNow incident states.
const (
	StateNew        = 1
	StateInProgress = 2
	StateOnHold     = 3
	StateResolved   = 6
	StateClosed     = 7
)

// AlertIncidentMapping tracks alert to incident mappings.
type AlertIncidentMapping struct {
	AlertID     string    `json:"alert_id"`
	IncidentID  string    `json:"incident_id"`
	IncidentNum string    `json:"incident_number"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ServiceNowIntegration manages ServiceNow integration.
type ServiceNowIntegration struct {
	mu        sync.RWMutex
	config    ServiceNowConfig
	client    *http.Client
	mappings  map[string]*AlertIncidentMapping
	connected bool
	lastError string
	incidents int64
}

// NewServiceNowIntegration creates a new ServiceNow integration.
func NewServiceNowIntegration() *ServiceNowIntegration {
	return &ServiceNowIntegration{
		config:   DefaultServiceNowConfig(),
		mappings: make(map[string]*AlertIncidentMapping),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Configure updates the ServiceNow configuration.
func (s *ServiceNowIntegration) Configure(config ServiceNowConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if config.SeverityMapping == nil {
		config.SeverityMapping = DefaultServiceNowConfig().SeverityMapping
	}
	if config.TableName == "" {
		config.TableName = "incident"
	}

	s.config = config
	logger.Infof("ServiceNow integration configured: %s", config.InstanceURL)
	return nil
}

// GetConfig returns current configuration.
func (s *ServiceNowIntegration) GetConfig() ServiceNowConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return config with masked password
	cfg := s.config
	if cfg.Password != "" {
		cfg.Password = "********"
	}
	return cfg
}

// GetStatus returns integration status.
func (s *ServiceNowIntegration) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"enabled":           s.config.Enabled,
		"connected":         s.connected,
		"last_error":        s.lastError,
		"incidents_created": s.incidents,
		"active_mappings":   len(s.mappings),
	}
}

// TestConnection verifies ServiceNow connectivity.
func (s *ServiceNowIntegration) TestConnection(ctx context.Context) error {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	if config.InstanceURL == "" || config.Username == "" {
		return fmt.Errorf("ServiceNow not configured")
	}

	// Test by fetching table metadata
	url := fmt.Sprintf("%s/api/now/table/%s?sysparm_limit=1", config.InstanceURL, config.TableName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	s.setAuthHeaders(req, config)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.mu.Lock()
		s.connected = false
		s.lastError = err.Error()
		s.mu.Unlock()
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		s.mu.Lock()
		s.connected = false
		s.lastError = "Authentication failed"
		s.mu.Unlock()
		return fmt.Errorf("authentication failed (401)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.mu.Lock()
		s.connected = false
		s.lastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		s.mu.Unlock()
		return fmt.Errorf("ServiceNow returned %d: %s", resp.StatusCode, string(body))
	}

	s.mu.Lock()
	s.connected = true
	s.lastError = ""
	s.mu.Unlock()

	logger.Info("ServiceNow connection test successful")
	return nil
}

// setAuthHeaders adds Basic auth headers.
func (s *ServiceNowIntegration) setAuthHeaders(req *http.Request, config ServiceNowConfig) {
	auth := base64.StdEncoding.EncodeToString([]byte(config.Username + ":" + config.Password))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
}

// CreateIncident creates a ServiceNow incident from an alert.
func (s *ServiceNowIntegration) CreateIncident(ctx context.Context, alert *domain.Alert) (*AlertIncidentMapping, error) {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	if !config.Enabled || !config.AutoCreate {
		return nil, nil
	}

	// Check if incident already exists
	s.mu.RLock()
	existing, exists := s.mappings[alert.ID]
	s.mu.RUnlock()
	if exists {
		return existing, nil
	}

	// Build incident
	incident := s.buildIncident(alert, config)

	// Create incident
	url := fmt.Sprintf("%s/api/now/table/%s", config.InstanceURL, config.TableName)
	data, _ := json.Marshal(incident)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.setAuthHeaders(req, config)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ServiceNow returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Result ServiceNowIncident `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Create mapping
	mapping := &AlertIncidentMapping{
		AlertID:     alert.ID,
		IncidentID:  result.Result.SysID,
		IncidentNum: result.Result.Number,
		Status:      "open",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.mu.Lock()
	s.mappings[alert.ID] = mapping
	s.incidents++
	s.mu.Unlock()

	logger.Infof("Created ServiceNow incident %s for alert %s", result.Result.Number, alert.ID)
	return mapping, nil
}

// buildIncident creates an incident from an alert.
func (s *ServiceNowIntegration) buildIncident(alert *domain.Alert, config ServiceNowConfig) ServiceNowIncident {
	severity := config.SeverityMapping[alert.Severity.String()]
	if severity == 0 {
		severity = 4 // Default to low
	}

	description := fmt.Sprintf(
		"Alert ID: %s\nRule: %s\nTimestamp: %s\nSeverity: %s\n\nEvent Data:\n%v",
		alert.ID,
		alert.RuleTitle,
		alert.Timestamp.Format(time.RFC3339),
		alert.Severity.String(),
		alert.EventData,
	)

	return ServiceNowIncident{
		ShortDescription: fmt.Sprintf("[%s] %s", alert.Severity.String(), alert.RuleTitle),
		Description:      description,
		Severity:         severity,
		Urgency:          severity,
		Impact:           severity,
		AssignmentGroup:  config.AssignmentGroup,
		State:            StateNew,
		Category:         "Security",
		CorrelationID:    alert.ID,
	}
}

// UpdateIncident updates an existing incident.
func (s *ServiceNowIntegration) UpdateIncident(ctx context.Context, alertID string, state int, notes string) error {
	s.mu.RLock()
	config := s.config
	mapping, exists := s.mappings[alertID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no incident mapping for alert %s", alertID)
	}

	update := map[string]interface{}{
		"state":      state,
		"work_notes": notes,
	}

	url := fmt.Sprintf("%s/api/now/table/%s/%s", config.InstanceURL, config.TableName, mapping.IncidentID)
	data, _ := json.Marshal(update)

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	s.setAuthHeaders(req, config)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update incident: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ServiceNow returned %d: %s", resp.StatusCode, string(body))
	}

	s.mu.Lock()
	mapping.UpdatedAt = time.Now()
	if state == StateResolved || state == StateClosed {
		mapping.Status = "resolved"
	}
	s.mu.Unlock()

	logger.Infof("Updated ServiceNow incident %s", mapping.IncidentNum)
	return nil
}

// GetMapping returns the incident mapping for an alert.
func (s *ServiceNowIntegration) GetMapping(alertID string) *AlertIncidentMapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mappings[alertID]
}

// ListMappings returns all active mappings.
func (s *ServiceNowIntegration) ListMappings() []*AlertIncidentMapping {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AlertIncidentMapping, 0, len(s.mappings))
	for _, m := range s.mappings {
		result = append(result, m)
	}
	return result
}
