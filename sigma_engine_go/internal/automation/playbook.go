// Package automation provides playbook and notification systems.
package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// StepType defines the type of playbook step.
type StepType string

const (
	StepNotifySlack  StepType = "notify_slack"
	StepNotifyTeams  StepType = "notify_teams"
	StepNotifyEmail  StepType = "notify_email"
	StepCreateTicket StepType = "create_ticket"
	StepAcknowledge  StepType = "acknowledge"
	StepResolve      StepType = "resolve"
	StepEscalate     StepType = "escalate"
	StepWait         StepType = "wait"
	StepConditional  StepType = "conditional"
	StepWebhook      StepType = "webhook"
)

// Playbook defines an automation workflow.
type Playbook struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Trigger     PlaybookTrigger `json:"trigger"`
	Steps       []PlaybookStep  `json:"steps"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// PlaybookTrigger defines when a playbook executes.
type PlaybookTrigger struct {
	Severities []string `json:"severities,omitempty"` // critical, high, medium, low
	RuleIDs    []string `json:"rule_ids,omitempty"`
	AgentIDs   []string `json:"agent_ids,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

// PlaybookStep defines a single step in the workflow.
type PlaybookStep struct {
	ID          string                 `json:"id"`
	Type        StepType               `json:"type"`
	Name        string                 `json:"name"`
	Config      map[string]interface{} `json:"config"`
	OnError     string                 `json:"on_error,omitempty"` // continue, stop, retry
	MaxRetries  int                    `json:"max_retries,omitempty"`
	TimeoutSecs int                    `json:"timeout_secs,omitempty"`
	Condition   *StepCondition         `json:"condition,omitempty"`
}

// StepCondition defines conditional logic.
type StepCondition struct {
	Field    string `json:"field"`    // alert.severity, alert.rule_id, etc.
	Operator string `json:"operator"` // equals, contains, matches, gt, lt
	Value    string `json:"value"`
}

// ExecutionStatus represents playbook execution state.
type ExecutionStatus string

const (
	StatusRunning ExecutionStatus = "running"
	StatusSuccess ExecutionStatus = "success"
	StatusFailed  ExecutionStatus = "failed"
	StatusPartial ExecutionStatus = "partial"
)

// PlaybookExecution tracks execution of a playbook.
type PlaybookExecution struct {
	ID           string                `json:"id"`
	PlaybookID   string                `json:"playbook_id"`
	AlertID      string                `json:"alert_id"`
	Status       ExecutionStatus       `json:"status"`
	StepResults  []StepExecutionResult `json:"step_results"`
	ErrorMessage string                `json:"error_message,omitempty"`
	StartedAt    time.Time             `json:"started_at"`
	CompletedAt  *time.Time            `json:"completed_at,omitempty"`
}

// StepExecutionResult tracks a single step result.
type StepExecutionResult struct {
	StepID    string        `json:"step_id"`
	StepName  string        `json:"step_name"`
	StepType  StepType      `json:"step_type"`
	Status    string        `json:"status"` // success, failed, skipped
	Output    interface{}   `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
}

// PlaybookManager manages playbooks.
type PlaybookManager struct {
	mu         sync.RWMutex
	playbooks  map[string]*Playbook
	executions []PlaybookExecution
	notifier   *NotificationManager
}

// NewPlaybookManager creates a new playbook manager.
func NewPlaybookManager(notifier *NotificationManager) *PlaybookManager {
	return &PlaybookManager{
		playbooks:  make(map[string]*Playbook),
		executions: make([]PlaybookExecution, 0),
		notifier:   notifier,
	}
}

// CreatePlaybook adds a new playbook.
func (m *PlaybookManager) CreatePlaybook(playbook Playbook) (*Playbook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if playbook.ID == "" {
		playbook.ID = uuid.New().String()
	}
	playbook.CreatedAt = time.Now()
	playbook.UpdatedAt = time.Now()

	// Generate IDs for steps if not provided
	for i := range playbook.Steps {
		if playbook.Steps[i].ID == "" {
			playbook.Steps[i].ID = uuid.New().String()
		}
	}

	m.playbooks[playbook.ID] = &playbook
	logger.Infof("Created playbook: %s (%s)", playbook.Name, playbook.ID)
	return &playbook, nil
}

// GetPlaybook retrieves a playbook by ID.
func (m *PlaybookManager) GetPlaybook(id string) (*Playbook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	playbook, exists := m.playbooks[id]
	if !exists {
		return nil, fmt.Errorf("playbook not found: %s", id)
	}
	return playbook, nil
}

// ListPlaybooks returns all playbooks.
func (m *PlaybookManager) ListPlaybooks() []*Playbook {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Playbook, 0, len(m.playbooks))
	for _, p := range m.playbooks {
		list = append(list, p)
	}
	return list
}

// UpdatePlaybook updates an existing playbook.
func (m *PlaybookManager) UpdatePlaybook(id string, playbook Playbook) (*Playbook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.playbooks[id]
	if !exists {
		return nil, fmt.Errorf("playbook not found: %s", id)
	}

	playbook.ID = id
	playbook.CreatedAt = existing.CreatedAt
	playbook.UpdatedAt = time.Now()
	m.playbooks[id] = &playbook
	return &playbook, nil
}

// DeletePlaybook removes a playbook.
func (m *PlaybookManager) DeletePlaybook(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.playbooks[id]; !exists {
		return fmt.Errorf("playbook not found: %s", id)
	}
	delete(m.playbooks, id)
	logger.Infof("Deleted playbook: %s", id)
	return nil
}

// ExecuteForAlert runs matching playbooks for an alert.
func (m *PlaybookManager) ExecuteForAlert(ctx context.Context, alert *domain.Alert) {
	m.mu.RLock()
	matching := make([]*Playbook, 0)
	for _, p := range m.playbooks {
		if p.Enabled && m.matchesTrigger(alert, p.Trigger) {
			matching = append(matching, p)
		}
	}
	m.mu.RUnlock()

	for _, playbook := range matching {
		go m.executePlaybook(ctx, playbook, alert)
	}
}

// matchesTrigger checks if an alert matches playbook trigger.
func (m *PlaybookManager) matchesTrigger(alert *domain.Alert, trigger PlaybookTrigger) bool {
	if len(trigger.Severities) > 0 {
		matched := false
		for _, s := range trigger.Severities {
			if s == alert.Severity.String() {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(trigger.RuleIDs) > 0 {
		matched := false
		for _, r := range trigger.RuleIDs {
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

// executePlaybook runs a single playbook.
func (m *PlaybookManager) executePlaybook(ctx context.Context, playbook *Playbook, alert *domain.Alert) {
	execution := PlaybookExecution{
		ID:          uuid.New().String(),
		PlaybookID:  playbook.ID,
		AlertID:     alert.ID,
		Status:      StatusRunning,
		StepResults: make([]StepExecutionResult, 0),
		StartedAt:   time.Now(),
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	logger.Infof("Executing playbook %s for alert %s", playbook.Name, alert.ID)

	// Execute each step
	for _, step := range playbook.Steps {
		result := m.executeStep(execCtx, step, alert)
		execution.StepResults = append(execution.StepResults, result)

		if result.Status == "failed" && step.OnError != "continue" {
			execution.Status = StatusFailed
			execution.ErrorMessage = result.Error
			break
		}
	}

	// Set final status
	if execution.Status != StatusFailed {
		allSuccess := true
		for _, r := range execution.StepResults {
			if r.Status != "success" {
				allSuccess = false
				break
			}
		}
		if allSuccess {
			execution.Status = StatusSuccess
		} else {
			execution.Status = StatusPartial
		}
	}

	now := time.Now()
	execution.CompletedAt = &now

	m.saveExecution(execution)
	logger.Infof("Playbook %s completed with status: %s", playbook.Name, execution.Status)
}

// executeStep runs a single step.
func (m *PlaybookManager) executeStep(ctx context.Context, step PlaybookStep, alert *domain.Alert) StepExecutionResult {
	start := time.Now()
	result := StepExecutionResult{
		StepID:    step.ID,
		StepName:  step.Name,
		StepType:  step.Type,
		StartedAt: start,
	}

	// Check condition if present
	if step.Condition != nil && !m.evaluateCondition(step.Condition, alert) {
		result.Status = "skipped"
		result.Duration = time.Since(start)
		return result
	}

	// Execute based on type
	var err error
	switch step.Type {
	case StepNotifySlack:
		err = m.notifier.SendSlack(ctx, alert, step.Config)
	case StepNotifyTeams:
		err = m.notifier.SendTeams(ctx, alert, step.Config)
	case StepNotifyEmail:
		err = m.notifier.SendEmail(ctx, alert, step.Config)
	case StepWait:
		duration, _ := time.ParseDuration(fmt.Sprintf("%v", step.Config["duration"]))
		if duration == 0 {
			duration = 30 * time.Second
		}
		select {
		case <-time.After(duration):
		case <-ctx.Done():
			err = ctx.Err()
		}
	case StepAcknowledge:
		result.Output = map[string]string{"action": "acknowledge", "alert_id": alert.ID}
	case StepResolve:
		result.Output = map[string]string{"action": "resolve", "alert_id": alert.ID}
	case StepWebhook:
		err = m.executeWebhookStep(ctx, step, alert)
	case StepCreateTicket:
		result.Output = map[string]string{"note": "create_ticket has no external ticketing integration in this build"}
	case StepEscalate:
		result.Output = map[string]string{"note": "escalate step is informational; use escalation rules for timed notifications"}
	case StepConditional:
		result.Output = map[string]string{"note": "conditional is evaluated per-step via condition field only"}
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	result.Duration = time.Since(start)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	} else {
		result.Status = "success"
	}

	return result
}

// executeWebhookStep POSTs JSON to config["url"] with optional config["body"] and config["headers"].
func (m *PlaybookManager) executeWebhookStep(ctx context.Context, step PlaybookStep, alert *domain.Alert) error {
	rawURL, _ := step.Config["url"].(string)
	if rawURL == "" {
		return fmt.Errorf("webhook step missing url")
	}
	urlStr := SubstituteVariables(rawURL, alert)

	body := map[string]interface{}{
		"alert_id":   alert.ID,
		"rule_id":    alert.RuleID,
		"rule_title": alert.RuleTitle,
		"severity":   alert.Severity.String(),
		"timestamp":  alert.Timestamp.UTC().Format(time.RFC3339),
	}
	if custom, ok := step.Config["body"].(map[string]interface{}); ok {
		for k, v := range custom {
			body[k] = v
		}
	}

	timeout := 30 * time.Second
	if step.TimeoutSecs > 0 {
		timeout = time.Duration(step.TimeoutSecs) * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, urlStr, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if hdrs, ok := step.Config["headers"].(map[string]interface{}); ok {
		for k, v := range hdrs {
			req.Header.Set(k, SubstituteVariables(fmt.Sprint(v), alert))
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}

// evaluateCondition checks if condition is met.
func (m *PlaybookManager) evaluateCondition(cond *StepCondition, alert *domain.Alert) bool {
	var fieldValue string
	switch cond.Field {
	case "alert.severity":
		fieldValue = alert.Severity.String()
	case "alert.rule_id":
		fieldValue = alert.RuleID
	case "alert.rule_name":
		fieldValue = alert.RuleTitle
	default:
		return false
	}

	switch cond.Operator {
	case "equals":
		return fieldValue == cond.Value
	case "contains":
		return strings.Contains(fieldValue, cond.Value)
	case "matches":
		matched, _ := regexp.MatchString(cond.Value, fieldValue)
		return matched
	default:
		return fieldValue == cond.Value
	}
}

// saveExecution stores execution record.
func (m *PlaybookManager) saveExecution(execution PlaybookExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions = append(m.executions, execution)

	// Keep last 1000 executions
	if len(m.executions) > 1000 {
		m.executions = m.executions[len(m.executions)-1000:]
	}
}

// GetExecutions returns executions for a playbook.
func (m *PlaybookManager) GetExecutions(playbookID string, limit int) []PlaybookExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PlaybookExecution, 0)
	for i := len(m.executions) - 1; i >= 0 && len(result) < limit; i-- {
		if m.executions[i].PlaybookID == playbookID {
			result = append(result, m.executions[i])
		}
	}
	return result
}

// SubstituteVariables replaces template variables in a string.
func SubstituteVariables(template string, alert *domain.Alert) string {
	replacements := map[string]string{
		"{{alert.id}}":        alert.ID,
		"{{alert.severity}}":  alert.Severity.String(),
		"{{alert.rule_id}}":   alert.RuleID,
		"{{alert.rule_name}}": alert.RuleTitle,
		"{{alert.timestamp}}": alert.Timestamp.Format(time.RFC3339),
	}

	result := template
	for k, v := range replacements {
		result = strings.ReplaceAll(result, k, v)
	}
	return result
}

// MarshalJSON for StepType.
func (s StepType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// UnmarshalJSON for StepType.
func (s *StepType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = StepType(str)
	return nil
}
