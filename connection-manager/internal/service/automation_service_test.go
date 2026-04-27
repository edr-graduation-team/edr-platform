package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// Mock implementations for testing
type MockAlertRepository struct {
	mock.Mock
}

func (m *MockAlertRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*models.Alert), args.Error(1)
}

type MockPlaybookRepository struct {
	mock.Mock
}

func (m *MockPlaybookRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ResponsePlaybook, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*models.ResponsePlaybook), args.Error(1)
}

type MockAutomationRuleRepository struct {
	mock.Mock
}

func (m *MockAutomationRuleRepository) GetMatchingRules(ctx context.Context, alert *models.Alert) ([]*models.AutomationRule, error) {
	args := m.Called(ctx, alert)
	return args.Get(0).([]*models.AutomationRule), args.Error(1)
}

type MockPlaybookExecutionRepository struct {
	mock.Mock
}

func (m *MockPlaybookExecutionRepository) Create(ctx context.Context, execution *models.PlaybookExecution) error {
	args := m.Called(ctx, execution)
	return args.Error(0)
}

func (m *MockPlaybookExecutionRepository) CreateSuggestion(ctx context.Context, suggestion *models.PlaybookSuggestion) error {
	args := m.Called(ctx, suggestion)
	return args.Error(0)
}

func (m *MockPlaybookExecutionRepository) GetSuggestions(ctx context.Context, alertID uuid.UUID) ([]models.PlaybookSuggestion, error) {
	args := m.Called(ctx, alertID)
	return args.Get(0).([]models.PlaybookSuggestion), args.Error(1)
}

type MockCommandService struct {
	mock.Mock
}

func (m *MockCommandService) ExecutePlaybookCommand(ctx context.Context, executionID uuid.UUID, cmd models.PlaybookCommand, agentID uuid.UUID) *CommandResult {
	args := m.Called(ctx, executionID, cmd, agentID)
	return args.Get(0).(*CommandResult)
}

type MockNotificationService struct {
	mock.Mock
}

func (m *MockNotificationService) SendAlertProcessed(alert *models.Alert, rulesCount, suggestionsCount int) {
	m.Called(alert, rulesCount, suggestionsCount)
}

func (m *MockNotificationService) SendPlaybookCompleted(execution *models.PlaybookExecution, success bool) {
	m.Called(execution, success)
}

type MockMetricsService struct {
	mock.Mock
}

func (m *MockMetricsService) RecordAlertProcessing(alert *models.Alert, rulesCount, suggestionsCount int, processingTime time.Duration) {
	m.Called(alert, rulesCount, suggestionsCount, processingTime)
}

func (m *MockMetricsService) RecordRuleExecution(ruleID uuid.UUID, success bool, executionTime time.Duration) {
	m.Called(ruleID, success, executionTime)
}

type MockMLOptimizer struct {
	mock.Mock
}

func (m *MockMLOptimizer) OptimizeRulePriority(rules []*models.AutomationRule, alert *models.Alert) []*models.AutomationRule {
	args := m.Called(rules, alert)
	return args.Get(0).([]*models.AutomationRule)
}

func (m *MockMLOptimizer) SuggestRuleOptimizations(ctx context.Context) ([]RuleOptimization, error) {
	args := m.Called(ctx)
	return args.Get(0).([]RuleOptimization), args.Error(1)
}

func TestAutomationService_ProcessAlert(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	mockAlertRepo := &MockAlertRepository{}
	mockPlaybookRepo := &MockPlaybookRepository{}
	mockAutomationRepo := &MockAutomationRuleRepository{}
	mockExecutionRepo := &MockPlaybookExecutionRepository{}
	mockCommandService := &MockCommandService{}
	mockNotificationService := &MockNotificationService{}
	mockMetricsService := &MockMetricsService{}
	mockMLOptimizer := &MockMLOptimizer{}

	service := NewAutomationService(
		logger,
		mockAlertRepo,
		mockPlaybookRepo,
		mockAutomationRepo,
		mockExecutionRepo,
		mockCommandService,
		mockNotificationService,
		mockMetricsService,
		mockMLOptimizer,
	)

	// Create test alert
	alertID := uuid.New()
	agentID := uuid.New()
	alert := &models.Alert{
		ID:        alertID,
		Severity:  "critical",
		Title:     "Test Alert",
		RuleName:  "malware_detected",
		AgentID:   agentID,
		RiskScore: 95,
		Confidence: 0.9,
	}

	// Create test rules
	ruleID := uuid.New()
	playbookID := uuid.New()
	rules := []*models.AutomationRule{
		{
			ID:          ruleID,
			Name:        "Test Rule",
			PlaybookID:  playbookID,
			Priority:    1,
			AutoExecute: true,
			Enabled:     true,
			SuccessRate: 0.95,
		},
	}

	// Setup mocks
	mockAutomationRepo.On("GetMatchingRules", mock.Anything, alert).Return(rules, nil)
	mockMLOptimizer.On("OptimizeRulePriority", rules, alert).Return(rules)
	mockExecutionRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.PlaybookExecution")).Return(nil)
	mockExecutionRepo.On("CreateSuggestion", mock.Anything, mock.AnythingOfType("*models.PlaybookSuggestion")).Return(nil)
	mockNotificationService.On("SendAlertProcessed", alert, 1, mock.AnythingOfType("int"))
	mockMetricsService.On("RecordAlertProcessing", alert, 1, mock.AnythingOfType("int"), mock.AnythingOfType("time.Duration"))
	mockCommandService.On("ExecutePlaybookCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CommandResult{
		Status:      "completed",
		CompletedAt: time.Now(),
	})
	mockMetricsService.On("RecordRuleExecution", ruleID, true, mock.AnythingOfType("time.Duration"))
	mockNotificationService.On("SendPlaybookCompleted", mock.Anything, true)

	// Execute
	err := service.ProcessAlert(context.Background(), alert)

	// Assert
	assert.NoError(t, err)
	mockAutomationRepo.AssertExpectations(t)
	mockMLOptimizer.AssertExpectations(t)
	mockNotificationService.AssertExpectations(t)
	mockMetricsService.AssertExpectations(t)
}

func TestAutomationService_ExecutePlaybookForAlert(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockAlertRepo := &MockAlertRepository{}
	mockPlaybookRepo := &MockPlaybookRepository{}
	mockAutomationRepo := &MockAutomationRuleRepository{}
	mockExecutionRepo := &MockPlaybookExecutionRepository{}
	mockCommandService := &MockCommandService{}
	mockNotificationService := &MockNotificationService{}
	mockMetricsService := &MockMetricsService{}
	mockMLOptimizer := &MockMLOptimizer{}

	service := NewAutomationService(
		logger,
		mockAlertRepo,
		mockPlaybookRepo,
		mockAutomationRepo,
		mockExecutionRepo,
		mockCommandService,
		mockNotificationService,
		mockMetricsService,
		mockMLOptimizer,
	)

	// Create test data
	alertID := uuid.New()
	playbookID := uuid.New()
	userID := uuid.New()

	// Setup mocks
	mockExecutionRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.PlaybookExecution")).Return(nil)

	// Execute
	execution, err := service.ExecutePlaybookForAlert(context.Background(), playbookID, alertID, userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, alertID, execution.AlertID)
	assert.Equal(t, playbookID, execution.PlaybookID)
	assert.Equal(t, "running", execution.Status)
	assert.NotNil(t, execution.CreatedBy)
	assert.Equal(t, userID, *execution.CreatedBy)

	mockExecutionRepo.AssertExpectations(t)
}

func TestAutomationService_GetSuggestions(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockAlertRepo := &MockAlertRepository{}
	mockPlaybookRepo := &MockPlaybookRepository{}
	mockAutomationRepo := &MockAutomationRuleRepository{}
	mockExecutionRepo := &MockPlaybookExecutionRepository{}
	mockCommandService := &MockCommandService{}
	mockNotificationService := &MockNotificationService{}
	mockMetricsService := &MockMetricsService{}
	mockMLOptimizer := &MockMLOptimizer{}

	service := NewAutomationService(
		logger,
		mockAlertRepo,
		mockPlaybookRepo,
		mockAutomationRepo,
		mockExecutionRepo,
		mockCommandService,
		mockNotificationService,
		mockMetricsService,
		mockMLOptimizer,
	)

	// Create test data
	alertID := uuid.New()
	suggestions := []models.PlaybookSuggestion{
		{
			AlertID:    alertID,
			PlaybookID: uuid.New(),
			Confidence: 0.95,
			Reason:     "Critical severity alert",
		},
	}

	// Setup mocks
	mockExecutionRepo.On("GetSuggestions", mock.Anything, alertID).Return(suggestions, nil)

	// Execute
	result, err := service.GetSuggestions(context.Background(), alertID)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, suggestions[0].AlertID, result[0].AlertID)
	assert.Equal(t, suggestions[0].Confidence, result[0].Confidence)

	mockExecutionRepo.AssertExpectations(t)
}

func TestAutomationService_evaluateAdvancedConditions(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewAutomationService(
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)

	// Create test alert
	alert := &models.Alert{
		Severity:  "critical",
		RuleName: "malware_detected",
		Confidence: 0.9,
	}

	// Test case 1: Matching severity and rule pattern
	conditions := map[string]interface{}{
		"severity":       []interface{}{"critical", "high"},
		"rule_patterns":  []interface{}{"malware", "trojan"},
		"confidence_threshold": 0.8,
		"logic_operator": "AND",
	}

	result := service.evaluateAdvancedConditions(conditions, alert)
	assert.True(t, result)

	// Test case 2: Non-matching severity
	conditions["severity"] = []interface{}{"medium", "low"}
	result = service.evaluateAdvancedConditions(conditions, alert)
	assert.False(t, result)

	// Test case 3: OR logic with one match
	conditions["logic_operator"] = "OR"
	conditions["severity"] = []interface{}{"medium", "critical"}
	result = service.evaluateAdvancedConditions(conditions, alert)
	assert.True(t, result)

	// Test case 4: Confidence threshold too high
	conditions["confidence_threshold"] = 0.95
	result = service.evaluateAdvancedConditions(conditions, alert)
	assert.False(t, result)
}

func TestAutomationService_generateIntelligentSuggestions(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	service := NewAutomationService(
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)

	// Test case 1: Critical severity alert
	alert := &models.Alert{
		Severity:  "critical",
		RuleName:  "test_rule",
		Confidence: 0.8,
	}

	suggestions := service.generateIntelligentSuggestions(alert)
	assert.Len(t, suggestions, 1)
	assert.Equal(t, "execute_immediate_containment", suggestions[0].Reason)
	assert.Equal(t, 0.95, suggestions[0].Confidence)

	// Test case 2: Malware rule name
	alert.RuleName = "malware_detected"
	suggestions = service.generateIntelligentSuggestions(alert)
	assert.Len(t, suggestions, 2)
	assert.Contains(t, suggestions, func(s models.PlaybookSuggestion) bool {
		return s.Reason == "Critical severity alert requires immediate containment"
	})
	assert.Contains(t, suggestions, func(s models.PlaybookSuggestion) bool {
		return s.Reason == "Malware-related behavior detected"
	})

	// Test case 3: High risk score
	alert.RiskScore = 9
	alert.Severity = "medium"
	suggestions = service.generateIntelligentSuggestions(alert)
	assert.Len(t, suggestions, 2)
	assert.Contains(t, suggestions, func(s models.PlaybookSuggestion) bool {
		return s.Reason == "High-risk agent detected"
	})

	// Test case 4: Ransomware
	alert.RuleName = "ransomware_encryption"
	suggestions = service.generateIntelligentSuggestions(alert)
	assert.Len(t, suggestions, 2)
	assert.Contains(t, suggestions, func(s models.PlaybookSuggestion) bool {
		return s.Reason == "Ransomware activity detected"
	})
}

func TestMLOptimizer_OptimizeRulePriority(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockMetricsRepo := &struct {
		repository.AutomationMetricsRepository
		mock.Mock
	}{}
	mockRuleRepo := &struct {
		repository.AutomationRuleRepository
		mock.Mock
	}{}

	optimizer := NewMLOptimizer(logger, mockMetricsRepo, mockRuleRepo)

	// Create test rules
	rules := []*models.AutomationRule{
		{
			ID:            uuid.New(),
			Name:          "High Priority Rule",
			Priority:      1,
			SuccessRate:   0.95,
			LastExecution: &time.Time{},
		},
		{
			ID:            uuid.New(),
			Name:          "Low Priority Rule",
			Priority:      10,
			SuccessRate:   0.6,
			LastExecution: &time.Time{},
		},
	}

	// Create test alert
	alert := &models.Alert{
		Severity: "critical",
		RuleName: "test_rule",
	}

	// Execute
	optimized := optimizer.OptimizeRulePriority(rules, alert)

	// Assert
	assert.Len(t, optimized, 2)
	// High priority rule should remain first due to better success rate
	assert.Equal(t, "High Priority Rule", optimized[0].Name)
	assert.Equal(t, "Low Priority Rule", optimized[1].Name)
}

func TestMLOptimizer_PredictRuleSuccess(t *testing.T) {
	// Setup
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockMetricsRepo := &struct {
		repository.AutomationMetricsRepository
		mock.Mock
	}{}
	mockRuleRepo := &struct {
		repository.AutomationRuleRepository
		mock.Mock
	}{}

	optimizer := NewMLOptimizer(logger, mockMetricsRepo, mockRuleRepo)

	// Create test rule
	rule := &models.AutomationRule{
		ID:            uuid.New(),
		Name:          "Test Rule",
		Priority:      5,
		SuccessRate:   0.8,
		LastExecution: &time.Time{},
	}

	// Create test alert
	alert := &models.Alert{
		Severity:  "critical",
		RuleName:  "test_rule",
		Confidence: 0.9,
	}

	// Execute
	prediction := optimizer.PredictRuleSuccess(rule, alert)

	// Assert
	assert.Greater(t, prediction, 0.0)
	assert.LessOrEqual(t, prediction, 1.0)
	// Should be boosted for critical alert with high priority rule
	assert.Greater(t, prediction, rule.SuccessRate)
}
