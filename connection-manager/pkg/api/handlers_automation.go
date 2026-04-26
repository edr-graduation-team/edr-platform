package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/models"
)

// AutomationHandlers handles automation-related API endpoints
type AutomationHandlers struct {
	logger            *logrus.Logger
	automationService *service.AutomationService
	metricsService    *service.MetricsService
}

// NewAutomationHandlers creates new automation handlers
func NewAutomationHandlers(
	logger *logrus.Logger,
	automationService *service.AutomationService,
	metricsService *service.MetricsService,
) *AutomationHandlers {
	return &AutomationHandlers{
		logger:            logger,
		automationService: automationService,
		metricsService:    metricsService,
	}
}

// CreatePlaybookRequest represents a request to create a playbook
type CreatePlaybookRequest struct {
	Name        string                   `json:"name" validate:"required"`
	Description string                   `json:"description,omitempty"`
	Category    string                   `json:"category" validate:"required"`
	Commands    []models.PlaybookCommand `json:"commands" validate:"required"`
}

// ExecutePlaybookRequest represents a request to execute a playbook
type ExecutePlaybookRequest struct {
	PlaybookID uuid.UUID `json:"playbook_id" validate:"required"`
}

// PlaybookListResponse represents a response with playbook list
type PlaybookListResponse struct {
	Data    []*models.ResponsePlaybook `json:"data"`
	Total   int                         `json:"total"`
	Meta    ResponseMeta                `json:"meta"`
}

// AutomationRuleListResponse represents a response with automation rule list
type AutomationRuleListResponse struct {
	Data    []*models.AutomationRule `json:"data"`
	Total   int                      `json:"total"`
	Meta    ResponseMeta             `json:"meta"`
}

// CreatePlaybook creates a new response playbook
func (h *AutomationHandlers) CreatePlaybook(c echo.Context) error {
	var req CreatePlaybookRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	// Validate request
	if err := h.validatePlaybookRequest(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}

	playbook := &models.ResponsePlaybook{
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Commands:    marshalCommands(req.Commands),
		CreatedBy:    getCurrentUserID(c),
	}

	if err := h.automationService.CreatePlaybook(c.Request().Context(), playbook); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create playbook")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Playbook created successfully",
		"data":    playbook,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListPlaybooks retrieves all response playbooks with optional filtering
func (h *AutomationHandlers) ListPlaybooks(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract query parameters
	category := c.QueryParam("category")
	enabled := c.QueryParam("enabled")
	alertID := c.QueryParam("alert_id")

	var playbooks []*models.ResponsePlaybook
	var err error

	if alertID != "" {
		// Get playbook suggestions for a specific alert
		alertUUID, err := uuid.Parse(alertID)
		if err != nil {
			return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid alert ID")
		}

		suggestions, err := h.automationService.GetSuggestions(ctx, alertUUID)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get suggestions")
		}

		// Convert suggestions to playbooks
		for _, suggestion := range suggestions {
			playbook, err := h.automationService.GetPlaybookByID(ctx, suggestion.PlaybookID)
			if err == nil {
				playbooks = append(playbooks, playbook)
			}
		}
	} else {
		// Get all playbooks with filtering
		filter := repository.PlaybookFilter{
			Category: &category,
			Enabled:  &enabled,
		}
		playbooks, err = h.automationService.ListPlaybooks(ctx, filter)
	}

	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch playbooks")
	}

	return c.JSON(http.StatusOK, PlaybookListResponse{
		Data:    playbooks,
		Total:   len(playbooks),
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetPlaybook retrieves a specific playbook by ID
func (h *AutomationHandlers) GetPlaybook(c echo.Context) error {
	playbookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid playbook ID")
	}

	playbook, err := h.automationService.GetPlaybookByID(c.Request().Context(), playbookID)
	if err != nil {
		return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Playbook not found")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": playbook,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ListAutomationRules retrieves all automation rules with optional filtering for alert
func (h *AutomationHandlers) ListAutomationRules(c echo.Context) error {
	ctx := c.Request().Context()
	alertIDStr := c.QueryParam("alert_id")

	var rules []*models.AutomationRule
	var err error

	if alertIDStr != "" {
		// Get rules matching a specific alert
		alertID, err := uuid.Parse(alertIDStr)
		if err != nil {
			return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid alert ID")
		}

		alert, err := h.automationService.GetAlertByID(ctx, alertID)
		if err != nil {
			return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Alert not found")
		}

		rules, err = h.automationService.GetMatchingRules(ctx, alert)
	} else {
		rules, err = h.automationService.ListRules(ctx)
	}

	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch rules")
	}

	return c.JSON(http.StatusOK, AutomationRuleListResponse{
		Data:    rules,
		Total:   len(rules),
		Meta: ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// CreateAutomationRule creates a new automation rule
func (h *AutomationHandlers) CreateAutomationRule(c echo.Context) error {
	var rule models.AutomationRule
	if err := c.Bind(&rule); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	if err := h.automationService.CreateRule(c.Request().Context(), &rule); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create rule")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Automation rule created successfully",
		"data":    rule,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ToggleAutomationRule toggles the enabled state of an automation rule
func (h *AutomationHandlers) ToggleAutomationRule(c echo.Context) error {
	ruleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid rule ID")
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	ctx := c.Request().Context()
	rule, err := h.automationService.GetRuleByID(ctx, ruleID)
	if err != nil {
		return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Rule not found")
	}

	rule.Enabled = req.Enabled

	if err := h.automationService.UpdateRule(ctx, rule); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update rule")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Rule state updated successfully",
		"data":    rule,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ExecutePlaybookForAlert executes a playbook for a specific alert
func (h *AutomationHandlers) ExecutePlaybookForAlert(c echo.Context) error {
	alertID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid alert ID")
	}

	var req ExecutePlaybookRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	execution, err := h.automationService.ExecutePlaybookForAlert(
		c.Request().Context(),
		req.PlaybookID,
		alertID,
		getCurrentUserID(c),
	)

	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to execute playbook")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Playbook execution started",
		"data":    execution,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetPlaybookSuggestions retrieves playbook suggestions for an alert
func (h *AutomationHandlers) GetPlaybookSuggestions(c echo.Context) error {
	alertID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid alert ID")
	}

	suggestions, err := h.automationService.GetSuggestions(c.Request().Context(), alertID)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch suggestions")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": suggestions,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAutomationMetrics retrieves automation metrics
func (h *AutomationHandlers) GetAutomationMetrics(c echo.Context) error {
	timeRange := c.QueryParam("time_range")
	if timeRange == "" {
		timeRange = "7d" // Default to 7 days
	}

	metrics, err := h.metricsService.GetMetrics(c.Request().Context(), timeRange)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch metrics")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": metrics,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAutomationOptimizations retrieves ML-based optimization suggestions
func (h *AutomationHandlers) GetAutomationOptimizations(c echo.Context) error {
	optimizations, err := h.automationService.GetRuleOptimizations(c.Request().Context())
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get optimizations")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": optimizations,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetPlaybookExecutions retrieves execution history for an alert
func (h *AutomationHandlers) GetPlaybookExecutions(c echo.Context) error {
	_, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid alert ID")
	}

	// This would typically call a repository method
	// For now, return empty list as placeholder
	executions := []*models.PlaybookExecution{}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": executions,
		"meta": ResponseMeta{
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// validatePlaybookRequest validates playbook creation request
func (h *AutomationHandlers) validatePlaybookRequest(req *CreatePlaybookRequest) error {
	if req.Name == "" {
		return errors.New("playbook name is required")
	}

	if req.Category == "" {
		return errors.New("playbook category is required")
	}

	if len(req.Commands) == 0 {
		return errors.New("at least one command is required")
	}

	// Validate commands
	for _, cmd := range req.Commands {
		if cmd.Type == "" {
			return errors.New("command type is required")
		}

		if cmd.Timeout <= 0 {
			return errors.New("timeout must be greater than 0")
		}
	}

	return nil
}

// marshalCommands converts playbook commands to JSON
func marshalCommands(commands []models.PlaybookCommand) json.RawMessage {
	data, _ := json.Marshal(commands)
	return data
}

// getCurrentUserID gets the current user ID from context
func getCurrentUserID(c echo.Context) uuid.UUID {
	if userID, ok := c.Get("user_id").(uuid.UUID); ok {
		return userID
	}
	return uuid.New() // Fallback - should be properly implemented
}
