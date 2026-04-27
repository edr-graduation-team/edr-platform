package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// MLOptimizer provides machine learning-based optimizations for automation rules
type MLOptimizer struct {
	logger      *logrus.Logger
	metricsRepo repository.AutomationMetricsRepository
	ruleRepo    repository.AutomationRuleRepository
}

// RulePerformance tracks performance metrics for automation rules
type RulePerformance struct {
	RuleID        uuid.UUID
	SuccessRate   float64
	AvgExecTime   time.Duration
	LastExecuted  time.Time
	Frequency     int
}

// NewMLOptimizer creates a new ML optimizer instance
func NewMLOptimizer(
	logger *logrus.Logger,
	metricsRepo repository.AutomationMetricsRepository,
	ruleRepo repository.AutomationRuleRepository,
) *MLOptimizer {
	return &MLOptimizer{
		logger:      logger,
		metricsRepo: metricsRepo,
		ruleRepo:    ruleRepo,
	}
}

// OptimizeRulePriority optimizes rule priorities based on performance
func (m *MLOptimizer) OptimizeRulePriority(rules []*models.AutomationRule, alert *models.Alert) []*models.AutomationRule {
	// Get performance data
	performances := m.getRulePerformances(rules)

	// Sort rules based on performance and original priority
	sortedRules := make([]*models.AutomationRule, len(rules))
	copy(sortedRules, rules)

	sort.Slice(sortedRules, func(i, j int) bool {
		perfI := performances[sortedRules[i].ID]
		perfJ := performances[sortedRules[j].ID]

		// Prioritize rules with better performance
		if perfI.SuccessRate > perfJ.SuccessRate {
			return true
		}
		if perfI.SuccessRate < perfJ.SuccessRate {
			return false
		}

		// Then consider original priority
		return sortedRules[i].Priority < sortedRules[j].Priority
	})

	return sortedRules
}

// getRulePerformances retrieves performance data for rules
func (m *MLOptimizer) getRulePerformances(rules []*models.AutomationRule) map[uuid.UUID]RulePerformance {
	performances := make(map[uuid.UUID]RulePerformance)

	for _, rule := range rules {
		metrics, err := m.metricsRepo.GetRuleMetrics(context.Background(), rule.ID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			m.logger.WithError(err).Warnf("Failed to get metrics for rule %s", rule.ID)
			continue
		}

		var successRate float64
		var avgExecTime time.Duration

		if metrics.ExecutionsCount > 0 {
			successRate = float64(metrics.SuccessfulExecutions) / float64(metrics.ExecutionsCount)
			avgExecTime = time.Duration(metrics.AvgExecutionTimeMs) * time.Millisecond
		}

		lastExecuted := time.Time{}
	if rule.LastExecution != nil {
		lastExecuted = *rule.LastExecution
	}
	performances[rule.ID] = RulePerformance{
			RuleID:       rule.ID,
			SuccessRate:  successRate,
			AvgExecTime:  avgExecTime,
			LastExecuted: lastExecuted,
			Frequency:    metrics.ExecutionsCount,
		}
	}

	return performances
}

// SuggestRuleOptimizations generates optimization suggestions for rules
func (m *MLOptimizer) SuggestRuleOptimizations(ctx context.Context) ([]RuleOptimization, error) {
	rules, err := m.ruleRepo.List(ctx)
	if err != nil {
		m.logger.WithError(err).Error("Failed to list rules")
		return nil, err
	}

	var optimizations []RuleOptimization
	performances := m.getRulePerformances(rules)

	for _, rule := range rules {
		perf := performances[rule.ID]

		// Suggest disabling low-performing rules
		if perf.SuccessRate < 0.3 && perf.Frequency > 10 {
			optimizations = append(optimizations, RuleOptimization{
				Type:       "disable",
				RuleID:     rule.ID,
				Reason:     "Low success rate with high frequency",
				Confidence: 0.9,
			})
		}

		// Suggest increasing cooldown for fast-executing rules
		if perf.AvgExecTime < 5*time.Second && rule.CooldownMinutes < 15 {
			optimizations = append(optimizations, RuleOptimization{
				Type:            "increase_cooldown",
				RuleID:          rule.ID,
				Reason:          "Fast execution with low cooldown",
				SuggestedValue:  30,
				Confidence:      0.8,
			})
		}

		// Suggest enabling auto-execute for high-performing manual rules
		if perf.SuccessRate > 0.95 && !rule.AutoExecute && perf.Frequency > 5 {
			optimizations = append(optimizations, RuleOptimization{
				Type:       "enable_auto_execute",
				RuleID:     rule.ID,
				Reason:     "Excellent success rate with manual execution",
				Confidence: 0.85,
			})
		}

		// Suggest adjusting confidence threshold
		if perf.SuccessRate < 0.6 && perf.Frequency > 20 {
			currentThreshold := 0.7 // Default assumption
			var conditions map[string]interface{}
			if len(rule.TriggerConditions) > 0 {
				if err := json.Unmarshal(rule.TriggerConditions, &conditions); err == nil {
					if threshold, ok := conditions["confidence_threshold"].(float64); ok {
						currentThreshold = threshold
					}
				}
			}

			if currentThreshold > 0.5 {
				optimizations = append(optimizations, RuleOptimization{
					Type:            "adjust_confidence_threshold",
					RuleID:          rule.ID,
					Reason:          "Low success rate suggests threshold is too high",
					SuggestedValue:  currentThreshold - 0.1,
					Confidence:      0.75,
				})
			}
		}

		// Suggest priority adjustments based on execution time
		if perf.AvgExecTime > 30*time.Minute && rule.Priority < 50 {
			optimizations = append(optimizations, RuleOptimization{
				Type:            "increase_priority",
				RuleID:          rule.ID,
				Reason:          "Slow execution should have higher priority",
				SuggestedValue:  rule.Priority - 10,
				Confidence:      0.7,
			})
		}
	}

	return optimizations, nil
}

// PredictRuleSuccess predicts the likelihood of successful execution for a rule
func (m *MLOptimizer) PredictRuleSuccess(rule *models.AutomationRule, alert *models.Alert) float64 {
	// Get historical performance
	performances := m.getRulePerformances([]*models.AutomationRule{rule})
	perf, exists := performances[rule.ID]
	if !exists {
		return 0.5 // Default prediction for new rules
	}

	// Base prediction on historical success rate
	prediction := perf.SuccessRate

	// Adjust based on alert characteristics
	if alert.Severity == "critical" && rule.Priority > 50 {
		prediction *= 1.1 // Boost for critical alerts with high priority rules
	}

	if alert.Confidence > 0.9 && prediction < 0.8 {
		prediction += 0.1 // Boost for high-confidence alerts
	}

	// Ensure prediction stays within bounds
	if prediction > 1.0 {
		prediction = 1.0
	}
	if prediction < 0.0 {
		prediction = 0.0
	}

	return prediction
}

// AnalyzeExecutionPatterns analyzes execution patterns to identify trends
func (m *MLOptimizer) AnalyzeExecutionPatterns(ctx context.Context) (*ExecutionPatternAnalysis, error) {
	rules, err := m.ruleRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	analysis := &ExecutionPatternAnalysis{
		TotalRules:        len(rules),
		EnabledRules:      0,
		AutoExecuteRules:  0,
		AvgSuccessRate:    0.0,
		AvgExecutionTime:  0,
		PeakHour:          0,
		HighPerformingRules: 0,
		LowPerformingRules:  0,
	}

	var totalSuccessRate float64
	var totalExecutionTime time.Duration
	var ruleCount int

	performances := m.getRulePerformances(rules)

	for _, rule := range rules {
		if rule.Enabled {
			analysis.EnabledRules++
		}
		if rule.AutoExecute {
			analysis.AutoExecuteRules++
		}

		perf := performances[rule.ID]
		totalSuccessRate += perf.SuccessRate
		totalExecutionTime += perf.AvgExecTime
		ruleCount++

		if perf.SuccessRate > 0.9 {
			analysis.HighPerformingRules++
		}
		if perf.SuccessRate < 0.5 {
			analysis.LowPerformingRules++
		}
	}

	if ruleCount > 0 {
		analysis.AvgSuccessRate = totalSuccessRate / float64(ruleCount)
		analysis.AvgExecutionTime = totalExecutionTime / time.Duration(ruleCount)
	}

	// Analyze peak execution hour (simplified - would need more data for accurate analysis)
	analysis.PeakHour = 14 // 2 PM as a common peak time

	return analysis, nil
}

// ExecutionPatternAnalysis contains analysis of execution patterns
type ExecutionPatternAnalysis struct {
	TotalRules           int           `json:"total_rules"`
	EnabledRules         int           `json:"enabled_rules"`
	AutoExecuteRules     int           `json:"auto_execute_rules"`
	AvgSuccessRate       float64       `json:"avg_success_rate"`
	AvgExecutionTime     time.Duration `json:"avg_execution_time"`
	PeakHour             int           `json:"peak_hour"`
	HighPerformingRules  int           `json:"high_performing_rules"`
	LowPerformingRules   int           `json:"low_performing_rules"`
}
