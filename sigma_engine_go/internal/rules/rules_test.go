// Package rules provides tests for custom rules.
package rules

import (
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

func TestCustomRuleManager_CreateRule(t *testing.T) {
	manager := NewCustomRuleManager()

	rule := CustomRule{
		Name:        "Test Rule",
		Description: "A test rule",
		Enabled:     true,
		Severity:    "high",
		Conditions: []ConditionGroup{
			{
				Logic: "and",
				Conditions: []Condition{
					{Field: "process_name", Operator: OpEquals, Value: "malware.exe"},
				},
			},
		},
	}

	result, err := manager.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	if result.ID == "" {
		t.Error("Rule ID should be generated")
	}
	if result.Version != 1 {
		t.Errorf("Expected version 1, got %d", result.Version)
	}
}

func TestCustomRuleManager_EvaluateAlert(t *testing.T) {
	manager := NewCustomRuleManager()

	// Create rule
	rule := CustomRule{
		Name:     "Malware Detection",
		Enabled:  true,
		Severity: "critical",
		Conditions: []ConditionGroup{
			{
				Logic: "and",
				Conditions: []Condition{
					{Field: "process_name", Operator: OpEquals, Value: "malware.exe"},
				},
			},
		},
	}
	manager.CreateRule(rule)

	// Matching alert
	matchingAlert := &domain.Alert{
		ID:        "alert-001",
		Severity:  domain.SeverityCritical,
		Timestamp: time.Now(),
		EventData: map[string]interface{}{
			"process_name": "malware.exe",
		},
	}

	matched := manager.EvaluateAlert(matchingAlert)
	if len(matched) != 1 {
		t.Errorf("Expected 1 match, got %d", len(matched))
	}

	// Non-matching alert
	nonMatchingAlert := &domain.Alert{
		ID: "alert-002",
		EventData: map[string]interface{}{
			"process_name": "notepad.exe",
		},
	}

	matched = manager.EvaluateAlert(nonMatchingAlert)
	if len(matched) != 0 {
		t.Errorf("Expected 0 matches, got %d", len(matched))
	}
}

func TestCustomRuleManager_ConditionOperators(t *testing.T) {
	manager := NewCustomRuleManager()

	alert := &domain.Alert{
		ID:       "test",
		Severity: domain.SeverityHigh,
		RuleID:   "rule-123",
		EventData: map[string]interface{}{
			"process_name": "powershell.exe",
			"command_line": "IEX(New-Object Net.WebClient).DownloadString",
		},
	}

	tests := []struct {
		name      string
		condition Condition
		want      bool
	}{
		{
			name:      "equals match",
			condition: Condition{Field: "process_name", Operator: OpEquals, Value: "powershell.exe"},
			want:      true,
		},
		{
			name:      "equals no match",
			condition: Condition{Field: "process_name", Operator: OpEquals, Value: "cmd.exe"},
			want:      false,
		},
		{
			name:      "contains match",
			condition: Condition{Field: "command_line", Operator: OpContains, Value: "DownloadString"},
			want:      true,
		},
		{
			name:      "contains no match",
			condition: Condition{Field: "command_line", Operator: OpContains, Value: "NotFound"},
			want:      false,
		},
		{
			name:      "exists match",
			condition: Condition{Field: "process_name", Operator: OpExists},
			want:      true,
		},
		{
			name:      "not_exists match",
			condition: Condition{Field: "unknown_field", Operator: OpNotExists},
			want:      true,
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

func TestCustomRuleManager_ORLogic(t *testing.T) {
	manager := NewCustomRuleManager()

	rule := CustomRule{
		Name:    "OR Logic Test",
		Enabled: true,
		Conditions: []ConditionGroup{
			{
				Logic: "or",
				Conditions: []Condition{
					{Field: "process_name", Operator: OpEquals, Value: "cmd.exe"},
					{Field: "process_name", Operator: OpEquals, Value: "powershell.exe"},
				},
			},
		},
	}
	manager.CreateRule(rule)

	// Should match (powershell.exe matches second condition)
	alert := &domain.Alert{
		ID:        "test",
		EventData: map[string]interface{}{"process_name": "powershell.exe"},
	}

	matched := manager.EvaluateAlert(alert)
	if len(matched) != 1 {
		t.Error("OR logic should match when any condition is true")
	}
}

func TestCustomRuleManager_TestRule(t *testing.T) {
	manager := NewCustomRuleManager()

	rule := &CustomRule{
		Name: "Test",
		Conditions: []ConditionGroup{
			{
				Logic: "and",
				Conditions: []Condition{
					{Field: "severity", Operator: OpEquals, Value: "critical"},
				},
			},
		},
	}

	// Match
	alertData := map[string]interface{}{"severity": "critical"}
	if !manager.TestRule(rule, alertData) {
		t.Error("Rule should match critical severity")
	}

	// No match
	alertData = map[string]interface{}{"severity": "low"}
	if manager.TestRule(rule, alertData) {
		t.Error("Rule should not match low severity")
	}
}
