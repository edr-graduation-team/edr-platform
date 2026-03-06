// Package handlers provides Alert Management API endpoints.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/database"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/gorilla/mux"
)

// AlertHandler handles alert management API endpoints.
type AlertHandler struct {
	repo database.AlertRepository
}

// NewAlertHandler creates a new alert handler.
func NewAlertHandler(repo database.AlertRepository) *AlertHandler {
	return &AlertHandler{repo: repo}
}

// RegisterRoutes registers alert routes on the router.
func (h *AlertHandler) RegisterRoutes(r *mux.Router) {
	// Bulk operations MUST be registered before {alert_id} wildcard to avoid mux conflicts
	r.HandleFunc("/sigma/alerts/bulk/status", h.BulkUpdateAlertStatus).Methods("PATCH")
	r.HandleFunc("/sigma/alerts", h.QueryAlerts).Methods("GET")
	r.HandleFunc("/sigma/alerts/{alert_id}", h.GetAlert).Methods("GET")
	r.HandleFunc("/sigma/alerts/{alert_id}/status", h.UpdateAlertStatus).Methods("PATCH")
	r.HandleFunc("/sigma/alerts/{alert_id}/acknowledge", h.AcknowledgeAlert).Methods("POST")
	r.HandleFunc("/sigma/alerts/{alert_id}", h.DeleteAlert).Methods("DELETE")
}

// AlertResponse is the API response for an alert.
type AlertResponse struct {
	ID                string                 `json:"id"`
	Timestamp         time.Time              `json:"timestamp"`
	AgentID           string                 `json:"agent_id"`
	RuleID            string                 `json:"rule_id"`
	RuleTitle         string                 `json:"rule_title"`
	Severity          string                 `json:"severity"`
	Category          string                 `json:"category,omitempty"`
	EventCount        int                    `json:"event_count"`
	EventIDs          []string               `json:"event_ids,omitempty"`
	MitreTactics      []string               `json:"mitre_tactics,omitempty"`
	MitreTechniques   []string               `json:"mitre_techniques,omitempty"`
	MatchedFields     map[string]interface{} `json:"matched_fields,omitempty"`
	ContextData       map[string]interface{} `json:"context_data,omitempty"`
	Status            string                 `json:"status"`
	AssignedTo        string                 `json:"assigned_to,omitempty"`
	ResolutionNotes   string                 `json:"resolution_notes,omitempty"`
	Confidence        *float64               `json:"confidence,omitempty"`
	FalsePositiveRisk *float64               `json:"false_positive_risk,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// AlertsListResponse is the API response for listing alerts.
type AlertsListResponse struct {
	Count  int              `json:"count"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Alerts []*AlertResponse `json:"alerts"`
}

// QueryAlerts handles GET /api/v1/sigma/alerts
func (h *AlertHandler) QueryAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	filters := database.AlertFilters{
		Limit:  100,
		Offset: 0,
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 && limit <= 1000 {
			filters.Limit = limit
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		filters.AgentID = v
	}
	if v := r.URL.Query().Get("rule_id"); v != "" {
		filters.RuleID = v
	}
	if v := r.URL.Query().Get("severity"); v != "" {
		filters.Severity = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("status"); v != "" {
		filters.Status = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("date_from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.DateFrom = t
		}
	}
	if v := r.URL.Query().Get("date_to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.DateTo = t
		}
	}
	if v := r.URL.Query().Get("search"); v != "" {
		filters.Search = v
	}
	if v := r.URL.Query().Get("sort"); v != "" {
		if strings.HasPrefix(v, "-") {
			filters.SortBy = strings.TrimPrefix(v, "-")
			filters.SortOrder = "desc"
		} else {
			filters.SortBy = v
			filters.SortOrder = "asc"
		}
	}

	// Query database
	alerts, total, err := h.repo.List(ctx, filters)
	if err != nil {
		logger.Errorf("Failed to query alerts: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to query alerts")
		return
	}

	// Convert to response
	response := AlertsListResponse{
		Count:  len(alerts),
		Total:  total,
		Limit:  filters.Limit,
		Offset: filters.Offset,
		Alerts: make([]*AlertResponse, 0, len(alerts)),
	}

	for _, alert := range alerts {
		response.Alerts = append(response.Alerts, toAlertResponse(alert))
	}

	writeJSON(w, http.StatusOK, response)
}

// GetAlert handles GET /api/v1/sigma/alerts/{alert_id}
func (h *AlertHandler) GetAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	alertID := vars["alert_id"]

	alert, err := h.repo.GetByID(ctx, alertID)
	if err != nil {
		logger.Errorf("Failed to get alert: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get alert")
		return
	}
	if alert == nil {
		writeError(w, http.StatusNotFound, "Alert not found")
		return
	}

	writeJSON(w, http.StatusOK, toAlertResponse(alert))
}

// UpdateStatusRequest is the request for updating alert status.
type UpdateStatusRequest struct {
	Status string `json:"status"`
	Notes  string `json:"notes,omitempty"`
}

// UpdateAlertStatus handles PATCH /api/v1/sigma/alerts/{alert_id}/status
func (h *AlertHandler) UpdateAlertStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	alertID := vars["alert_id"]

	// Check if alert exists
	existing, _ := h.repo.GetByID(ctx, alertID)
	if existing == nil {
		writeError(w, http.StatusNotFound, "Alert not found")
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"open":           true,
		"acknowledged":   true,
		"investigating":  true,
		"resolved":       true,
		"false_positive": true,
		"suppressed":     true,
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "Invalid status value")
		return
	}

	if err := h.repo.UpdateStatus(ctx, alertID, req.Status, req.Notes); err != nil {
		logger.Errorf("Failed to update alert status: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to update alert status")
		return
	}

	existing.Status = req.Status
	existing.ResolutionNotes = req.Notes
	writeJSON(w, http.StatusOK, toAlertResponse(existing))
}

// AcknowledgeAlert handles POST /api/v1/sigma/alerts/{alert_id}/acknowledge
func (h *AlertHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	alertID := vars["alert_id"]

	// Check if alert exists
	existing, _ := h.repo.GetByID(ctx, alertID)
	if existing == nil {
		writeError(w, http.StatusNotFound, "Alert not found")
		return
	}

	if err := h.repo.UpdateStatus(ctx, alertID, "acknowledged", ""); err != nil {
		logger.Errorf("Failed to acknowledge alert: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to acknowledge alert")
		return
	}

	existing.Status = "acknowledged"
	writeJSON(w, http.StatusOK, toAlertResponse(existing))
}

// DeleteAlert handles DELETE /api/v1/sigma/alerts/{alert_id}
func (h *AlertHandler) DeleteAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	alertID := vars["alert_id"]

	// Check if alert exists
	existing, _ := h.repo.GetByID(ctx, alertID)
	if existing == nil {
		writeError(w, http.StatusNotFound, "Alert not found")
		return
	}

	if err := h.repo.Delete(ctx, alertID); err != nil {
		logger.Errorf("Failed to delete alert: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete alert")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// toAlertResponse converts a database alert to API response.
func toAlertResponse(alert *database.Alert) *AlertResponse {
	return &AlertResponse{
		ID:                alert.ID,
		Timestamp:         alert.Timestamp,
		AgentID:           alert.AgentID,
		RuleID:            alert.RuleID,
		RuleTitle:         alert.RuleTitle,
		Severity:          alert.Severity,
		Category:          alert.Category,
		EventCount:        alert.EventCount,
		EventIDs:          alert.EventIDs,
		MitreTactics:      alert.MitreTactics,
		MitreTechniques:   alert.MitreTechniques,
		MatchedFields:     alert.MatchedFields,
		ContextData:       alert.ContextData,
		Status:            alert.Status,
		AssignedTo:        alert.AssignedTo,
		ResolutionNotes:   alert.ResolutionNotes,
		Confidence:        alert.Confidence,
		FalsePositiveRisk: alert.FalsePositiveRisk,
		CreatedAt:         alert.CreatedAt,
		UpdatedAt:         alert.UpdatedAt,
	}
}

// BulkUpdateRequest is the request for bulk updating alert status.
type BulkUpdateRequest struct {
	IDs      []string `json:"ids"`
	AlertIDs []string `json:"alert_ids"` // alias used by some frontend payloads
	Status   string   `json:"status"`
}

// BulkUpdateAlertStatus handles PATCH /api/v1/sigma/alerts/bulk/status
func (h *AlertHandler) BulkUpdateAlertStatus(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}

	var req BulkUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Accept either "ids" or "alert_ids"
	ids := req.IDs
	if len(ids) == 0 {
		ids = req.AlertIDs
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "ids or alert_ids is required")
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"open": true, "acknowledged": true, "investigating": true,
		"resolved": true, "false_positive": true, "suppressed": true,
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "Invalid status value")
		return
	}

	if err := h.repo.BulkUpdateStatus(r.Context(), ids, req.Status); err != nil {
		logger.Errorf("Failed to bulk update alert status: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to bulk update alert status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"updated": len(ids),
		"status":  req.Status,
	})
}
