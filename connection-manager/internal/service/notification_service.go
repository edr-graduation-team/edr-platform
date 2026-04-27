package service

import (
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// NotificationService handles notifications for automation events
type NotificationService struct {
	logger *logrus.Logger
}

// NewNotificationService creates a new notification service instance
func NewNotificationService(logger *logrus.Logger) *NotificationService {
	return &NotificationService{
		logger: logger,
	}
}

// SendAlertProcessed sends notification when an alert is processed
func (s *NotificationService) SendAlertProcessed(alert *models.Alert, rulesCount, suggestionsCount int) {
	s.logger.WithFields(logrus.Fields{
		"alert_id":          alert.ID,
		"severity":          alert.Severity,
		"rule_name":         alert.RuleName,
		"agent_id":          alert.AgentID,
		"matching_rules":    rulesCount,
		"suggestions_count": suggestionsCount,
	}).Info("Alert processing notification sent")

	// In a real implementation, this would send notifications via:
	// - WebSocket to connected clients
	// - Email notifications
	// - Slack/Teams integrations
	// - Push notifications
}

// SendPlaybookCompleted sends notification when a playbook execution completes
func (s *NotificationService) SendPlaybookCompleted(execution *models.PlaybookExecution, success bool) {
	status := "completed"
	if !success {
		status = "failed"
	}

	s.logger.WithFields(logrus.Fields{
		"execution_id":       execution.ID,
		"alert_id":          execution.AlertID,
		"playbook_id":        execution.PlaybookID,
		"agent_id":          execution.AgentID,
		"status":            status,
		"commands_executed": execution.CommandsExecuted,
		"commands_total":    execution.CommandsTotal,
		"execution_time_ms":  execution.ExecutionTimeMs,
	}).Info("Playbook execution notification sent")

	// Send real-time notifications to dashboard clients
	// Send email notifications for critical failures
	// Update alert status based on execution results
}

// SendRuleExecuted sends notification when an automation rule is executed
func (s *NotificationService) SendRuleExecuted(ruleID uuid.UUID, alertID uuid.UUID, success bool) {
	s.logger.WithFields(logrus.Fields{
		"rule_id":  ruleID,
		"alert_id": alertID,
		"success":  success,
	}).Info("Automation rule execution notification sent")
}

// SendErrorNotification sends notification for automation errors
func (s *NotificationService) SendErrorNotification(component string, err error, context map[string]interface{}) {
	fields := logrus.Fields{
		"component": component,
		"error":     err.Error(),
	}
	
	for k, v := range context {
		fields[k] = v
	}
	
	s.logger.WithFields(fields).Error("Automation error notification sent")
}

// SendOptimizationSuggestion sends notification for ML optimization suggestions
func (s *NotificationService) SendOptimizationSuggestion(suggestions []interface{}) {
	s.logger.WithField("suggestions_count", len(suggestions)).Info("Optimization suggestions notification sent")
}

// SendCooldownNotification sends notification when a rule is in cooldown
func (s *NotificationService) SendCooldownNotification(ruleID uuid.UUID, ruleName string, cooldownMinutes int) {
	s.logger.WithFields(logrus.Fields{
		"rule_id":         ruleID,
		"rule_name":       ruleName,
		"cooldown_minutes": cooldownMinutes,
	}).Info("Rule cooldown notification sent")
}

// SendMetricsUpdate sends notification when metrics are updated
func (s *NotificationService) SendMetricsUpdate(metrics *models.AutomationMetricsSummary) {
	s.logger.WithFields(logrus.Fields{
		"total_executions":       metrics.TotalExecutions,
		"successful_executions": metrics.SuccessfulExecutions,
		"failed_executions":     metrics.FailedExecutions,
		"success_rate":          metrics.SuccessRate,
		"avg_execution_time":    metrics.AvgExecutionTime,
	}).Info("Metrics update notification sent")
}
