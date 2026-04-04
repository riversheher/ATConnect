package middleware

import "github.com/prometheus/client_golang/prometheus"

// HTTPMetrics holds the Prometheus collectors used by the Logging middleware.
// These are defined here (rather than in internal/observability) to avoid
// circular imports — middleware should not depend on observability, since
// observability may depend on middleware types in the future.
//
// The metrics are registered and owned by internal/observability/metrics;
// this struct simply carries references so the logging middleware can
// record values.
type HTTPMetrics struct {
	// RequestsTotal counts HTTP requests, labelled by method, path, and status.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration observes request latency in seconds, labelled by method and path.
	RequestDuration *prometheus.HistogramVec
}

// RecoveryMetrics holds the Prometheus collectors used by the Recovery middleware.
type RecoveryMetrics struct {
	// PanicsTotal counts the number of panics caught by the recovery middleware.
	PanicsTotal prometheus.Counter
}
