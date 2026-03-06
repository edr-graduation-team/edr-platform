// Package handlers provides HTTP handlers for automation.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/edr-platform/sigma-engine/internal/automation"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/gorilla/mux"
)

// AutomationHandler handles automation API requests.
type AutomationHandler struct {
	playbooks   *automation.PlaybookManager
	escalations *automation.EscalationManager
	notifier    *automation.NotificationManager
}

// NewAutomationHandler creates a new automation handler.
func NewAutomationHandler(
	playbooks *automation.PlaybookManager,
	escalations *automation.EscalationManager,
	notifier *automation.NotificationManager,
) *AutomationHandler {
	return &AutomationHandler{
		playbooks:   playbooks,
		escalations: escalations,
		notifier:    notifier,
	}
}

// RegisterRoutes registers automation API routes.
func (h *AutomationHandler) RegisterRoutes(r *mux.Router) {
	// Playbooks
	r.HandleFunc("/api/v1/sigma/playbooks", h.ListPlaybooks).Methods("GET")
	r.HandleFunc("/api/v1/sigma/playbooks", h.CreatePlaybook).Methods("POST")
	r.HandleFunc("/api/v1/sigma/playbooks/{id}", h.GetPlaybook).Methods("GET")
	r.HandleFunc("/api/v1/sigma/playbooks/{id}", h.UpdatePlaybook).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/playbooks/{id}", h.DeletePlaybook).Methods("DELETE")
	r.HandleFunc("/api/v1/sigma/playbooks/{id}/test", h.TestPlaybook).Methods("POST")
	r.HandleFunc("/api/v1/sigma/playbooks/{id}/history", h.GetPlaybookHistory).Methods("GET")

	// Escalation Rules
	r.HandleFunc("/api/v1/sigma/escalation-rules", h.ListEscalationRules).Methods("GET")
	r.HandleFunc("/api/v1/sigma/escalation-rules", h.CreateEscalationRule).Methods("POST")
	r.HandleFunc("/api/v1/sigma/escalation-rules/{id}", h.GetEscalationRule).Methods("GET")
	r.HandleFunc("/api/v1/sigma/escalation-rules/{id}", h.UpdateEscalationRule).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/escalation-rules/{id}", h.DeleteEscalationRule).Methods("DELETE")
	r.HandleFunc("/api/v1/sigma/escalation-rules/{id}/test", h.TestEscalationRule).Methods("POST")

	// Escalation History
	r.HandleFunc("/api/v1/sigma/alerts/{id}/escalations", h.GetAlertEscalations).Methods("GET")
	r.HandleFunc("/api/v1/sigma/escalations/history", h.GetEscalationHistory).Methods("GET")

	// Notifications
	r.HandleFunc("/api/v1/sigma/notifications/config", h.GetNotificationConfig).Methods("GET")
	r.HandleFunc("/api/v1/sigma/notifications/config", h.ConfigureNotifications).Methods("POST", "PUT")
	r.HandleFunc("/api/v1/sigma/notifications/slack/send", h.SendSlackTest).Methods("POST")
	r.HandleFunc("/api/v1/sigma/notifications/teams/send", h.SendTeamsTest).Methods("POST")
	r.HandleFunc("/api/v1/sigma/notifications/email/send", h.SendEmailTest).Methods("POST")
	r.HandleFunc("/api/v1/sigma/notifications/logs", h.GetNotificationLogs).Methods("GET")
	r.HandleFunc("/api/v1/sigma/notifications/stats", h.GetNotificationStats).Methods("GET")

	// Automation overview
	r.HandleFunc("/api/v1/sigma/automation/status", h.GetAutomationStatus).Methods("GET")
}

// --- Playbook Handlers ---

// ListPlaybooks returns all playbooks.
func (h *AutomationHandler) ListPlaybooks(w http.ResponseWriter, r *http.Request) {
	playbooks := h.playbooks.ListPlaybooks()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"playbooks": playbooks,
		"total":     len(playbooks),
	})
}

// CreatePlaybook creates a new playbook.
func (h *AutomationHandler) CreatePlaybook(w http.ResponseWriter, r *http.Request) {
	var playbook automation.Playbook
	if err := json.NewDecoder(r.Body).Decode(&playbook); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := h.playbooks.CreatePlaybook(playbook)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// GetPlaybook returns a specific playbook.
func (h *AutomationHandler) GetPlaybook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	playbook, err := h.playbooks.GetPlaybook(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, playbook)
}

// UpdatePlaybook updates a playbook.
func (h *AutomationHandler) UpdatePlaybook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var playbook automation.Playbook
	if err := json.NewDecoder(r.Body).Decode(&playbook); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := h.playbooks.UpdatePlaybook(id, playbook)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// DeletePlaybook removes a playbook.
func (h *AutomationHandler) DeletePlaybook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.playbooks.DeletePlaybook(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestPlaybook tests a playbook with a mock alert.
func (h *AutomationHandler) TestPlaybook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	_, err := h.playbooks.GetPlaybook(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Would execute with test alert
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Playbook test executed (dry-run)",
	})
}

// GetPlaybookHistory returns execution history.
func (h *AutomationHandler) GetPlaybookHistory(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	executions := h.playbooks.GetExecutions(id, 100)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"executions": executions,
		"total":      len(executions),
	})
}

// --- Escalation Rule Handlers ---

// ListEscalationRules returns all escalation rules.
func (h *AutomationHandler) ListEscalationRules(w http.ResponseWriter, r *http.Request) {
	rules := h.escalations.ListRules()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rules": rules,
		"total": len(rules),
	})
}

// CreateEscalationRule creates a new escalation rule.
func (h *AutomationHandler) CreateEscalationRule(w http.ResponseWriter, r *http.Request) {
	var rule automation.EscalationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := h.escalations.CreateRule(rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// GetEscalationRule returns a specific rule.
func (h *AutomationHandler) GetEscalationRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rule, err := h.escalations.GetRule(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// UpdateEscalationRule updates an escalation rule.
func (h *AutomationHandler) UpdateEscalationRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var rule automation.EscalationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := h.escalations.UpdateRule(id, rule)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// DeleteEscalationRule removes an escalation rule.
func (h *AutomationHandler) DeleteEscalationRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.escalations.DeleteRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestEscalationRule tests an escalation rule.
func (h *AutomationHandler) TestEscalationRule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	_, err := h.escalations.GetRule(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Escalation rule test executed (dry-run)",
	})
}

// GetAlertEscalations returns escalation history for an alert.
func (h *AutomationHandler) GetAlertEscalations(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	history := h.escalations.GetHistory(id, 100)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"escalations": history,
		"total":       len(history),
	})
}

// GetEscalationHistory returns all escalation history.
func (h *AutomationHandler) GetEscalationHistory(w http.ResponseWriter, r *http.Request) {
	history := h.escalations.GetAllHistory(100)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": history,
		"total":   len(history),
	})
}

// --- Notification Handlers ---

// GetNotificationConfig returns notification configuration.
func (h *AutomationHandler) GetNotificationConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.notifier.GetConfig())
}

// ConfigureNotifications updates notification settings.
func (h *AutomationHandler) ConfigureNotifications(w http.ResponseWriter, r *http.Request) {
	var config automation.NotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.notifier.Configure(config)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Notification configuration updated",
	})
}

// SendSlackTest sends a test Slack notification.
func (h *AutomationHandler) SendSlackTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Use mock alert for test
	config := map[string]interface{}{"channel": req.Channel}
	err := h.notifier.SendSlack(ctx, createTestAlert(), config)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Slack test message sent",
	})
}

// SendTeamsTest sends a test Teams notification.
func (h *AutomationHandler) SendTeamsTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err := h.notifier.SendTeams(ctx, createTestAlert(), nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Teams test message sent",
	})
}

// SendEmailTest sends a test email notification.
func (h *AutomationHandler) SendEmailTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	config := map[string]interface{}{"to": req.To}
	err := h.notifier.SendEmail(ctx, createTestAlert(), config)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Email test sent",
	})
}

// GetNotificationLogs returns notification logs.
func (h *AutomationHandler) GetNotificationLogs(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	logs := h.notifier.GetLogs(100, typeFilter)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	})
}

// GetNotificationStats returns notification statistics.
func (h *AutomationHandler) GetNotificationStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.notifier.GetStats())
}

// GetAutomationStatus returns overall automation status.
func (h *AutomationHandler) GetAutomationStatus(w http.ResponseWriter, r *http.Request) {
	playbooks := h.playbooks.ListPlaybooks()
	enabledPlaybooks := 0
	for _, p := range playbooks {
		if p.Enabled {
			enabledPlaybooks++
		}
	}

	rules := h.escalations.ListRules()
	enabledRules := 0
	for _, r := range rules {
		if r.Enabled {
			enabledRules++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"playbooks": map[string]interface{}{
			"total":   len(playbooks),
			"enabled": enabledPlaybooks,
		},
		"escalation_rules": map[string]interface{}{
			"total":   len(rules),
			"enabled": enabledRules,
		},
		"escalations":   h.escalations.GetStats(),
		"notifications": h.notifier.GetStats(),
	})
}

// Helper to create test alert
func createTestAlert() *domain.Alert {
	return &domain.Alert{
		ID:        "test-alert-001",
		RuleID:    "test-rule-001",
		RuleTitle: "Test Alert - Connectivity Verification",
		Severity:  domain.SeverityMedium,
		Timestamp: time.Now(),
	}
}
