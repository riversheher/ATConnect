package observability

import (
	"encoding/json"
	"net/http"

	"github.com/riversheher/atconnect/pkg/store"
)

// HealthChecker provides HTTP handlers for Kubernetes-style health probes.
//
//   - /livez  — liveness: "is the process alive?"
//   - /readyz — readiness: "can this instance serve traffic?"
//
// These follow the convention established in Kubernetes v1.16+ (which
// deprecated the older /healthz endpoint in favour of /livez and /readyz).
type HealthChecker struct {
	store store.Store
}

// NewHealthChecker creates a HealthChecker that verifies store connectivity
// for the readiness probe.
func NewHealthChecker(s store.Store) *HealthChecker {
	return &HealthChecker{store: s}
}

// healthResponse is the JSON body returned by health endpoints.
type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// ServeLivez returns 200 OK unconditionally. If the HTTP server can respond
// at all, the process is alive.
func (h *HealthChecker) ServeLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

// ServeReadyz checks whether the service is ready to accept traffic.
// Currently this verifies store connectivity via Store.Ping().
//
// Returns 200 if all checks pass, 503 if any check fails.
func (h *HealthChecker) ServeReadyz(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	healthy := true

	// Check store connectivity.
	if err := h.store.Ping(r.Context()); err != nil {
		checks["store"] = "error: " + err.Error()
		healthy = false
	} else {
		checks["store"] = "ok"
	}

	resp := healthResponse{Checks: checks}
	w.Header().Set("Content-Type", "application/json")

	if healthy {
		resp.Status = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		resp.Status = "not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(resp)
}
