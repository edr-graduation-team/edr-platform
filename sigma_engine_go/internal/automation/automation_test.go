// Package automation provides unit tests for automation components.
package automation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

func TestPlaybookManager_CreatePlaybook(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewPlaybookManager(notifier)

	playbook := Playbook{
		Name:        "Test Playbook",
		Description: "A test playbook",
		Enabled:     true,
		Trigger: PlaybookTrigger{
			Severities: []string{"critical", "high"},
		},
		Steps: []PlaybookStep{
			{Name: "Notify Slack", Type: StepNotifySlack},
		},
	}

	result, err := manager.CreatePlaybook(playbook)
	if err != nil {
		t.Fatalf("CreatePlaybook failed: %v", err)
	}

	if result.ID == "" {
		t.Error("Playbook ID should be generated")
	}
	if result.Name != "Test Playbook" {
		t.Errorf("Expected name 'Test Playbook', got '%s'", result.Name)
	}
	if len(result.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(result.Steps))
	}
}

func TestPlaybookManager_ListPlaybooks(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewPlaybookManager(notifier)

	manager.CreatePlaybook(Playbook{Name: "Playbook 1"})
	manager.CreatePlaybook(Playbook{Name: "Playbook 2"})

	playbooks := manager.ListPlaybooks()
	if len(playbooks) != 2 {
		t.Errorf("Expected 2 playbooks, got %d", len(playbooks))
	}
}

func TestPlaybookManager_DeletePlaybook(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewPlaybookManager(notifier)

	playbook, _ := manager.CreatePlaybook(Playbook{Name: "Delete Me"})

	err := manager.DeletePlaybook(playbook.ID)
	if err != nil {
		t.Fatalf("DeletePlaybook failed: %v", err)
	}

	_, err = manager.GetPlaybook(playbook.ID)
	if err == nil {
		t.Error("Playbook should not exist after deletion")
	}
}

func TestPlaybookManager_MatchesTrigger(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewPlaybookManager(notifier)

	alert := &domain.Alert{
		ID:       "test-alert",
		Severity: domain.SeverityCritical,
		RuleID:   "rule-001",
	}

	tests := []struct {
		name    string
		trigger PlaybookTrigger
		want    bool
	}{
		{
			name:    "Empty trigger matches all",
			trigger: PlaybookTrigger{},
			want:    true,
		},
		{
			name: "Severity matches",
			trigger: PlaybookTrigger{
				Severities: []string{"critical", "high"},
			},
			want: true,
		},
		{
			name: "Severity does not match",
			trigger: PlaybookTrigger{
				Severities: []string{"low"},
			},
			want: false,
		},
		{
			name: "Rule ID matches",
			trigger: PlaybookTrigger{
				RuleIDs: []string{"rule-001"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.matchesTrigger(alert, tt.trigger)
			if got != tt.want {
				t.Errorf("matchesTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubstituteVariables(t *testing.T) {
	alert := &domain.Alert{
		ID:        "alert-123",
		Severity:  domain.SeverityHigh,
		RuleID:    "rule-456",
		RuleTitle: "Suspicious Process",
		Timestamp: time.Date(2026, 3, 3, 15, 30, 0, 0, time.UTC),
	}

	tests := []struct {
		template string
		expected string
	}{
		{
			template: "Alert: {{alert.rule_name}}",
			expected: "Alert: Suspicious Process",
		},
		{
			template: "Severity: {{alert.severity}}",
			expected: "Severity: high",
		},
		{
			template: "ID: {{alert.id}}, Rule: {{alert.rule_id}}",
			expected: "ID: alert-123, Rule: rule-456",
		},
	}

	for _, tt := range tests {
		result := SubstituteVariables(tt.template, alert)
		if result != tt.expected {
			t.Errorf("SubstituteVariables(%s) = %s, want %s", tt.template, result, tt.expected)
		}
	}
}

func TestStepCondition_Evaluate(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewPlaybookManager(notifier)

	alert := &domain.Alert{
		ID:        "test-alert",
		Severity:  domain.SeverityCritical,
		RuleID:    "rule-001",
		RuleTitle: "Malware Detected",
	}

	tests := []struct {
		name      string
		condition *StepCondition
		want      bool
	}{
		{
			name: "equals matches",
			condition: &StepCondition{
				Field:    "alert.severity",
				Operator: "equals",
				Value:    "critical",
			},
			want: true,
		},
		{
			name: "equals does not match",
			condition: &StepCondition{
				Field:    "alert.severity",
				Operator: "equals",
				Value:    "low",
			},
			want: false,
		},
		{
			name: "contains matches",
			condition: &StepCondition{
				Field:    "alert.rule_name",
				Operator: "contains",
				Value:    "Malware",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.evaluateCondition(tt.condition, alert)
			if got != tt.want {
				t.Errorf("evaluateCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNotificationManager_Configure(t *testing.T) {
	notifier := NewNotificationManager()

	config := NotificationConfig{
		SlackWebhookURL: "https://hooks.slack.com/test",
		TeamsWebhookURL: "https://outlook.office.com/webhook/test",
		SMTPServer:      "smtp.example.com",
		SMTPPort:        587,
		EmailFrom:       "sigma@example.com",
	}

	notifier.Configure(config)

	got := notifier.GetConfig()
	if got.SlackWebhookURL != config.SlackWebhookURL {
		t.Errorf("Expected Slack URL %s, got %s", config.SlackWebhookURL, got.SlackWebhookURL)
	}
	if got.SMTPPassword != "********" {
		t.Error("Password should be masked in GetConfig()")
	}
}

func TestNotificationManager_SendSlack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type application/json")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotificationManager()
	notifier.Configure(NotificationConfig{
		SlackWebhookURL: server.URL,
		DefaultChannel:  "#test",
	})

	alert := &domain.Alert{
		ID:        "alert-123",
		Severity:  domain.SeverityCritical,
		RuleTitle: "Test Alert",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := notifier.SendSlack(ctx, alert, nil)
	if err != nil {
		t.Errorf("SendSlack failed: %v", err)
	}

	logs := notifier.GetLogs(10, "slack")
	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].Status != "sent" {
		t.Errorf("Expected status 'sent', got '%s'", logs[0].Status)
	}
}

func TestEscalationManager_CreateRule(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewEscalationManager(notifier)

	rule := EscalationRule{
		Name:        "Critical Alert Escalation",
		Description: "Escalate critical alerts",
		Enabled:     true,
		Trigger: EscalationTrigger{
			Severities: []string{"critical"},
		},
		Levels: []EscalationLevel{
			{Level: 1, TimeThreshold: 5, NotifyTo: []string{"manager@example.com"}, Action: "notify"},
			{Level: 2, TimeThreshold: 15, NotifyTo: []string{"director@example.com"}, Action: "notify"},
		},
	}

	result, err := manager.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	if result.ID == "" {
		t.Error("Rule ID should be generated")
	}
	if len(result.Levels) != 2 {
		t.Errorf("Expected 2 levels, got %d", len(result.Levels))
	}
}

func TestEscalationManager_ListRules(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewEscalationManager(notifier)

	manager.CreateRule(EscalationRule{Name: "Rule 1"})
	manager.CreateRule(EscalationRule{Name: "Rule 2"})

	rules := manager.ListRules()
	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}
}

func TestEscalationManager_TrackAlert(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewEscalationManager(notifier)

	alert := &domain.Alert{
		ID:        "alert-123",
		Severity:  domain.SeverityCritical,
		Timestamp: time.Now(),
	}

	manager.TrackAlert(alert)
	manager.UpdateAlertStatus("alert-123", "acknowledged")

	stats := manager.GetStats()
	if stats["tracked_alerts"] != 1 {
		t.Errorf("Expected 1 tracked alert, got %v", stats["tracked_alerts"])
	}
}

func TestEscalationManager_MatchesTrigger(t *testing.T) {
	notifier := NewNotificationManager()
	manager := NewEscalationManager(notifier)

	state := &AlertState{
		AlertID:  "alert-123",
		Severity: "critical",
		Status:   "open",
	}

	tests := []struct {
		name    string
		trigger EscalationTrigger
		want    bool
	}{
		{
			name:    "Empty trigger matches all",
			trigger: EscalationTrigger{},
			want:    true,
		},
		{
			name: "Severity matches",
			trigger: EscalationTrigger{
				Severities: []string{"critical"},
			},
			want: true,
		},
		{
			name: "Status matches",
			trigger: EscalationTrigger{
				StatusEquals: "open",
			},
			want: true,
		},
		{
			name: "Status does not match",
			trigger: EscalationTrigger{
				StatusEquals: "acknowledged",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.matchesTrigger(state, tt.trigger)
			if got != tt.want {
				t.Errorf("matchesTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepType_JSON(t *testing.T) {
	step := StepNotifySlack

	// Test marshaling
	data, err := step.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if string(data) != `"notify_slack"` {
		t.Errorf("Expected '\"notify_slack\"', got %s", string(data))
	}

	// Test unmarshaling
	var parsed StepType
	err = parsed.UnmarshalJSON([]byte(`"notify_teams"`))
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if parsed != StepNotifyTeams {
		t.Errorf("Expected StepNotifyTeams, got %s", parsed)
	}
}
