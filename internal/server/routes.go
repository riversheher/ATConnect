package server

import (
	"log/slog"
	"net/http"

	"github.com/riversheher/atconnect/internal/oauth"
)

// RegisterRoutes sets up all HTTP routes for the server.
func (s *Server) RegisterRoutes(oauthClient *oauth.Client) {
	s.mux.HandleFunc("/callback", handleOAuthCallback(oauthClient))
	s.mux.HandleFunc("/health", handleHealth())
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

// handleHealth returns a simple OK response for liveness checks.
func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}
