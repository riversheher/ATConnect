// Package middleware provides HTTP middleware for the ATConnect server.
//
// All middleware follows the standard func(http.Handler) http.Handler signature
// and composes naturally: recovery(requestid(logging(handler))).
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// requestIDKey is a package-private context key type. Using a struct type
// (rather than a string) prevents collisions with other packages that might
// store a "request_id" value in context.
type requestIDKey struct{}

// headerRequestID is the standard HTTP header for request correlation.
const headerRequestID = "X-Request-ID"

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID has been set.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// requestIDHandler is the HTTP handler produced by the RequestID middleware.
type requestIDHandler struct {
	next http.Handler
}

// ServeHTTP ensures every request has an X-Request-ID. If the incoming request
// already carries the header, that value is propagated. Otherwise, a new
// 128-bit random hex ID is generated.
//
// The ID is stored in the request context (accessible via RequestIDFromContext)
// and set on the response header.
func (h *requestIDHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get(headerRequestID)
	if id == "" {
		id = generateID()
	}

	// Store in context for downstream handlers and middleware.
	ctx := context.WithValue(r.Context(), requestIDKey{}, id)

	// Set on response so callers can correlate responses to requests.
	w.Header().Set(headerRequestID, id)

	h.next.ServeHTTP(w, r.WithContext(ctx))
}

// RequestID is middleware that ensures every request has an X-Request-ID.
func RequestID(next http.Handler) http.Handler {
	return &requestIDHandler{next: next}
}

// generateID produces a 128-bit (16-byte) cryptographically random hex string.
// This provides ample collision resistance for request correlation without
// requiring an external UUID dependency.
func generateID() string {
	b := make([]byte, 16)
	// crypto/rand.Read never returns an error on Linux/macOS/Windows.
	// If it did (e.g. entropy exhaustion), falling back to an empty ID
	// is acceptable — the logging middleware will simply log an empty field.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
