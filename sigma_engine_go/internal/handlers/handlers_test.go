// Package handlers provides unit tests for API handlers.
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]string
	json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, "healthy", response["status"])
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	data := map[string]string{"key": "value"}
	writeJSON(rr, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]string
	json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, "value", response["key"])
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()

	writeError(rr, http.StatusBadRequest, "test error")

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response map[string]string
	json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, "test error", response["error"])
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	assert.Equal(t, ":8080", cfg.Address)
	assert.Greater(t, cfg.ReadTimeout.Seconds(), float64(0))
	assert.Greater(t, cfg.WriteTimeout.Seconds(), float64(0))
}

func TestRuleResponseStruct(t *testing.T) {
	response := RuleResponse{
		ID:       "rule-123",
		Title:    "Test Rule",
		Severity: "high",
		Enabled:  true,
		Status:   "stable",
	}

	assert.Equal(t, "rule-123", response.ID)
	assert.Equal(t, "high", response.Severity)
	assert.True(t, response.Enabled)
}

func TestAlertResponseStruct(t *testing.T) {
	confidence := 0.95
	response := AlertResponse{
		ID:         "alert-123",
		RuleID:     "rule-456",
		Severity:   "critical",
		Status:     "open",
		EventCount: 3,
		Confidence: &confidence,
	}

	assert.Equal(t, "alert-123", response.ID)
	assert.Equal(t, "critical", response.Severity)
	assert.Equal(t, 3, response.EventCount)
	assert.Equal(t, 0.95, *response.Confidence)
}

func TestRulesListResponse(t *testing.T) {
	response := RulesListResponse{
		Count:  10,
		Total:  100,
		Limit:  50,
		Offset: 0,
		Rules:  make([]*RuleResponse, 0),
	}

	assert.Equal(t, 10, response.Count)
	assert.Equal(t, int64(100), response.Total)
	assert.Equal(t, 50, response.Limit)
}

func TestAlertsListResponse(t *testing.T) {
	response := AlertsListResponse{
		Count:  5,
		Total:  25,
		Limit:  10,
		Offset: 0,
		Alerts: make([]*AlertResponse, 0),
	}

	assert.Equal(t, 5, response.Count)
	assert.Equal(t, int64(25), response.Total)
}

func TestCreateRuleRequest(t *testing.T) {
	reqBody := `{"id":"rule-1","title":"Test","content":"test content","severity":"high"}`

	var req CreateRuleRequest
	err := json.Unmarshal([]byte(reqBody), &req)

	assert.NoError(t, err)
	assert.Equal(t, "rule-1", req.ID)
	assert.Equal(t, "Test", req.Title)
	assert.Equal(t, "high", req.Severity)
}

func TestUpdateStatusRequest(t *testing.T) {
	reqBody := `{"status":"acknowledged","notes":"Investigating"}`

	var req UpdateStatusRequest
	err := json.Unmarshal([]byte(reqBody), &req)

	assert.NoError(t, err)
	assert.Equal(t, "acknowledged", req.Status)
	assert.Equal(t, "Investigating", req.Notes)
}

func TestBulkImportRequest(t *testing.T) {
	reqBody := `{"rules":[{"id":"r1","title":"Rule 1","content":"c1"},{"id":"r2","title":"Rule 2","content":"c2"}]}`

	var req BulkImportRequest
	err := json.Unmarshal([]byte(reqBody), &req)

	assert.NoError(t, err)
	assert.Len(t, req.Rules, 2)
	assert.Equal(t, "r1", req.Rules[0].ID)
}

func TestBulkImportResponse(t *testing.T) {
	response := BulkImportResponse{
		Imported: 5,
		Failed:   1,
		Errors:   []string{"rule-x: duplicate ID"},
	}

	assert.Equal(t, 5, response.Imported)
	assert.Equal(t, 1, response.Failed)
	assert.Len(t, response.Errors, 1)
}

func TestAlertStatsResponse(t *testing.T) {
	response := AlertStatsResponse{
		TotalAlerts: 1000,
		BySeverity:  map[string]int64{"high": 100, "medium": 500},
		ByStatus:    map[string]int64{"open": 600, "closed": 400},
		Alerts24h:   50,
		Alerts7d:    200,
	}

	assert.Equal(t, int64(1000), response.TotalAlerts)
	assert.Equal(t, int64(100), response.BySeverity["high"])
	assert.Equal(t, int64(50), response.Alerts24h)
}

func TestRuleStatsResponse(t *testing.T) {
	response := RuleStatsResponse{
		TotalRules:    4367,
		EnabledRules:  4200,
		DisabledRules: 167,
		BySeverity:    map[string]int64{"high": 1000},
	}

	assert.Equal(t, int64(4367), response.TotalRules)
	assert.Equal(t, int64(4200), response.EnabledRules)
}

func TestAlertStreamFilters(t *testing.T) {
	filters := AlertStreamFilters{
		Severity: []string{"critical", "high"},
		RuleID:   "sigma-123",
		AgentID:  "agent-456",
	}

	assert.Len(t, filters.Severity, 2)
	assert.Equal(t, "sigma-123", filters.RuleID)
}

func TestWebSocketMessage(t *testing.T) {
	msg := WebSocketMessage{
		Type: "ping",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err)

	var parsed WebSocketMessage
	json.Unmarshal(data, &parsed)
	assert.Equal(t, "ping", parsed.Type)
}

func TestCORSMiddleware(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestLoggingMiddleware(t *testing.T) {
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestNewWebSocketServer(t *testing.T) {
	server := NewWebSocketServer()

	assert.NotNil(t, server)
	assert.NotNil(t, server.clients)
	assert.NotNil(t, server.broadcast)
	assert.Equal(t, 0, server.ClientCount())
}

func TestMarshalJSON(t *testing.T) {
	buf := new(bytes.Buffer)

	response := map[string]interface{}{
		"test": "value",
		"num":  123,
	}

	encoder := json.NewEncoder(buf)
	err := encoder.Encode(response)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "test")
}
