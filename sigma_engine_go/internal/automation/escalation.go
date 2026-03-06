// Package automation provides escalation rules for alerts.
package automation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// EscalationLevel defines a single escalation tier.
type EscalationLevel struct {
	Level         int      `json:"level"`
	TimeThreshold int      `json:"time_threshold_mins"` // Minutes after alert creation
	NotifyTo      []string `json:"notify_to"`           // User IDs or group names
	Action        string   `json:"action"`              // notify, create_ticket, page_oncall
}

// EscalationRule defines when to escalate alerts.
type EscalationRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Trigger     EscalationTrigger `json:"trigger"`
	Levels      []EscalationLevel `json:"levels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// EscalationTrigger determines which alerts apply.
type EscalationTrigger struct {
	Severities   []string `json:"severities,omitempty"`
	RuleIDs      []string `json:"rule_ids,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	StatusEquals string   `json:"status_equals,omitempty"` // open, acknowledged
}

// EscalationHistory tracks triggered escalations.
type EscalationHistory struct {
	ID          string    `json:"id"`
	AlertID     string    `json:"alert_id"`
	RuleID      string    `json:"rule_id"`
	RuleName    string    `json:"rule_name"`
	Level       int       `json:"level"`
	NotifiedTo  []string  `json:"notified_to"`
	Action      string    `json:"action"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// AlertState tracks alert status for escalation.
type AlertState struct {
	AlertID     string    `json:"alert_id"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"` // open, acknowledged, resolved
	CreatedAt   time.Time `json:"created_at"`
	EscalatedTo int       `json:"escalated_to"` // Highest escalation level triggered
}

// EscalationManager handles escalation rules.
type EscalationManager struct {
	mu       sync.RWMutex
	rules    map[string]*EscalationRule
	history  []EscalationHistory
	states   map[string]*AlertState
	notifier *NotificationManager
}

// NewEscalationManager creates an escalation manager.
func NewEscalationManager(notifier *NotificationManager) *EscalationManager {
	return &EscalationManager{
		rules:    make(map[string]*EscalationRule),
		history:  make([]EscalationHistory, 0),
		states:   make(map[string]*AlertState),
		notifier: notifier,
	}
}

// CreateRule adds a new escalation rule.
func (m *EscalationManager) CreateRule(rule EscalationRule) (*EscalationRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	m.rules[rule.ID] = &rule
	logger.Infof("Created escalation rule: %s (%s)", rule.Name, rule.ID)
	return &rule, nil
}

// GetRule retrieves an escalation rule.
func (m *EscalationManager) GetRule(id string) (*EscalationRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, exists := m.rules[id]
	if !exists {
		return nil, fmt.Errorf("escalation rule not found: %s", id)
	}
	return rule, nil
}

// ListRules returns all escalation rules.
func (m *EscalationManager) ListRules() []*EscalationRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*EscalationRule, 0, len(m.rules))
	for _, r := range m.rules {
		list = append(list, r)
	}
	return list
}

// UpdateRule updates an escalation rule.
func (m *EscalationManager) UpdateRule(id string, rule EscalationRule) (*EscalationRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.rules[id]
	if !exists {
		return nil, fmt.Errorf("escalation rule not found: %s", id)
	}

	rule.ID = id
	rule.CreatedAt = existing.CreatedAt
	rule.UpdatedAt = time.Now()
	m.rules[id] = &rule
	return &rule, nil
}

// DeleteRule removes an escalation rule.
func (m *EscalationManager) DeleteRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[id]; !exists {
		return fmt.Errorf("escalation rule not found: %s", id)
	}
	delete(m.rules, id)
	logger.Infof("Deleted escalation rule: %s", id)
	return nil
}

// TrackAlert registers an alert for escalation tracking.
func (m *EscalationManager) TrackAlert(alert *domain.Alert) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.states[alert.ID] = &AlertState{
		AlertID:   alert.ID,
		Severity:  alert.Severity.String(),
		Status:    "open",
		CreatedAt: alert.Timestamp,
	}
}

// UpdateAlertStatus updates tracking status.
func (m *EscalationManager) UpdateAlertStatus(alertID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[alertID]; exists {
		state.Status = status
	}
}

// CheckEscalations evaluates and triggers escalations.
func (m *EscalationManager) CheckEscalations(ctx context.Context) {
	m.mu.RLock()
	rules := make([]*EscalationRule, 0)
	for _, r := range m.rules {
		if r.Enabled {
			rules = append(rules, r)
		}
	}

	states := make([]*AlertState, 0)
	for _, s := range m.states {
		if s.Status != "resolved" {
			states = append(states, s)
		}
	}
	m.mu.RUnlock()

	now := time.Now()

	for _, state := range states {
		for _, rule := range rules {
			if !m.matchesTrigger(state, rule.Trigger) {
				continue
			}

			// Check each escalation level
			for _, level := range rule.Levels {
				elapsed := int(now.Sub(state.CreatedAt).Minutes())

				// Skip if already escalated to this level
				if state.EscalatedTo >= level.Level {
					continue
				}

				// Check if threshold exceeded
				if elapsed >= level.TimeThreshold {
					m.triggerEscalation(ctx, state, rule, level)
				}
			}
		}
	}
}

// matchesTrigger checks if alert state matches rule trigger.
func (m *EscalationManager) matchesTrigger(state *AlertState, trigger EscalationTrigger) bool {
	// Check severity
	if len(trigger.Severities) > 0 {
		matched := false
		for _, s := range trigger.Severities {
			if s == state.Severity {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check status
	if trigger.StatusEquals != "" && trigger.StatusEquals != state.Status {
		return false
	}

	return true
}

// triggerEscalation executes an escalation level.
func (m *EscalationManager) triggerEscalation(ctx context.Context, state *AlertState, rule *EscalationRule, level EscalationLevel) {
	logger.Infof("Triggering escalation: %s level %d for alert %s", rule.Name, level.Level, state.AlertID)

	// Execute escalation action
	switch level.Action {
	case "notify":
		// Send notification to escalation recipients
		for _, recipient := range level.NotifyTo {
			logger.Infof("Escalation notification to: %s", recipient)
		}
	case "create_ticket":
		logger.Info("Creating escalation ticket")
	case "page_oncall":
		logger.Info("Paging on-call engineer")
	}

	// Record history
	history := EscalationHistory{
		ID:          uuid.New().String(),
		AlertID:     state.AlertID,
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Level:       level.Level,
		NotifiedTo:  level.NotifyTo,
		Action:      level.Action,
		TriggeredAt: time.Now(),
	}

	m.mu.Lock()
	m.history = append(m.history, history)
	state.EscalatedTo = level.Level

	// Keep last 1000 history entries
	if len(m.history) > 1000 {
		m.history = m.history[len(m.history)-1000:]
	}
	m.mu.Unlock()
}

// GetHistory retrieves escalation history for an alert.
func (m *EscalationManager) GetHistory(alertID string, limit int) []EscalationHistory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]EscalationHistory, 0)
	for i := len(m.history) - 1; i >= 0 && len(result) < limit; i-- {
		if m.history[i].AlertID == alertID {
			result = append(result, m.history[i])
		}
	}
	return result
}

// GetAllHistory retrieves all escalation history.
func (m *EscalationManager) GetAllHistory(limit int) []EscalationHistory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]EscalationHistory, 0)
	for i := len(m.history) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, m.history[i])
	}
	return result
}

// StartBackgroundChecker runs periodic escalation checks.
func (m *EscalationManager) StartBackgroundChecker(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.CheckEscalations(ctx)
			}
		}
	}()
}

// GetStats returns escalation statistics.
func (m *EscalationManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total_rules":       len(m.rules),
		"tracked_alerts":    len(m.states),
		"total_escalations": len(m.history),
	}
}
