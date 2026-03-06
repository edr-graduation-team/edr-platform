// Package metrics provides Prometheus metrics for Sigma Engine.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for Sigma Engine.
type Metrics struct {
	// Counters
	EventsTotal        prometheus.Counter
	AlertsTotal        *prometheus.CounterVec
	AlertsDeduplicated prometheus.Counter
	ErrorsTotal        *prometheus.CounterVec
	RulesMatchedTotal  *prometheus.CounterVec

	// Histograms
	EventProcessingSeconds prometheus.Histogram
	RuleMatchingSeconds    prometheus.Histogram
	DatabaseQuerySeconds   prometheus.Histogram
	KafkaLatencySeconds    prometheus.Histogram

	// Gauges
	EventsInFlight    prometheus.Gauge
	ActiveConnections prometheus.Gauge
	WebSocketClients  prometheus.Gauge
	RulesEnabled      prometheus.Gauge
	AlertsInDatabase  prometheus.Gauge
	KafkaLag          prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "sigma"
	}

	return &Metrics{
		// Counters
		EventsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "events_total",
			Help:      "Total number of events processed",
		}),

		AlertsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "alerts_total",
			Help:      "Total number of alerts created",
		}, []string{"severity", "rule_id"}),

		AlertsDeduplicated: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "alerts_deduplicated_total",
			Help:      "Total number of alerts deduplicated",
		}),

		ErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "errors_total",
			Help:      "Total number of errors",
		}, []string{"type"}),

		RulesMatchedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rules_matched_total",
			Help:      "Total number of rule matches",
		}, []string{"rule_id", "severity"}),

		// Histograms
		EventProcessingSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "event_processing_seconds",
			Help:      "Time spent processing events",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		}),

		RuleMatchingSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "rule_matching_seconds",
			Help:      "Time spent matching rules",
			Buckets:   []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01},
		}),

		DatabaseQuerySeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "database_query_seconds",
			Help:      "Time spent on database queries",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		}),

		KafkaLatencySeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "kafka_latency_seconds",
			Help:      "Kafka round-trip latency",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		}),

		// Gauges
		EventsInFlight: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "events_in_flight",
			Help:      "Number of events currently being processed",
		}),

		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_connections",
			Help:      "Number of active database connections",
		}),

		WebSocketClients: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "websocket_clients",
			Help:      "Number of connected WebSocket clients",
		}),

		RulesEnabled: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "rules_enabled",
			Help:      "Number of enabled Sigma rules",
		}),

		AlertsInDatabase: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "alerts_in_database",
			Help:      "Total alerts in database",
		}),

		KafkaLag: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "kafka_lag",
			Help:      "Kafka consumer lag",
		}),
	}
}

// RecordEvent records an event being processed.
func (m *Metrics) RecordEvent() {
	m.EventsTotal.Inc()
}

// RecordEventProcessing records event processing time.
func (m *Metrics) RecordEventProcessing(seconds float64) {
	m.EventProcessingSeconds.Observe(seconds)
}

// RecordAlert records an alert creation.
func (m *Metrics) RecordAlert(severity, ruleID string) {
	m.AlertsTotal.WithLabelValues(severity, ruleID).Inc()
}

// RecordDeduplication records an alert deduplication.
func (m *Metrics) RecordDeduplication() {
	m.AlertsDeduplicated.Inc()
}

// RecordRuleMatch records a rule match.
func (m *Metrics) RecordRuleMatch(ruleID, severity string) {
	m.RulesMatchedTotal.WithLabelValues(ruleID, severity).Inc()
}

// RecordRuleMatching records rule matching time.
func (m *Metrics) RecordRuleMatching(seconds float64) {
	m.RuleMatchingSeconds.Observe(seconds)
}

// RecordDatabaseQuery records database query time.
func (m *Metrics) RecordDatabaseQuery(seconds float64) {
	m.DatabaseQuerySeconds.Observe(seconds)
}

// RecordKafkaLatency records Kafka latency.
func (m *Metrics) RecordKafkaLatency(seconds float64) {
	m.KafkaLatencySeconds.Observe(seconds)
}

// RecordError records an error.
func (m *Metrics) RecordError(errorType string) {
	m.ErrorsTotal.WithLabelValues(errorType).Inc()
}

// SetEventsInFlight sets the number of events in flight.
func (m *Metrics) SetEventsInFlight(count float64) {
	m.EventsInFlight.Set(count)
}

// SetActiveConnections sets the number of active connections.
func (m *Metrics) SetActiveConnections(count float64) {
	m.ActiveConnections.Set(count)
}

// SetWebSocketClients sets the number of WebSocket clients.
func (m *Metrics) SetWebSocketClients(count float64) {
	m.WebSocketClients.Set(count)
}

// SetRulesEnabled sets the number of enabled rules.
func (m *Metrics) SetRulesEnabled(count float64) {
	m.RulesEnabled.Set(count)
}

// SetAlertsInDatabase sets the total alerts in database.
func (m *Metrics) SetAlertsInDatabase(count float64) {
	m.AlertsInDatabase.Set(count)
}

// SetKafkaLag sets the Kafka consumer lag.
func (m *Metrics) SetKafkaLag(lag float64) {
	m.KafkaLag.Set(lag)
}

// Global metrics instance
var DefaultMetrics = NewMetrics("sigma")
