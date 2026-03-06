// Package handlers provides HTTP handlers for integrations.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/edr-platform/sigma-engine/internal/integrations"
	"github.com/gorilla/mux"
)

// IntegrationHandler handles integration API requests.
type IntegrationHandler struct {
	webhooks   *integrations.WebhookManager
	splunk     *integrations.SplunkIntegration
	servicenow *integrations.ServiceNowIntegration
}

// NewIntegrationHandler creates a new integration handler.
func NewIntegrationHandler(
	webhooks *integrations.WebhookManager,
	splunk *integrations.SplunkIntegration,
	servicenow *integrations.ServiceNowIntegration,
) *IntegrationHandler {
	return &IntegrationHandler{
		webhooks:   webhooks,
		splunk:     splunk,
		servicenow: servicenow,
	}
}

// RegisterRoutes registers integration API routes.
func (h *IntegrationHandler) RegisterRoutes(r *mux.Router) {
	// Webhooks
	r.HandleFunc("/api/v1/sigma/webhooks", h.ListWebhooks).Methods("GET")
	r.HandleFunc("/api/v1/sigma/webhooks", h.CreateWebhook).Methods("POST")
	r.HandleFunc("/api/v1/sigma/webhooks/{id}", h.GetWebhook).Methods("GET")
	r.HandleFunc("/api/v1/sigma/webhooks/{id}", h.UpdateWebhook).Methods("PUT")
	r.HandleFunc("/api/v1/sigma/webhooks/{id}", h.DeleteWebhook).Methods("DELETE")
	r.HandleFunc("/api/v1/sigma/webhooks/{id}/test", h.TestWebhook).Methods("POST")
	r.HandleFunc("/api/v1/sigma/webhooks/{id}/logs", h.GetWebhookLogs).Methods("GET")

	// Splunk
	r.HandleFunc("/api/v1/sigma/integrations/splunk/config", h.GetSplunkConfig).Methods("GET")
	r.HandleFunc("/api/v1/sigma/integrations/splunk/config", h.ConfigureSplunk).Methods("POST", "PUT")
	r.HandleFunc("/api/v1/sigma/integrations/splunk/test", h.TestSplunk).Methods("POST")
	r.HandleFunc("/api/v1/sigma/integrations/splunk/status", h.GetSplunkStatus).Methods("GET")

	// ServiceNow
	r.HandleFunc("/api/v1/sigma/integrations/servicenow/config", h.GetServiceNowConfig).Methods("GET")
	r.HandleFunc("/api/v1/sigma/integrations/servicenow/config", h.ConfigureServiceNow).Methods("POST", "PUT")
	r.HandleFunc("/api/v1/sigma/integrations/servicenow/test", h.TestServiceNow).Methods("POST")
	r.HandleFunc("/api/v1/sigma/integrations/servicenow/status", h.GetServiceNowStatus).Methods("GET")
	r.HandleFunc("/api/v1/sigma/integrations/servicenow/mappings", h.ListServiceNowMappings).Methods("GET")

	// Integration overview
	r.HandleFunc("/api/v1/sigma/integrations/status", h.GetIntegrationsStatus).Methods("GET")
}

// --- Webhook Handlers ---

// ListWebhooks returns all webhooks.
func (h *IntegrationHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks := h.webhooks.ListWebhooks()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooks,
		"total":    len(webhooks),
	})
}

// CreateWebhook creates a new webhook.
func (h *IntegrationHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var config integrations.WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	webhook, err := h.webhooks.CreateWebhook(config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, webhook)
}

// GetWebhook returns a specific webhook.
func (h *IntegrationHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	webhook, err := h.webhooks.GetWebhook(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, webhook)
}

// UpdateWebhook updates a webhook.
func (h *IntegrationHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var config integrations.WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	webhook, err := h.webhooks.UpdateWebhook(id, config)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, webhook)
}

// DeleteWebhook removes a webhook.
func (h *IntegrationHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.webhooks.DeleteWebhook(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestWebhook tests webhook connectivity.
func (h *IntegrationHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	webhook, err := h.webhooks.GetWebhook(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.webhooks.TestWebhook(ctx, webhook); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Webhook test delivered successfully",
	})
}

// GetWebhookLogs returns delivery logs for a webhook.
func (h *IntegrationHandler) GetWebhookLogs(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	logs := h.webhooks.GetLogs(id, 100)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	})
}

// --- Splunk Handlers ---

// GetSplunkConfig returns Splunk configuration.
func (h *IntegrationHandler) GetSplunkConfig(w http.ResponseWriter, r *http.Request) {
	config := h.splunk.GetConfig()
	// Mask token
	if config.HECToken != "" {
		config.HECToken = "********"
	}
	writeJSON(w, http.StatusOK, config)
}

// ConfigureSplunk updates Splunk configuration.
func (h *IntegrationHandler) ConfigureSplunk(w http.ResponseWriter, r *http.Request) {
	var config integrations.SplunkConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.splunk.Configure(config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Splunk configuration updated",
	})
}

// TestSplunk tests Splunk connectivity.
func (h *IntegrationHandler) TestSplunk(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.splunk.TestConnection(ctx); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Splunk HEC connection successful",
	})
}

// GetSplunkStatus returns Splunk integration status.
func (h *IntegrationHandler) GetSplunkStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.splunk.GetStatus())
}

// --- ServiceNow Handlers ---

// GetServiceNowConfig returns ServiceNow configuration.
func (h *IntegrationHandler) GetServiceNowConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.servicenow.GetConfig())
}

// ConfigureServiceNow updates ServiceNow configuration.
func (h *IntegrationHandler) ConfigureServiceNow(w http.ResponseWriter, r *http.Request) {
	var config integrations.ServiceNowConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.servicenow.Configure(config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "ServiceNow configuration updated",
	})
}

// TestServiceNow tests ServiceNow connectivity.
func (h *IntegrationHandler) TestServiceNow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.servicenow.TestConnection(ctx); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "ServiceNow connection successful",
	})
}

// GetServiceNowStatus returns ServiceNow integration status.
func (h *IntegrationHandler) GetServiceNowStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.servicenow.GetStatus())
}

// ListServiceNowMappings returns all alert-incident mappings.
func (h *IntegrationHandler) ListServiceNowMappings(w http.ResponseWriter, r *http.Request) {
	mappings := h.servicenow.ListMappings()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
		"total":    len(mappings),
	})
}

// --- Overview Handler ---

// GetIntegrationsStatus returns status of all integrations.
func (h *IntegrationHandler) GetIntegrationsStatus(w http.ResponseWriter, r *http.Request) {
	webhooks := h.webhooks.ListWebhooks()
	enabledWebhooks := 0
	for _, wh := range webhooks {
		if wh.Enabled {
			enabledWebhooks++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": map[string]interface{}{
			"total":   len(webhooks),
			"enabled": enabledWebhooks,
		},
		"splunk":     h.splunk.GetStatus(),
		"servicenow": h.servicenow.GetStatus(),
	})
}
