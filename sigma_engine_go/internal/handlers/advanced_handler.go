// Package handlers provides HTTP handlers for advanced features.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/edr-platform/sigma-engine/internal/analytics"
	"github.com/edr-platform/sigma-engine/internal/ml"
	"github.com/edr-platform/sigma-engine/internal/rules"
	"github.com/edr-platform/sigma-engine/internal/user"
	"github.com/gorilla/mux"
)

// AdvancedHandler handles advanced feature API requests.
type AdvancedHandler struct {
	baseline    *ml.BaselineManager
	customRules *rules.CustomRuleManager
	profiles    *user.ProfileManager
	correlation *analytics.CorrelationManager
}

// NewAdvancedHandler creates a new advanced handler.
func NewAdvancedHandler(
	baseline *ml.BaselineManager,
	customRules *rules.CustomRuleManager,
	profiles *user.ProfileManager,
	correlation *analytics.CorrelationManager,
) *AdvancedHandler {
	return &AdvancedHandler{
		baseline:    baseline,
		customRules: customRules,
		profiles:    profiles,
		correlation: correlation,
	}
}

// RegisterRoutes registers advanced feature routes.
func (h *AdvancedHandler) RegisterRoutes(r *mux.Router) {
	// ML / Baselines
	r.HandleFunc("/api/v1/sigma/ml/status", h.GetMLStatus).Methods("GET")
	r.HandleFunc("/api/v1/sigma/ml/baselines", h.ListBaselines).Methods("GET")
	r.HandleFunc("/api/v1/sigma/agents/{id}/baseline", h.GetAgentBaseline).Methods("GET")
	r.HandleFunc("/api/v1/sigma/agents/{id}/baseline/learn", h.LearnBaseline).Methods("POST")
	r.HandleFunc("/api/v1/sigma/alerts/{id}/ml-score", h.GetMLScore).Methods("GET")

	// Custom Rules
	r.HandleFunc("/api/v1/sigma/rules/custom", h.ListCustomRules).Methods("GET")
	r.HandleFunc("/api/v1/sigma/rules/custom", h.CreateCustomRule).Methods("POST")
	r.HandleFunc("/api/v1/sigma/rules/custom/{id}", h.GetCustomRule).Methods("GET")
	r.HandleFunc("/api/v1/sigma/rules/custom/{id}", h.UpdateCustomRule).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/rules/custom/{id}", h.DeleteCustomRule).Methods("DELETE")
	r.HandleFunc("/api/v1/sigma/rules/custom/{id}/test", h.TestCustomRule).Methods("POST")
	r.HandleFunc("/api/v1/sigma/rules/custom/{id}/metrics", h.GetCustomRuleMetrics).Methods("GET")

	// User Profiles
	r.HandleFunc("/api/v1/sigma/users/{id}/profiles", h.ListUserProfiles).Methods("GET")
	r.HandleFunc("/api/v1/sigma/users/{id}/profiles", h.CreateUserProfile).Methods("POST")
	r.HandleFunc("/api/v1/sigma/profiles/{id}", h.GetProfile).Methods("GET")
	r.HandleFunc("/api/v1/sigma/profiles/{id}", h.UpdateProfile).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/profiles/{id}", h.DeleteProfile).Methods("DELETE")
	r.HandleFunc("/api/v1/sigma/users/{id}/alerts", h.GetUserAlerts).Methods("GET")

	// Correlation / Incidents
	r.HandleFunc("/api/v1/sigma/alerts/correlation", h.GetCorrelations).Methods("GET")
	r.HandleFunc("/api/v1/sigma/incidents", h.ListIncidents).Methods("GET")
	r.HandleFunc("/api/v1/sigma/incidents", h.CreateIncident).Methods("POST")
	r.HandleFunc("/api/v1/sigma/incidents/{id}", h.GetIncident).Methods("GET")
	r.HandleFunc("/api/v1/sigma/incidents/{id}", h.UpdateIncident).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/incidents/{id}/notes", h.AddIncidentNote).Methods("POST")

	// Analytics
	r.HandleFunc("/api/v1/sigma/analytics/summary", h.GetAnalyticsSummary).Methods("GET")
}

// --- ML Handlers ---

func (h *AdvancedHandler) GetMLStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.baseline.GetMLStatus())
}

func (h *AdvancedHandler) ListBaselines(w http.ResponseWriter, r *http.Request) {
	baselines := h.baseline.ListBaselines()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"baselines": baselines,
		"total":     len(baselines),
	})
}

func (h *AdvancedHandler) GetAgentBaseline(w http.ResponseWriter, r *http.Request) {
	agentID := mux.Vars(r)["id"]
	bType := r.URL.Query().Get("type")
	if bType == "" {
		bType = "process"
	}

	baseline, err := h.baseline.GetBaseline(agentID, ml.BaselineType(bType))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, baseline)
}

func (h *AdvancedHandler) LearnBaseline(w http.ResponseWriter, r *http.Request) {
	agentID := mux.Vars(r)["id"]

	var req struct {
		Type   string                   `json:"type"`
		Events []map[string]interface{} `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	bType := ml.BaselineProcess
	if req.Type != "" {
		bType = ml.BaselineType(req.Type)
	}

	baseline, err := h.baseline.LearnBaseline(agentID, bType, req.Events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, baseline)
}

func (h *AdvancedHandler) GetMLScore(w http.ResponseWriter, r *http.Request) {
	alertID := mux.Vars(r)["id"]
	scores := h.baseline.GetAnomalyScores(alertID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alert_id": alertID,
		"scores":   scores,
	})
}

// --- Custom Rule Handlers ---

func (h *AdvancedHandler) ListCustomRules(w http.ResponseWriter, r *http.Request) {
	rules := h.customRules.ListRules()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rules": rules,
		"total": len(rules),
	})
}

func (h *AdvancedHandler) CreateCustomRule(w http.ResponseWriter, r *http.Request) {
	var rule rules.CustomRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	result, err := h.customRules.CreateRule(rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *AdvancedHandler) GetCustomRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rule, err := h.customRules.GetRule(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *AdvancedHandler) UpdateCustomRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var rule rules.CustomRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	result, err := h.customRules.UpdateRule(id, rule)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *AdvancedHandler) DeleteCustomRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.customRules.DeleteRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdvancedHandler) TestCustomRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rule, err := h.customRules.GetRule(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var alertData map[string]interface{}
	json.NewDecoder(r.Body).Decode(&alertData)

	matched := h.customRules.TestRule(rule, alertData)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"matched": matched,
		"rule_id": id,
	})
}

func (h *AdvancedHandler) GetCustomRuleMetrics(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	metrics, err := h.customRules.GetRuleMetrics(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// --- Profile Handlers ---

func (h *AdvancedHandler) ListUserProfiles(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["id"]
	profiles := h.profiles.GetUserProfiles(userID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"profiles": profiles,
		"total":    len(profiles),
	})
}

func (h *AdvancedHandler) CreateUserProfile(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["id"]
	var profile user.AlertProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	profile.UserID = userID

	result, err := h.profiles.CreateProfile(profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *AdvancedHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	profile, err := h.profiles.GetProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *AdvancedHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var profile user.AlertProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	result, err := h.profiles.UpdateProfile(id, profile)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *AdvancedHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.profiles.DeleteProfile(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdvancedHandler) GetUserAlerts(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["id"]
	alerts := h.profiles.GetUserAlerts(userID, 100)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alerts": alerts,
		"total":  len(alerts),
	})
}

// --- Correlation Handlers ---

func (h *AdvancedHandler) GetCorrelations(w http.ResponseWriter, r *http.Request) {
	alertID := r.URL.Query().Get("alert_id")
	if alertID == "" {
		writeJSON(w, http.StatusOK, h.correlation.GetAnalytics())
		return
	}

	correlations := h.correlation.GetAlertCorrelations(alertID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alert_id":     alertID,
		"correlations": correlations,
		"total":        len(correlations),
	})
}

func (h *AdvancedHandler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	incidents := h.correlation.ListIncidents(status)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"incidents": incidents,
		"total":     len(incidents),
	})
}

func (h *AdvancedHandler) CreateIncident(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlertIDs []string `json:"alert_ids"`
		Title    string   `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	incident, err := h.correlation.CreateIncident(req.AlertIDs, req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, incident)
}

func (h *AdvancedHandler) GetIncident(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	incident, err := h.correlation.GetIncident(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (h *AdvancedHandler) UpdateIncident(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	incident, err := h.correlation.UpdateIncident(id, req.Status)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, incident)
}

func (h *AdvancedHandler) AddIncidentNote(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		AuthorID string `json:"author_id"`
		Content  string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.correlation.AddNote(id, req.AuthorID, req.Content); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Note added"})
}

func (h *AdvancedHandler) GetAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	summary := map[string]interface{}{
		"ml":          h.baseline.GetMLStatus(),
		"correlation": h.correlation.GetAnalytics(),
		"profiles":    h.profiles.GetProfileStats(),
	}
	writeJSON(w, http.StatusOK, summary)
}
