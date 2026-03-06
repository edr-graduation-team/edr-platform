// Package integrations provides unit tests for integration components.
package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

func TestWebhookManager_CreateWebhook(t *testing.T) {
	manager := NewWebhookManager()

	config := WebhookConfig{
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Enabled: true,
	}

	webhook, err := manager.CreateWebhook(config)
	if err != nil {
		t.Fatalf("CreateWebhook failed: %v", err)
	}

	if webhook.ID == "" {
		t.Error("Webhook ID should be generated")
	}
	if webhook.Name != "Test Webhook" {
		t.Errorf("Expected name 'Test Webhook', got '%s'", webhook.Name)
	}
}

func TestWebhookManager_ListWebhooks(t *testing.T) {
	manager := NewWebhookManager()

	// Create multiple webhooks
	manager.CreateWebhook(WebhookConfig{Name: "Webhook 1", URL: "https://a.com"})
	manager.CreateWebhook(WebhookConfig{Name: "Webhook 2", URL: "https://b.com"})

	webhooks := manager.ListWebhooks()
	if len(webhooks) != 2 {
		t.Errorf("Expected 2 webhooks, got %d", len(webhooks))
	}
}

func TestWebhookManager_DeleteWebhook(t *testing.T) {
	manager := NewWebhookManager()

	webhook, _ := manager.CreateWebhook(WebhookConfig{Name: "Delete Me", URL: "https://x.com"})

	err := manager.DeleteWebhook(webhook.ID)
	if err != nil {
		t.Fatalf("DeleteWebhook failed: %v", err)
	}

	_, err = manager.GetWebhook(webhook.ID)
	if err == nil {
		t.Error("Webhook should not exist after deletion")
	}
}

func TestWebhookManager_MatchesFilters(t *testing.T) {
	manager := NewWebhookManager()

	alert := &domain.Alert{
		ID:       "test-alert",
		Severity: domain.SeverityCritical,
		RuleID:   "rule-001",
	}

	tests := []struct {
		name    string
		filters WebhookFilters
		want    bool
	}{
		{
			name:    "No filters - matches all",
			filters: WebhookFilters{},
			want:    true,
		},
		{
			name: "Severity matches",
			filters: WebhookFilters{
				Severities: []string{"critical", "high"},
			},
			want: true,
		},
		{
			name: "Severity does not match",
			filters: WebhookFilters{
				Severities: []string{"low"},
			},
			want: false,
		},
		{
			name: "Rule ID matches",
			filters: WebhookFilters{
				RuleIDs: []string{"rule-001"},
			},
			want: true,
		},
		{
			name: "Rule ID does not match",
			filters: WebhookFilters{
				RuleIDs: []string{"rule-999"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.matchesFilters(alert, tt.filters)
			if got != tt.want {
				t.Errorf("matchesFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebhookManager_TestWebhook(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type application/json")
		}

		var payload WebhookPayload
		json.NewDecoder(r.Body).Decode(&payload)

		if payload.Status != "test" {
			t.Error("Expected test payload")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager := NewWebhookManager()
	webhook := &WebhookConfig{
		ID:          "test-id",
		URL:         server.URL,
		RetryPolicy: DefaultRetryPolicy(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.TestWebhook(ctx, webhook)
	if err != nil {
		t.Errorf("TestWebhook failed: %v", err)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("Expected 5 max retries, got %d", policy.MaxRetries)
	}
	if policy.InitialBackoff != time.Second {
		t.Errorf("Expected 1s initial backoff, got %v", policy.InitialBackoff)
	}
	if policy.MaxBackoff != 16*time.Second {
		t.Errorf("Expected 16s max backoff, got %v", policy.MaxBackoff)
	}
}

func TestSplunkIntegration_Configure(t *testing.T) {
	splunk := NewSplunkIntegration()

	config := SplunkConfig{
		Enabled:     true,
		HECEndpoint: "https://splunk.local:8088",
		HECToken:    "test-token",
		Index:       "test-index",
		Source:      "sigma:engine",
		SourceType:  "_json",
	}

	err := splunk.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	got := splunk.GetConfig()
	if got.HECEndpoint != config.HECEndpoint {
		t.Errorf("Expected endpoint %s, got %s", config.HECEndpoint, got.HECEndpoint)
	}
}

func TestSplunkIntegration_GetStatus(t *testing.T) {
	splunk := NewSplunkIntegration()

	status := splunk.GetStatus()
	if status["enabled"] != false {
		t.Error("Expected enabled=false by default")
	}
	if status["connected"] != false {
		t.Error("Expected connected=false by default")
	}
}

func TestServiceNowIntegration_Configure(t *testing.T) {
	sn := NewServiceNowIntegration()

	config := ServiceNowConfig{
		Enabled:         true,
		InstanceURL:     "https://dev123.service-now.com",
		Username:        "admin",
		Password:        "secret",
		AssignmentGroup: "SOC Team",
	}

	err := sn.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	got := sn.GetConfig()
	if got.InstanceURL != config.InstanceURL {
		t.Errorf("Expected URL %s, got %s", config.InstanceURL, got.InstanceURL)
	}
	if got.Password != "********" {
		t.Error("Password should be masked in GetConfig()")
	}
}

func TestServiceNowIntegration_GetStatus(t *testing.T) {
	sn := NewServiceNowIntegration()

	status := sn.GetStatus()
	if status["enabled"] != false {
		t.Error("Expected enabled=false by default")
	}
	if status["incidents_created"] != int64(0) {
		t.Error("Expected 0 incidents by default")
	}
}

func TestServiceNowIntegration_BuildIncident(t *testing.T) {
	sn := NewServiceNowIntegration()
	sn.Configure(ServiceNowConfig{
		Enabled:         true,
		AssignmentGroup: "SOC",
	})

	alert := &domain.Alert{
		ID:        "alert-123",
		RuleTitle: "Suspicious Activity",
		Severity:  domain.SeverityCritical,
		Timestamp: time.Now(),
		EventData: map[string]interface{}{"process": "malware.exe"},
	}

	// Get actual config with password for building
	sn.mu.RLock()
	actualConfig := sn.config
	sn.mu.RUnlock()

	incident := sn.buildIncident(alert, actualConfig)

	if incident.Severity != 1 {
		t.Errorf("Critical should map to severity 1, got %d", incident.Severity)
	}
	if incident.Category != "Security" {
		t.Errorf("Expected category 'Security', got '%s'", incident.Category)
	}
	if incident.State != StateNew {
		t.Errorf("Expected state New (1), got %d", incident.State)
	}
}

func TestDeliveryStatus_Constants(t *testing.T) {
	if DeliveryPending != "pending" {
		t.Error("DeliveryPending should be 'pending'")
	}
	if DeliveryDelivered != "delivered" {
		t.Error("DeliveryDelivered should be 'delivered'")
	}
	if DeliveryFailed != "failed" {
		t.Error("DeliveryFailed should be 'failed'")
	}
}
