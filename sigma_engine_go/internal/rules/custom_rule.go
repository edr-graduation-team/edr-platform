// Package rules provides custom rule builder functionality.
package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// ConditionOperator defines condition comparison operators.
type ConditionOperator string

const (
	OpEquals      ConditionOperator = "equals"
	OpNotEquals   ConditionOperator = "not_equals"
	OpContains    ConditionOperator = "contains"
	OpNotContains ConditionOperator = "not_contains"
	OpMatches     ConditionOperator = "matches"
	OpExists      ConditionOperator = "exists"
	OpNotExists   ConditionOperator = "not_exists"
	OpGreaterThan ConditionOperator = "gt"
	OpLessThan    ConditionOperator = "lt"
	OpIn          ConditionOperator = "in"
	OpNotIn       ConditionOperator = "not_in"
)

// CustomRule defines a user-created detection rule.
type CustomRule struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Enabled           bool             `json:"enabled"`
	Severity          string           `json:"severity"` // critical, high, medium, low
	Conditions        []ConditionGroup `json:"conditions"`
	Actions           []RuleAction     `json:"actions"`
	FalsePositiveRisk float64          `json:"false_positive_risk"`
	Version           int              `json:"version"`
	CreatedBy         string           `json:"created_by"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// ConditionGroup defines a group of conditions with AND/OR logic.
type ConditionGroup struct {
	Logic      string      `json:"logic"` // and, or
	Conditions []Condition `json:"conditions"`
}

// Condition defines a single rule condition.
type Condition struct {
	Field    string            `json:"field"`
	Operator ConditionOperator `json:"operator"`
	Value    interface{}       `json:"value"`
}

// RuleAction defines what happens when rule matches.
type RuleAction struct {
	Type   string                 `json:"type"` // alert, escalate, notify, ticket
	Config map[string]interface{} `json:"config"`
}

// RuleExecution tracks rule execution results.
type RuleExecution struct {
	ID          string    `json:"id"`
	RuleID      string    `json:"rule_id"`
	AlertID     string    `json:"alert_id"`
	Matched     bool      `json:"matched"`
	ExecutionMs int64     `json:"execution_ms"`
	ExecutedAt  time.Time `json:"executed_at"`
}

// RuleMetrics tracks rule effectiveness.
type RuleMetrics struct {
	RuleID          string    `json:"rule_id"`
	TotalExecutions int       `json:"total_executions"`
	Matches         int       `json:"matches"`
	TruePositives   int       `json:"true_positives"`
	FalsePositives  int       `json:"false_positives"`
	Accuracy        float64   `json:"accuracy"`
	LastUpdated     time.Time `json:"last_updated"`
}

// CustomRuleManager manages custom detection rules.
type CustomRuleManager struct {
	mu         sync.RWMutex
	rules      map[string]*CustomRule
	executions []RuleExecution
	metrics    map[string]*RuleMetrics
}

// NewCustomRuleManager creates a new custom rule manager.
func NewCustomRuleManager() *CustomRuleManager {
	return &CustomRuleManager{
		rules:      make(map[string]*CustomRule),
		executions: make([]RuleExecution, 0),
		metrics:    make(map[string]*RuleMetrics),
	}
}

// CreateRule adds a new custom rule.
func (m *CustomRuleManager) CreateRule(rule CustomRule) (*CustomRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	rule.Version = 1
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	m.rules[rule.ID] = &rule
	m.metrics[rule.ID] = &RuleMetrics{RuleID: rule.ID}

	logger.Infof("Created custom rule: %s (%s)", rule.Name, rule.ID)
	return &rule, nil
}

// GetRule retrieves a custom rule by ID.
func (m *CustomRuleManager) GetRule(id string) (*CustomRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, exists := m.rules[id]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", id)
	}
	return rule, nil
}

// ListRules returns all custom rules.
func (m *CustomRuleManager) ListRules() []*CustomRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*CustomRule, 0, len(m.rules))
	for _, r := range m.rules {
		list = append(list, r)
	}
	return list
}

// UpdateRule updates an existing rule.
func (m *CustomRuleManager) UpdateRule(id string, rule CustomRule) (*CustomRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.rules[id]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", id)
	}

	rule.ID = id
	rule.Version = existing.Version + 1
	rule.CreatedAt = existing.CreatedAt
	rule.CreatedBy = existing.CreatedBy
	rule.UpdatedAt = time.Now()
	m.rules[id] = &rule
	return &rule, nil
}

// DeleteRule removes a custom rule.
func (m *CustomRuleManager) DeleteRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[id]; !exists {
		return fmt.Errorf("rule not found: %s", id)
	}
	delete(m.rules, id)
	delete(m.metrics, id)
	logger.Infof("Deleted custom rule: %s", id)
	return nil
}

// EvaluateAlert tests alert against all enabled rules.
func (m *CustomRuleManager) EvaluateAlert(alert *domain.Alert) []*CustomRule {
	m.mu.RLock()
	enabledRules := make([]*CustomRule, 0)
	for _, r := range m.rules {
		if r.Enabled {
			enabledRules = append(enabledRules, r)
		}
	}
	m.mu.RUnlock()

	matched := make([]*CustomRule, 0)
	for _, rule := range enabledRules {
		start := time.Now()
		matches := m.evaluateRule(rule, alert)
		duration := time.Since(start)

		// Record execution
		exec := RuleExecution{
			ID:          uuid.New().String(),
			RuleID:      rule.ID,
			AlertID:     alert.ID,
			Matched:     matches,
			ExecutionMs: duration.Milliseconds(),
			ExecutedAt:  time.Now(),
		}
		m.saveExecution(exec)

		if matches {
			matched = append(matched, rule)
		}
	}

	return matched
}

// evaluateRule checks if an alert matches a rule.
func (m *CustomRuleManager) evaluateRule(rule *CustomRule, alert *domain.Alert) bool {
	if len(rule.Conditions) == 0 {
		return false
	}

	// Evaluate each condition group (groups are OR'd together by default)
	for _, group := range rule.Conditions {
		if m.evaluateConditionGroup(group, alert) {
			return true
		}
	}

	return false
}

// evaluateConditionGroup evaluates a condition group.
func (m *CustomRuleManager) evaluateConditionGroup(group ConditionGroup, alert *domain.Alert) bool {
	if len(group.Conditions) == 0 {
		return false
	}

	results := make([]bool, len(group.Conditions))
	for i, cond := range group.Conditions {
		results[i] = m.evaluateCondition(cond, alert)
	}

	// Apply logic
	if group.Logic == "or" {
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	}

	// Default to AND
	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition.
func (m *CustomRuleManager) evaluateCondition(cond Condition, alert *domain.Alert) bool {
	fieldValue := m.getFieldValue(cond.Field, alert)
	condValue := fmt.Sprintf("%v", cond.Value)

	switch cond.Operator {
	case OpEquals:
		return fieldValue == condValue
	case OpNotEquals:
		return fieldValue != condValue
	case OpContains:
		return strings.Contains(fieldValue, condValue)
	case OpNotContains:
		return !strings.Contains(fieldValue, condValue)
	case OpMatches:
		matched, _ := regexp.MatchString(condValue, fieldValue)
		return matched
	case OpExists:
		return fieldValue != ""
	case OpNotExists:
		return fieldValue == ""
	case OpIn:
		if values, ok := cond.Value.([]interface{}); ok {
			for _, v := range values {
				if fmt.Sprintf("%v", v) == fieldValue {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// getFieldValue extracts field value from alert.
func (m *CustomRuleManager) getFieldValue(field string, alert *domain.Alert) string {
	switch field {
	case "severity":
		return alert.Severity.String()
	case "rule_id":
		return alert.RuleID
	case "rule_name":
		return alert.RuleTitle
	case "category":
		return string(alert.EventCategory)
	default:
		// Check event data
		if alert.EventData != nil {
			if val, ok := alert.EventData[field]; ok {
				return fmt.Sprintf("%v", val)
			}
		}
		return ""
	}
}

// saveExecution stores rule execution record.
func (m *CustomRuleManager) saveExecution(exec RuleExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.executions = append(m.executions, exec)
	if len(m.executions) > 10000 {
		m.executions = m.executions[len(m.executions)-10000:]
	}

	// Update metrics
	if metrics, exists := m.metrics[exec.RuleID]; exists {
		metrics.TotalExecutions++
		if exec.Matched {
			metrics.Matches++
		}
		metrics.LastUpdated = time.Now()
	}
}

// GetRuleMetrics returns metrics for a rule.
func (m *CustomRuleManager) GetRuleMetrics(ruleID string) (*RuleMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[ruleID]
	if !exists {
		return nil, fmt.Errorf("metrics not found: %s", ruleID)
	}
	return metrics, nil
}

// TestRule tests a rule against a sample alert.
func (m *CustomRuleManager) TestRule(rule *CustomRule, alertData map[string]interface{}) bool {
	testAlert := &domain.Alert{
		ID:        "test-alert",
		EventData: alertData,
	}

	if sev, ok := alertData["severity"].(string); ok {
		switch sev {
		case "critical":
			testAlert.Severity = domain.SeverityCritical
		case "high":
			testAlert.Severity = domain.SeverityHigh
		case "medium":
			testAlert.Severity = domain.SeverityMedium
		default:
			testAlert.Severity = domain.SeverityLow
		}
	}

	return m.evaluateRule(rule, testAlert)
}

// ToJSON serializes a rule to JSON.
func (r *CustomRule) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
