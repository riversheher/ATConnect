package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riversheher/atproto-oidc/internal/config"
)

// Server is the main HTTP server for the OIDC bridge.
type Server struct {
	cfg    *config.Config
	mux    *http.ServeMux
	server *http.Server
}

// New creates a new Server with the provided configuration.
func New(cfg *config.Config) *Server {
	mux := http.NewServeMux()
	return &Server{
		cfg: cfg,
		mux: mux,
		server: &http.Server{
			Addr:    cfg.Server.ListenAddress,
			Handler: mux,
		},
	}
}

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It performs graceful shutdown with a 10-second timeout.
func (s *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}
