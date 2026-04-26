package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// MetricsService handles automation metrics collection and analysis
type MetricsService struct {
	logger     *logrus.Logger
	metricsRepo repository.AutomationMetricsRepository
}

// NewMetricsService creates a new metrics service instance
func NewMetricsService(
	logger *logrus.Logger,
	metricsRepo repository.AutomationMetricsRepository,
) *MetricsService {
	return &MetricsService{
		logger:      logger,
		metricsRepo: metricsRepo,
	}
}

// RecordAlertProcessing records metrics for alert processing
func (s *MetricsService) RecordAlertProcessing(alert *models.Alert, rulesCount, suggestionsCount int, processingTime time.Duration) {
	s.logger.WithFields(logrus.Fields{
		"alert_id":        alert.ID,
		"severity":        alert.Severity,
		"rules_count":     rulesCount,
		"suggestions_count": suggestionsCount,
		"processing_time_ms": processingTime.Milliseconds(),
	}).Info("Alert processing completed")
}

// RecordRuleExecution records metrics for a rule execution
func (s *MetricsService) RecordRuleExecution(ruleID uuid.UUID, success bool, executionTime time.Duration) {
	ctx := context.Background()
	
	err := s.metricsRepo.RecordRuleExecution(ctx, ruleID, success, executionTime)
	if err != nil {
		s.logger.WithError(err).Error("Failed to record rule execution metrics")
	}

	s.logger.WithFields(logrus.Fields{
		"rule_id":         ruleID,
		"success":         success,
		"execution_time_ms": executionTime.Milliseconds(),
	}).Info("Rule execution recorded")
}

// GetMetrics retrieves overall automation metrics
func (s *MetricsService) GetMetrics(ctx context.Context, timeRange string) (*models.AutomationMetricsSummary, error) {
	return s.metricsRepo.GetMetrics(ctx, timeRange)
}

// GetRulePerformance retrieves performance metrics for a specific rule
func (s *MetricsService) GetRulePerformance(ctx context.Context, ruleID uuid.UUID, since time.Time) (*models.AutomationMetrics, error) {
	return s.metricsRepo.GetRuleMetrics(ctx, ruleID, since)
}

// RecordCommandExecution records metrics for command execution
func (s *MetricsService) RecordCommandExecution(commandType string, status string, executionTime time.Duration) {
	s.logger.WithFields(logrus.Fields{
		"command_type":      commandType,
		"status":           status,
		"execution_time_ms": executionTime.Milliseconds(),
	}).Info("Command execution recorded")
}

// GetAlertStats retrieves alert statistics
func (s *MetricsService) GetAlertStats(ctx context.Context) (*AlertStats, error) {
	// This would typically query the alert_stats table
	// For now, return a placeholder structure
	return &AlertStats{
		TotalAlerts:   0,
		CriticalAlerts: 0,
		HighAlerts:     0,
		MediumAlerts:   0,
		LowAlerts:      0,
		LastUpdated:   time.Now(),
	}, nil
}

// AlertStats represents alert statistics
type AlertStats struct {
	TotalAlerts   int       `json:"total_alerts"`
	CriticalAlerts int       `json:"critical_alerts"`
	HighAlerts     int       `json:"high_alerts"`
	MediumAlerts   int       `json:"medium_alerts"`
	LowAlerts      int       `json:"low_alerts"`
	LastUpdated    time.Time `json:"last_updated"`
}
