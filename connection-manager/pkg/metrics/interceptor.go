// Package metrics provides gRPC interceptor for metrics collection.
package metrics

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// MetricsInterceptor provides gRPC interceptors for metrics collection.
type MetricsInterceptor struct {
	metrics *Metrics
}

// NewMetricsInterceptor creates a new metrics interceptor.
func NewMetricsInterceptor(m *Metrics) *MetricsInterceptor {
	return &MetricsInterceptor{metrics: m}
}

// UnaryInterceptor collects metrics for unary RPCs.
func (mi *MetricsInterceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	mi.metrics.RequestsInFlight.Inc()
	defer mi.metrics.RequestsInFlight.Dec()

	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start).Seconds()

	statusCode := "OK"
	if err != nil {
		if st, ok := status.FromError(err); ok {
			statusCode = st.Code().String()
		} else {
			statusCode = "UNKNOWN"
		}
		mi.metrics.RecordError(statusCode)
	}

	mi.metrics.RecordRequest(info.FullMethod, statusCode, duration)

	return resp, err
}

// StreamInterceptor collects metrics for streaming RPCs.
func (mi *MetricsInterceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	mi.metrics.ActiveStreams.Inc()
	defer mi.metrics.ActiveStreams.Dec()

	start := time.Now()
	err := handler(srv, ss)
	duration := time.Since(start).Seconds()

	statusCode := "OK"
	if err != nil {
		if st, ok := status.FromError(err); ok {
			statusCode = st.Code().String()
		} else {
			statusCode = "UNKNOWN"
		}
		mi.metrics.RecordError(statusCode)
	}

	mi.metrics.RecordRequest(info.FullMethod, statusCode, duration)

	return err
}
