package server

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/internal/observability"
)

// RegisterRoutes sets up all HTTP routes for the server.
//
// Route groups:
//   - /callback   — ATProto OAuth redirect handler
//   - /livez      — liveness probe (always 200)
//   - /readyz     — readiness probe (checks store connectivity)
//   - /metrics    — Prometheus scrape endpoint
func (s *Server) RegisterRoutes(oauthClient *oauth.Client) {
	// OAuth callback
	s.mux.HandleFunc("/callback", handleOAuthCallback(oauthClient))

	// Health probes
	health := observability.NewHealthChecker(s.store)
	s.mux.HandleFunc("/livez", health.ServeLivez)
	s.mux.HandleFunc("/readyz", health.ServeReadyz)

	// Prometheus metrics
	s.mux.Handle("/metrics", promhttp.Handler())
}

// handleOAuthCallback processes the ATProto OAuth redirect callback.
func handleOAuthCallback(oauthClient *oauth.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := oauthClient.HandleCallback(r.Context(), r.URL.Query())
		if err != nil {
			slog.Error("OAuth callback failed", "error", err)
			http.Error(w, "Authentication failed", http.StatusInternalServerError)
			return
		}

		slog.Info("OAuth callback successful", "did", sess.AccountDID)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Authentication successful! DID: " + string(sess.AccountDID)))
	}
}
