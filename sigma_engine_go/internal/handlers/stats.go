// Package handlers provides Statistics API endpoints.
package handlers

import (
	"net/http"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/database"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/gorilla/mux"
)

// PerformanceMetricsProvider abstracts real-time metrics from the event loop.
type PerformanceMetricsProvider interface {
	GetEventsPerSecond() float64
	GetAlertsPerSecond() float64
	GetAverageLatencyMs() float64
	GetProcessingErrors() uint64
	GetEventsProcessed() uint64
}

// StatsHandler handles statistics API endpoints.
type StatsHandler struct {
	alertRepo   database.AlertRepository
	ruleRepo    database.RuleRepository
	perfMetrics PerformanceMetricsProvider
}

// NewStatsHandler creates a new stats handler.
func NewStatsHandler(alertRepo database.AlertRepository, ruleRepo database.RuleRepository) *StatsHandler {
	return &StatsHandler{
		alertRepo: alertRepo,
		ruleRepo:  ruleRepo,
	}
}

// SetPerformanceMetrics injects a real-time metrics provider.
func (h *StatsHandler) SetPerformanceMetrics(provider PerformanceMetricsProvider) {
	h.perfMetrics = provider
}

// RegisterRoutes registers stats routes on the router.
func (h *StatsHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/sigma/stats/alerts", h.AlertStats).Methods("GET")
	r.HandleFunc("/sigma/stats/rules", h.RuleStats).Methods("GET")
	r.HandleFunc("/sigma/stats/performance", h.PerformanceStats).Methods("GET")
	r.HandleFunc("/sigma/stats/timeline", h.TimelineStats).Methods("GET")
}

// AlertStatsResponse is the response for alert statistics.
type AlertStatsResponse struct {
	TotalAlerts   int64            `json:"total_alerts"`
	BySeverity    map[string]int64 `json:"by_severity"`
	ByStatus      map[string]int64 `json:"by_status"`
	ByRule        map[string]int64 `json:"by_rule,omitempty"`
	ByAgent       map[string]int64 `json:"by_agent,omitempty"`
	Alerts24h     int64            `json:"alerts_24h"`
	Alerts7d      int64            `json:"alerts_7d"`
	AvgConfidence float64          `json:"avg_confidence"`
}

// AlertStats handles GET /api/v1/sigma/stats/alerts
func (h *StatsHandler) AlertStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.alertRepo.GetStats(ctx)
	if err != nil {
		logger.Errorf("Failed to get alert stats: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get alert stats")
		return
	}

	response := AlertStatsResponse{
		TotalAlerts:   stats.TotalAlerts,
		BySeverity:    stats.BySeverity,
		ByStatus:      stats.ByStatus,
		ByRule:        stats.ByRule,
		ByAgent:       stats.ByAgent,
		Alerts24h:     stats.Last24Hours,
		Alerts7d:      stats.Last7Days,
		AvgConfidence: stats.AvgConfidence,
	}

	writeJSON(w, http.StatusOK, response)
}

// RuleStatsResponse is the response for rule statistics.
type RuleStatsResponse struct {
	TotalRules    int64            `json:"total_rules"`
	EnabledRules  int64            `json:"enabled_rules"`
	DisabledRules int64            `json:"disabled_rules"`
	BySeverity    map[string]int64 `json:"by_severity"`
	ByProduct     map[string]int64 `json:"by_product"`
	ByCategory    map[string]int64 `json:"by_category"`
	BySource      map[string]int64 `json:"by_source"`
}

// RuleStats handles GET /api/v1/sigma/stats/rules
func (h *StatsHandler) RuleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.ruleRepo.GetStats(ctx)
	if err != nil {
		logger.Errorf("Failed to get rule stats: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get rule stats")
		return
	}

	response := RuleStatsResponse{
		TotalRules:    stats.TotalRules,
		EnabledRules:  stats.EnabledRules,
		DisabledRules: stats.DisabledRules,
		BySeverity:    stats.BySeverity,
		ByProduct:     stats.ByProduct,
		ByCategory:    stats.ByCategory,
		BySource:      stats.BySource,
	}

	writeJSON(w, http.StatusOK, response)
}

// PerformanceStatsResponse is the response for performance statistics.
type PerformanceStatsResponse struct {
	EventsPerSecond    float64 `json:"events_per_second"`
	AlertsPerSecond    float64 `json:"alerts_per_second"`
	AvgEventLatencyMs  float64 `json:"avg_event_latency_ms"`
	AvgRuleMatchingMs  float64 `json:"avg_rule_matching_ms"`
	AvgDatabaseQueryMs float64 `json:"avg_database_query_ms"`
	ActiveConnections  int     `json:"active_connections"`
	WebSocketClients   int     `json:"websocket_clients"`
	KafkaConsumerLag   int64   `json:"kafka_consumer_lag"`
	ErrorRate          float64 `json:"error_rate"`
}

// PerformanceStats handles GET /api/v1/sigma/stats/performance
func (h *StatsHandler) PerformanceStats(w http.ResponseWriter, r *http.Request) {
	var response PerformanceStatsResponse

	if h.perfMetrics != nil {
		processed := h.perfMetrics.GetEventsProcessed()
		errors := h.perfMetrics.GetProcessingErrors()
		var errorRate float64
		if processed > 0 {
			errorRate = float64(errors) / float64(processed)
		}

		response = PerformanceStatsResponse{
			EventsPerSecond:    h.perfMetrics.GetEventsPerSecond(),
			AlertsPerSecond:    h.perfMetrics.GetAlertsPerSecond(),
			AvgEventLatencyMs:  h.perfMetrics.GetAverageLatencyMs(),
			AvgRuleMatchingMs:  h.perfMetrics.GetAverageLatencyMs(), // Same as event latency for now
			AvgDatabaseQueryMs: 0,
			ActiveConnections:  0,
			WebSocketClients:   0,
			KafkaConsumerLag:   0,
			ErrorRate:          errorRate,
		}
	}
	// If no metrics provider, all fields default to zero

	writeJSON(w, http.StatusOK, response)
}

// TimelineStats handles GET /api/v1/sigma/stats/timeline
// Returns timeline data for the alert timeline chart. Currently returns an
// empty array stub to prevent 404 errors and frontend rendering crashes.
func (h *StatsHandler) TimelineStats(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement real timeline aggregation from sigma_alerts table using
	// the "from", "to", and "granularity" query parameters.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": []interface{}{},
	})
}
