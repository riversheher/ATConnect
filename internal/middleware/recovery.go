package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// recoveryHandler is the HTTP handler produced by the Recovery middleware.
type recoveryHandler struct {
	next    http.Handler
	metrics *RecoveryMetrics
}

// ServeHTTP delegates to the next handler and recovers from any panics.
//
// This MUST be the outermost middleware in the chain so that panics from any
// layer (including other middleware) are caught.
func (h *recoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer h.handlePanic(w, r)
	h.next.ServeHTTP(w, r)
}

// handlePanic is the deferred recovery function. If a panic occurred it logs
// the panic value and stack trace, increments the Prometheus counter, and
// writes a generic 500 JSON response.
//
// Panic details are never leaked to the client — they appear only in server logs.
func (h *recoveryHandler) handlePanic(w http.ResponseWriter, r *http.Request) {
	err := recover()
	if err == nil {
		return
	}

	stack := string(debug.Stack())

	slog.Error("panic recovered",
		"error", err,
		"stack", stack,
		"request_id", RequestIDFromContext(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
	)

	// Increment Prometheus counter if available.
	if h.metrics != nil {
		h.metrics.PanicsTotal.Inc()
	}

	// Write a generic error response. If headers have already been
	// sent (unlikely after a panic), this will be a no-op — the
	// client will see an incomplete response, which is the best we
	// can do.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "internal_error",
		"message": "An internal error occurred",
	})
}

// Recovery is middleware that catches panics from downstream handlers, logs
// the panic value and stack trace, increments a Prometheus counter, and
// returns a generic 500 response.
//
// The response body is a JSON object matching our error format:
//
//	{"error": "internal_error", "message": "An internal error occurred"}
func Recovery(metrics *RecoveryMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &recoveryHandler{next: next, metrics: metrics}
	}
}
