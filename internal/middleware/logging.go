package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code and
// bytes written. This allows the logging middleware to report these values
// after the handler has completed.
//
// Limitation: this wrapper does not implement optional interfaces such as
// http.Flusher or http.Hijacker. For an OIDC provider serving JSON responses
// this is acceptable. See logging_plan.md §5 for discussion.
type responseWriter struct {
	http.ResponseWriter
	status       int
	wroteHeader  bool
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.status = http.StatusOK
		rw.wroteHeader = true
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// loggingHandler is the HTTP handler produced by the Logging middleware.
type loggingHandler struct {
	next    http.Handler
	metrics *HTTPMetrics
}

// ServeHTTP wraps the downstream handler to capture status code, response
// size, and duration. After the handler completes it emits a structured log
// entry and records Prometheus metrics (if a collector was provided).
//
// Log fields: method, path, status, duration_ms, bytes, request_id, remote_addr.
//
// Log level is determined by the response status code:
//   - 2xx/3xx → Info
//   - 4xx     → Warn
//   - 5xx     → Error
func (h *loggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

	h.next.ServeHTTP(wrapped, r)

	duration := time.Since(start)
	level := levelForStatus(wrapped.status)

	slog.Log(r.Context(), level, "http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", wrapped.status,
		"duration_ms", duration.Milliseconds(),
		"bytes", wrapped.bytesWritten,
		"request_id", RequestIDFromContext(r.Context()),
		"remote_addr", r.RemoteAddr,
	)

	// Record Prometheus metrics if a metrics collector was provided.
	if h.metrics != nil {
		statusStr := strconv.Itoa(wrapped.status)
		h.metrics.RequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		h.metrics.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())
	}
}

// Logging is middleware that emits a structured log entry for every HTTP
// request after the handler has completed.
//
// HTTP-level Prometheus metrics (http_requests_total, http_request_duration_seconds)
// are also recorded here, since this middleware already captures all the needed
// dimensions. This avoids wrapping the ResponseWriter a second time.
func Logging(metrics *HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &loggingHandler{next: next, metrics: metrics}
	}
}

// levelForStatus maps an HTTP status code to the appropriate slog level.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
