package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/riversheher/atconnect/internal/middleware"
)

// Metrics holds all Prometheus metrics for the application.
//
// Metrics are grouped by concern:
//   - HTTP: request counts and latency (recorded by the logging middleware)
//   - Recovery: panic counter (recorded by the recovery middleware)
//   - OAuth: ATProto OAuth flow outcomes (recorded by handlers)
//   - OIDC: token issuance counters (recorded by handlers, future Phase 4)
//   - Store: data store operation counters (recorded by store adapters, future)
//
// Use NewMetrics() to register all metrics with Prometheus's default registry.
// The returned Metrics struct is then passed to middleware and handlers that
// need to record values.
type Metrics struct {
	// HTTP metrics — passed to middleware.Logging.
	HTTP *middleware.HTTPMetrics

	// Recovery metrics — passed to middleware.Recovery.
	Recovery *middleware.RecoveryMetrics

	// OAuthFlowsTotal counts ATProto OAuth flow outcomes.
	// Labels: status (started, completed, failed).
	OAuthFlowsTotal *prometheus.CounterVec

	// OIDCTokensIssuedTotal counts OIDC tokens issued.
	// Labels: type (id_token, access_token).
	OIDCTokensIssuedTotal *prometheus.CounterVec

	// StoreOperationsTotal counts store read/write operations.
	// Labels: operation, status (ok, error).
	StoreOperationsTotal *prometheus.CounterVec
}

// NewMetrics registers all application metrics with the default Prometheus
// registry and returns a Metrics struct holding references to them.
//
// promauto is used for registration — it panics on duplicate registration,
// which is the correct behaviour since metrics should only be registered once.
func NewMetrics() *Metrics {
	httpRequestsTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, by method, path, and status.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration := promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency distribution in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	panicsTotal := promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "panics_recovered_total",
			Help: "Total number of panics caught by the recovery middleware.",
		},
	)

	oauthFlowsTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "oauth_flows_total",
			Help: "Total ATProto OAuth flow outcomes, by status.",
		},
		[]string{"status"},
	)

	oidcTokensIssuedTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "oidc_tokens_issued_total",
			Help: "Total OIDC tokens issued, by token type.",
		},
		[]string{"type"},
	)

	storeOperationsTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_operations_total",
			Help: "Total store operations, by operation and status.",
		},
		[]string{"operation", "status"},
	)

	return &Metrics{
		HTTP: &middleware.HTTPMetrics{
			RequestsTotal:   httpRequestsTotal,
			RequestDuration: httpRequestDuration,
		},
		Recovery: &middleware.RecoveryMetrics{
			PanicsTotal: panicsTotal,
		},
		OAuthFlowsTotal:       oauthFlowsTotal,
		OIDCTokensIssuedTotal: oidcTokensIssuedTotal,
		StoreOperationsTotal:  storeOperationsTotal,
	}
}
