package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// AutomationService provides intelligent automation capabilities for EDR alerts
type AutomationService struct {
	logger              *logrus.Logger
	alertRepo           repository.AlertRepository
	playbookRepo        repository.ResponsePlaybookRepository
	automationRepo      repository.AutomationRuleRepository
	executionRepo       repository.PlaybookExecutionRepository
	commandService      *CommandService
	notificationService *NotificationService
	metricsService      *MetricsService
	mlOptimizer         *MLOptimizer
	cooldownMap         map[uuid.UUID]time.Time
	cooldownMutex       sync.RWMutex
	c2Address           string // C2 gRPC address injected into isolate commands
}

// NewAutomationService creates a new automation service instance
func NewAutomationService(
	logger *logrus.Logger,
	alertRepo repository.AlertRepository,
	playbookRepo repository.ResponsePlaybookRepository,
	automationRepo repository.AutomationRuleRepository,
	executionRepo repository.PlaybookExecutionRepository,
	commandService *CommandService,
	notificationService *NotificationService,
	metricsService *MetricsService,
	mlOptimizer *MLOptimizer,
) *AutomationService {
	return &AutomationService{
		logger:              logger,
		alertRepo:           alertRepo,
		playbookRepo:        playbookRepo,
		automationRepo:      automationRepo,
		executionRepo:       executionRepo,
		commandService:      commandService,
		notificationService: notificationService,
		metricsService:      metricsService,
		mlOptimizer:         mlOptimizer,
		cooldownMap:         make(map[uuid.UUID]time.Time),
	}
}

// SetC2Address sets the C2 gRPC address that is automatically injected as
// "server_address" into isolate_network playbook commands so the agent builds
// correct firewall ALLOW rules without falling back to its config file.
func (s *AutomationService) SetC2Address(addr string) {
	s.c2Address = addr
}

// ProcessAlert handles the main alert processing workflow
func (s *AutomationService) ProcessAlert(ctx context.Context, alert *models.Alert) error {
	startTime := time.Now()

	s.logger.WithFields(logrus.Fields{
		"alert_id":  alert.ID,
		"severity":  alert.Severity,
		"rule_name": alert.RuleName,
		"agent_id":  alert.AgentID,
	}).Info("Starting intelligent alert processing")

	// 1. Find matching automation rules
	rules, err := s.automationRepo.GetMatchingRules(ctx, alert)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get matching automation rules")
		return err
	}

	s.logger.Infof("Found %d matching automation rules", len(rules))

	// 2. Optimize rule priorities using ML
	optimizedRules := s.mlOptimizer.OptimizeRulePriority(rules, alert)

	// 3. Execute only the SINGLE highest-priority matching auto-rule.
	//    Executing all 10 rules simultaneously would flood the agent with
	//    redundant commands. The optimizer already sorted by priority.
	var executedRule *models.AutomationRule
	for _, rule := range optimizedRules {
		if rule.AutoExecute && s.shouldExecute(rule, alert) {
			executedRule = rule
			break // take only the top rule
		}
	}
	if executedRule != nil {
		s.logger.Infof("[automation] Auto-executing rule: %s (playbook: %s)", executedRule.Name, executedRule.PlaybookID)
		go s.executePlaybookAsync(ctx, executedRule.PlaybookID, alert.ID, alert.AgentID, executedRule.ID)
		s.setCooldown(executedRule.ID)
	}

	// 4. Generate intelligent playbook suggestions
	suggestions := s.generateIntelligentSuggestions(alert)
	if err := s.storeSuggestions(ctx, alert.ID, suggestions); err != nil {
		s.logger.WithError(err).Error("Failed to store suggestions")
	}

	// 5. Update metrics
	processingTime := time.Since(startTime)
	s.metricsService.RecordAlertProcessing(alert, len(rules), len(suggestions), processingTime)

	// 6. Send intelligent notifications
	s.notificationService.SendAlertProcessed(alert, len(rules), len(suggestions))

	return nil
}

// shouldExecute checks if a rule should be executed based on conditions
func (s *AutomationService) shouldExecute(rule *models.AutomationRule, alert *models.Alert) bool {
	// Check cooldown period
	if s.isInCooldown(rule.ID, rule.CooldownMinutes) {
		s.logger.Infof("Rule %s is in cooldown period", rule.Name)
		return false
	}

	// Check advanced conditions
	var conditions map[string]interface{}
	if len(rule.TriggerConditions) > 0 {
		if err := json.Unmarshal(rule.TriggerConditions, &conditions); err != nil {
			s.logger.WithError(err).Error("Failed to unmarshal trigger conditions")
			return false
		}
	}
	return s.evaluateAdvancedConditions(conditions, alert)
}

// evaluateAdvancedConditions evaluates complex trigger conditions
func (s *AutomationService) evaluateAdvancedConditions(conditions map[string]interface{}, alert *models.Alert) bool {
	// Check severity
	if severities, ok := conditions["severity"].([]interface{}); ok {
		if !contains(severities, alert.Severity) {
			return false
		}
	}

	// Check rule patterns
	if patterns, ok := conditions["rule_patterns"].([]interface{}); ok {
		ruleName := strings.ToLower(alert.RuleName)
		matched := false
		for _, pattern := range patterns {
			if strings.Contains(ruleName, strings.ToLower(pattern.(string))) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check confidence threshold
	if threshold, ok := conditions["confidence_threshold"].(float64); ok && threshold > 0 {
		if alert.Confidence < threshold {
			return false
		}
	}

	// Check logic operator
	if operator, ok := conditions["logic_operator"].(string); ok && operator == "OR" {
		// For OR logic, any condition match is sufficient
		return true
	}

	return true
}

// generateIntelligentSuggestions creates smart playbook suggestions
func (s *AutomationService) generateIntelligentSuggestions(alert *models.Alert) []models.PlaybookSuggestion {
	var suggestions []models.PlaybookSuggestion

	ruleName := strings.ToLower(alert.RuleName)

	// Severity-based suggestions
	if alert.Severity == "critical" {
		suggestions = append(suggestions, models.PlaybookSuggestion{
			PlaybookID: uuid.Nil, // placeholder — real ID resolved at execution time
			Confidence: 0.95,
			Reason:     "Critical severity alert requires immediate containment",
			MITREMatch: []string{"T1055", "T1059"},
		})
	}

	// Rule name-based suggestions
	if strings.Contains(ruleName, "malware") || strings.Contains(ruleName, "trojan") {
		suggestions = append(suggestions, models.PlaybookSuggestion{
			PlaybookID: uuid.Nil,
			Confidence: 0.85,
			Reason:     "Malware-related behavior detected",
			MITREMatch: []string{"T1055", "T1543"},
		})
	}

	// Risk score-based suggestions
	if alert.RiskScore > 8 {
		suggestions = append(suggestions, models.PlaybookSuggestion{
			PlaybookID: uuid.Nil,
			Confidence: 0.90,
			Reason:     "High-risk agent detected",
			MITREMatch: []string{"T1082", "T1018"},
		})
	}

	// Ransomware-specific suggestions
	if strings.Contains(ruleName, "ransomware") || strings.Contains(ruleName, "encryption") {
		suggestions = append(suggestions, models.PlaybookSuggestion{
			PlaybookID: uuid.Nil,
			Confidence: 0.98,
			Reason:     "Ransomware activity detected",
			MITREMatch: []string{"T1486", "T1059"},
		})
	}

	return suggestions
}

// executePlaybookAsync executes a playbook asynchronously with monitoring
func (s *AutomationService) executePlaybookAsync(ctx context.Context, playbookID, alertID, agentID, ruleID uuid.UUID) {
	startTime := time.Now()

	execution := &models.PlaybookExecution{
		AlertID:    alertID,
		PlaybookID: playbookID,
		RuleID:     &ruleID,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	trackingEnabled := true
	if err := s.executionRepo.Create(ctx, execution); err != nil {
		// alert_id may be synthetic (auto-execute path) and not in the alerts table.
		// Log the error but DO NOT return — the agent command must still execute.
		s.logger.WithError(err).Warn("Could not create execution record (alert_id not in alerts table); proceeding with command execution anyway")
		execution.ID = uuid.Nil
		trackingEnabled = false
	}

	// Execute commands — this is the critical path that actually runs on the agent
	success := s.executePlaybookCommands(ctx, playbookID, alertID, agentID, execution.ID)

	// Update tracking record only if it was created successfully
	if trackingEnabled {
		executionTime := time.Since(startTime)
		execution.Status = "completed"
		execution.CompletedAt = &time.Time{}
		execution.ExecutionTimeMs = int(executionTime.Milliseconds())
		if !success {
			execution.Status = "failed"
			execution.ErrorMessage = "One or more commands failed"
		}
		s.executionRepo.Update(ctx, execution)
		s.notificationService.SendPlaybookCompleted(execution, success)
	}

	// Always update rule metrics regardless of tracking
	s.updateRuleMetrics(ruleID, success, time.Since(startTime))

	// Mark the sigma_alert as auto-contained so the Dashboard Alerts page
	// can display the "AUTO CONTAINED" badge.
	// Fire-and-forget: best-effort, never blocks the response pipeline.
	if success && alertID != uuid.Nil {
		go s.markAlertAutoResponse(alertID)
	}

	s.logger.Infof("[playbook] Execution finished — playbook: %s success: %v", playbookID, success)
}

// markAlertAutoResponse appends "auto_response:triggered" to the sigma_alert notes
// by calling the sigma_engine's PATCH /api/v1/alerts/:id/status endpoint.
// Using the HTTP API is more reliable than a direct cross-service DB write.
func (s *AutomationService) markAlertAutoResponse(alertID uuid.UUID) {
	if alertID == uuid.Nil {
		return
	}

	// Try the DB-direct path first (fast path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.alertRepo.AppendAutoResponseNote(ctx, alertID); err == nil {
		s.logger.Infof("[automation] Marked alert %s as auto_response ✅ (DB path)", alertID)
		return
	}

	// Fallback: call sigma_engine HTTP API to update resolution_notes
	sigmaURL := os.Getenv("SIGMA_ENGINE_URL")
	if sigmaURL == "" {
		sigmaURL = "http://sigma-engine:8080" // docker-compose service name + port
	}

	endpoint := fmt.Sprintf("%s/api/v1/sigma/alerts/%s/status", sigmaURL, alertID.String())
	// sigma_engine's UpdateStatusRequest uses "notes" (not "resolution_notes")
	payload := `{"status":"open","notes":"auto_response:triggered"}`

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, strings.NewReader(payload))
	if err != nil {
		s.logger.WithError(err).Warn("[automation] Could not build sigma PATCH request (non-fatal)")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.WithError(err).WithField("alert_id", alertID).
			Warn("[automation] Could not mark sigma_alert as auto_response via HTTP (non-fatal)")
		return
	}
	resp.Body.Close()
	s.logger.Infof("[automation] Marked alert %s as auto_response ✅ (HTTP path, status: %d)", alertID, resp.StatusCode)
}


// executePlaybookCommands executes all commands in a playbook
func (s *AutomationService) executePlaybookCommands(ctx context.Context, playbookID, alertID, agentID, executionID uuid.UUID) bool {
	playbook, err := s.playbookRepo.GetByID(ctx, playbookID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get playbook")
		return false
	}

	// If agentID is not provided directly, try fetching from alerts table
	if agentID == uuid.Nil {
		if alert, err := s.alertRepo.GetByID(ctx, alertID); err == nil {
			agentID = alert.AgentID
		} else {
			s.logger.WithError(err).Error("Failed to get alert for agentID")
			return false
		}
	}

	success := true
	var commands []models.PlaybookCommand
	if err := json.Unmarshal(playbook.Commands, &commands); err != nil {
		s.logger.WithError(err).Error("Failed to unmarshal playbook commands")
		return false
	}

	for i, cmd := range commands {
		s.logger.Infof("Executing command %d/%d: %s", i+1, len(commands), cmd.Type)

		// Enrich missing parameters with sensible defaults so the agent
		// doesn't reject commands that require specific fields.
		enrichPlaybookCommandParams(&cmd, alertID, s.c2Address)

		result := s.commandService.ExecutePlaybookCommand(ctx, executionID, cmd, agentID)

		if result.Status == "failed" && cmd.OnFailure == "stop" {
			s.logger.Errorf("Command %s failed, stopping playbook execution", cmd.Type)
			success = false
			break
		}
		if result.Status == "failed" {
			s.logger.Warnf("Command %s failed but continuing", cmd.Type)
		}
	}

	return success
}

// enrichPlaybookCommandParams injects required parameters that may be missing
// from the playbook definition in the DB. This ensures commands that need
// specific fields (pid, log_types, file_path) work without manual input.
func enrichPlaybookCommandParams(cmd *models.PlaybookCommand, alertID uuid.UUID, c2Address string) {
	if cmd.Parameters == nil {
		cmd.Parameters = make(map[string]interface{})
	}

	switch cmd.Type {
	case "terminate_process", "kill_process":
		// Agent supports process_name fallback when pid is missing.
		// Use mshta.exe as default since this playbook targets MSHTA malware.
		if _, hasPID := cmd.Parameters["pid"]; !hasPID {
			if _, hasName := cmd.Parameters["process_name"]; !hasName {
				cmd.Parameters["process_name"] = "mshta.exe"
			}
		}

	case "collect_forensics", "collect_logs":
		// Require at least one of: log_types, types, file_path, path.
		_, hasLogTypes := cmd.Parameters["log_types"]
		_, hasTypes := cmd.Parameters["types"]
		_, hasFilePath := cmd.Parameters["file_path"]
		_, hasPath := cmd.Parameters["path"]
		if !hasLogTypes && !hasTypes && !hasFilePath && !hasPath {
			cmd.Parameters["log_types"] = "Security,System,Sysmon"
		}

	case "quarantine_file":
		// Default to mshta.exe path when file_path is missing.
		if _, ok := cmd.Parameters["file_path"]; !ok {
			if _, ok := cmd.Parameters["path"]; !ok {
				cmd.Parameters["file_path"] = `C:\Windows\System32\mshta.exe`
			}
		}

	case "run_cmd":
		// Agent reads params["cmd"] (not "command").
		// Provide a safe forensic default when neither key is present.
		_, hasCmd := cmd.Parameters["cmd"]
		_, hasCommand := cmd.Parameters["command"]
		if !hasCmd && !hasCommand {
			// Default: capture running process list for post-isolation forensics.
			// This is whitelisted in playbookAllowedCommands on the agent side.
			cmd.Parameters["cmd"] = `powershell -Command "Get-Process | Select-Object Id,ProcessName,CPU,WorkingSet | Sort-Object CPU -Descending | ConvertTo-Json"`
		} else if hasCommand && !hasCmd {
			// Normalise: agent always reads "cmd"
			cmd.Parameters["cmd"] = cmd.Parameters["command"]
			delete(cmd.Parameters, "command")
		}

	case "isolate", "isolate_network", "unisolate", "unisolate_network", "restore_network":
		// Inject C2 server_address so the agent builds correct ALLOW firewall rules.
		// Without this, the agent falls back to its config file (e.g. edr.local)
		// which may not resolve in the deployment environment.
		if _, hasAddr := cmd.Parameters["server_address"]; !hasAddr {
			if c2Address != "" {
				cmd.Parameters["server_address"] = c2Address
			}
		}
	}

	// Always inject from_playbook marker for security policy bypass
	cmd.Parameters["from_playbook"] = "true"
}

// isInCooldown checks if a rule is in cooldown period
func (s *AutomationService) isInCooldown(ruleID uuid.UUID, cooldownMinutes int) bool {
	if cooldownMinutes <= 0 {
		return false
	}

	s.cooldownMutex.RLock()
	defer s.cooldownMutex.RUnlock()

	lastExecution, exists := s.cooldownMap[ruleID]
	if !exists {
		return false
	}

	return time.Since(lastExecution) < time.Duration(cooldownMinutes)*time.Minute
}

// setCooldown sets a cooldown for a rule
func (s *AutomationService) setCooldown(ruleID uuid.UUID) {
	s.cooldownMutex.Lock()
	defer s.cooldownMutex.Unlock()

	s.cooldownMap[ruleID] = time.Now()
}

// updateRuleMetrics updates rule performance metrics
func (s *AutomationService) updateRuleMetrics(ruleID uuid.UUID, success bool, executionTime time.Duration) {
	// Update rule success rate
	rule, err := s.automationRepo.GetByID(context.Background(), ruleID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get rule for metrics update")
		return
	}

	// Exponential weighted average. Initialise to 1.0 on first execution so a
	// brand-new rule doesn't collapse to 0 after a single failure.
	if rule.LastExecution == nil {
		rule.SuccessRate = 1.0
	}
	if success {
		rule.SuccessRate = rule.SuccessRate*0.8 + 0.2
	} else {
		rule.SuccessRate = rule.SuccessRate * 0.8
	}

	rule.LastExecution = &time.Time{}
	*rule.LastExecution = time.Now()

	s.automationRepo.Update(context.Background(), rule)

	// Set cooldown
	s.setCooldown(ruleID)

	// Record metrics
	s.metricsService.RecordRuleExecution(ruleID, success, executionTime)
}

// storeSuggestions stores playbook suggestions for an alert
func (s *AutomationService) storeSuggestions(ctx context.Context, alertID uuid.UUID, suggestions []models.PlaybookSuggestion) error {
	for _, suggestion := range suggestions {
		suggestion.AlertID = alertID
		suggestion.CreatedAt = time.Now()
		if err := s.executionRepo.CreateSuggestion(ctx, &suggestion); err != nil {
			s.logger.WithError(err).Error("Failed to store suggestion")
			return err
		}
	}
	return nil
}

// GetSuggestions retrieves playbook suggestions for an alert
func (s *AutomationService) GetSuggestions(ctx context.Context, alertID uuid.UUID) ([]models.PlaybookSuggestion, error) {
	return s.executionRepo.GetSuggestions(ctx, alertID)
}

// ExecutePlaybookForAlert manually executes a playbook for a specific alert
func (s *AutomationService) ExecutePlaybookForAlert(ctx context.Context, playbookID, alertID, userID uuid.UUID) (*models.PlaybookExecution, error) {
	execution := &models.PlaybookExecution{
		AlertID:    alertID,
		PlaybookID: playbookID,
		Status:     "running",
		StartedAt:  time.Now(),
		CreatedBy:  &userID,
	}

	if err := s.executionRepo.Create(ctx, execution); err != nil {
		return nil, err
	}

	// Fetch agentID from alert (manual execute path — alertID is always a real DB record)
	agentID := uuid.Nil
	if alert, err := s.alertRepo.GetByID(ctx, alertID); err == nil {
		agentID = alert.AgentID
	}

	// Execute asynchronously
	go s.executePlaybookAsync(ctx, playbookID, alertID, agentID, uuid.New())

	return execution, nil
}

// GetMatchingRules retrieves rules that match an alert
func (s *AutomationService) GetMatchingRules(ctx context.Context, alert *models.Alert) ([]*models.AutomationRule, error) {
	return s.automationRepo.GetMatchingRules(ctx, alert)
}

// GetPlaybookByID retrieves a playbook by ID
func (s *AutomationService) GetPlaybookByID(ctx context.Context, playbookID uuid.UUID) (*models.ResponsePlaybook, error) {
	return s.playbookRepo.GetByID(ctx, playbookID)
}

// CreatePlaybook creates a new playbook
func (s *AutomationService) CreatePlaybook(ctx context.Context, playbook *models.ResponsePlaybook) error {
	return s.playbookRepo.Create(ctx, playbook)
}

// ListPlaybooks retrieves all playbooks
func (s *AutomationService) ListPlaybooks(ctx context.Context, filter repository.PlaybookFilter) ([]*models.ResponsePlaybook, error) {
	return s.playbookRepo.List(ctx, filter)
}

// ListRules retrieves all automation rules
func (s *AutomationService) ListRules(ctx context.Context) ([]*models.AutomationRule, error) {
	return s.automationRepo.List(ctx)
}

// CreateRule creates a new automation rule
func (s *AutomationService) CreateRule(ctx context.Context, rule *models.AutomationRule) error {
	return s.automationRepo.Create(ctx, rule)
}

// GetRuleByID retrieves an automation rule by ID
func (s *AutomationService) GetRuleByID(ctx context.Context, ruleID uuid.UUID) (*models.AutomationRule, error) {
	return s.automationRepo.GetByID(ctx, ruleID)
}

// UpdateRule updates an automation rule
func (s *AutomationService) UpdateRule(ctx context.Context, rule *models.AutomationRule) error {
	return s.automationRepo.Update(ctx, rule)
}

// DeleteRule deletes an automation rule
func (s *AutomationService) DeleteRule(ctx context.Context, ruleID uuid.UUID) error {
	return s.automationRepo.Delete(ctx, ruleID)
}

// DeletePlaybook deletes a playbook
func (s *AutomationService) DeletePlaybook(ctx context.Context, playbookID uuid.UUID) error {
	return s.playbookRepo.Delete(ctx, playbookID)
}

// GetAlertByID retrieves an alert by ID
func (s *AutomationService) GetAlertByID(ctx context.Context, alertID uuid.UUID) (*models.Alert, error) {
	return s.alertRepo.GetByID(ctx, alertID)
}

// GetRuleOptimizations retrieves ML-based rule optimization suggestions
func (s *AutomationService) GetRuleOptimizations(ctx context.Context) ([]RuleOptimization, error) {
	return s.mlOptimizer.SuggestRuleOptimizations(ctx)
}

// Helper function to check if slice contains element
func contains(slice []interface{}, item interface{}) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RuleOptimization represents a suggested optimization for a rule
type RuleOptimization struct {
	Type           string      `json:"type"`
	RuleID         uuid.UUID   `json:"rule_id"`
	Reason         string      `json:"reason"`
	SuggestedValue interface{} `json:"suggested_value"`
	Confidence     float64     `json:"confidence"`
}
