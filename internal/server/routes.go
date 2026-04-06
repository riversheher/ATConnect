package server

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/internal/observability"
	"github.com/riversheher/atconnect/internal/oidc"
)

// RegisterRoutes sets up all HTTP routes for the server.
//
// Route groups:
//   - /callback   — ATProto OAuth redirect handler
//   - /livez      — liveness probe (always 200)
//   - /readyz     — readiness probe (checks store connectivity)
//   - /metrics    — Prometheus scrape endpoint
//
// When oidcProvider is non-nil (OIDC enabled), additional routes are registered:
//   - /.well-known/openid-configuration — OIDC discovery document
//   - /jwks       — JSON Web Key Set
//   - /authorize  — OIDC authorization endpoint
//   - /token      — OIDC token endpoint
//   - /callback   — OIDC-aware ATProto callback (replaces plain callback)
func (s *Server) RegisterRoutes(oauthClient *oauth.Client, oidcProvider *oidc.Provider) {
	// Callback handler: OIDC-aware if provider is enabled, plain otherwise.
	if oidcProvider != nil {
		// OIDC endpoints
		s.mux.HandleFunc("GET /.well-known/openid-configuration", oidcProvider.HandleDiscovery)
		s.mux.HandleFunc("GET /jwks", oidcProvider.HandleJWKS)
		s.mux.HandleFunc("GET /authorize", oidcProvider.HandleAuthorize)
		s.mux.HandleFunc("POST /token", oidcProvider.HandleToken)

		// OIDC-aware callback (handles both OIDC flows and plain ATProto auth)
		s.mux.HandleFunc("/callback", oidcProvider.HandleCallback)

		slog.Info("OIDC provider routes registered")
	} else {
		// Plain ATProto callback (no OIDC)
		s.mux.HandleFunc("/callback", handleOAuthCallback(oauthClient))
	}

	// Health probes
	health := observability.NewHealthChecker(s.store)
	s.mux.HandleFunc("/livez", health.ServeLivez)
	s.mux.HandleFunc("/readyz", health.ServeReadyz)

	// Prometheus metrics
	s.mux.Handle("/metrics", promhttp.Handler())
}

// handleOAuthCallback processes the ATProto OAuth redirect callback.
// Used when OIDC is not enabled.
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
