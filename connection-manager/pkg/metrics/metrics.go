// Package metrics provides Prometheus metrics for the gRPC server.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics.
type Metrics struct {
	// Request metrics
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	RequestsInFlight prometheus.Gauge

	// Agent metrics
	AgentsOnline     prometheus.Gauge
	AgentsPending    prometheus.Gauge
	AgentsRegistered prometheus.Counter

	// Event metrics
	EventBatchesReceived prometheus.Counter
	EventsReceived       prometheus.Counter
	EventBatchSize       prometheus.Histogram
	EventPayloadBytes    prometheus.Counter

	// Certificate metrics
	CertsIssued  prometheus.Counter
	CertsRenewed prometheus.Counter
	CertsRevoked prometheus.Counter

	// Stream metrics
	ActiveStreams prometheus.Gauge

	// Error metrics
	ErrorsTotal *prometheus.CounterVec

	// Rate limiting metrics
	RateLimitHits prometheus.Counter
}

// NewMetrics creates and registers all metrics.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		// Request metrics
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Total number of gRPC requests",
			},
			[]string{"method", "status"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Duration of gRPC requests",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		RequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "requests_in_flight",
				Help:      "Number of requests currently being processed",
			},
		),

		// Agent metrics
		AgentsOnline: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "agents_online",
				Help:      "Number of currently online agents",
			},
		),
		AgentsPending: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "agents_pending",
				Help:      "Number of agents pending approval",
			},
		),
		AgentsRegistered: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "agents_registered_total",
				Help:      "Total number of agents registered",
			},
		),

		// Event metrics
		EventBatchesReceived: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "event_batches_received_total",
				Help:      "Total number of event batches received",
			},
		),
		EventsReceived: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "events_received_total",
				Help:      "Total number of events received",
			},
		),
		EventBatchSize: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "event_batch_size",
				Help:      "Size of event batches (number of events)",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
			},
		),
		EventPayloadBytes: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "event_payload_bytes_total",
				Help:      "Total bytes of event payloads received",
			},
		),

		// Certificate metrics
		CertsIssued: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificates_issued_total",
				Help:      "Total number of certificates issued",
			},
		),
		CertsRenewed: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificates_renewed_total",
				Help:      "Total number of certificates renewed",
			},
		),
		CertsRevoked: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificates_revoked_total",
				Help:      "Total number of certificates revoked",
			},
		),

		// Stream metrics
		ActiveStreams: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_streams",
				Help:      "Number of active gRPC streams",
			},
		),

		// Error metrics
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total number of errors",
			},
			[]string{"type"},
		),

		// Rate limiting metrics
		RateLimitHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_hits_total",
				Help:      "Total number of rate limit hits",
			},
		),
	}
}

// RecordRequest records a request completion.
func (m *Metrics) RecordRequest(method, status string, durationSeconds float64) {
	m.RequestsTotal.WithLabelValues(method, status).Inc()
	m.RequestDuration.WithLabelValues(method).Observe(durationSeconds)
}

// RecordEventBatch records an event batch.
func (m *Metrics) RecordEventBatch(eventCount int, payloadBytes int) {
	m.EventBatchesReceived.Inc()
	m.EventsReceived.Add(float64(eventCount))
	m.EventBatchSize.Observe(float64(eventCount))
	m.EventPayloadBytes.Add(float64(payloadBytes))
}

// RecordError records an error.
func (m *Metrics) RecordError(errorType string) {
	m.ErrorsTotal.WithLabelValues(errorType).Inc()
}
