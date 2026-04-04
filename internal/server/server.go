package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/middleware"
	"github.com/riversheher/atconnect/internal/observability"
	"github.com/riversheher/atconnect/pkg/store"
)

// Server is the main HTTP server for the OIDC bridge.
type Server struct {
	cfg     *config.Config
	mux     *http.ServeMux
	server  *http.Server
	store   store.Store
	metrics *observability.Metrics
}

// New creates a new Server with the provided configuration, store, and metrics.
//
// The store is used for health check probes (/readyz → Store.Ping) and is
// closed during graceful shutdown.
//
// metrics may be nil, in which case middleware will not record Prometheus
// counters (useful for testing or the CLI).
func New(cfg *config.Config, s store.Store, metrics *observability.Metrics) *Server {
	mux := http.NewServeMux()
	return &Server{
		cfg:     cfg,
		mux:     mux,
		store:   s,
		metrics: metrics,
		server: &http.Server{
			Addr:    cfg.Server.ListenAddress,
			Handler: mux,
		},
	}
}

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It performs graceful shutdown with a 10-second timeout, including closing
// the store.
func (s *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Apply the middleware chain to the mux.
	// Order: recovery (outermost) → requestid → logging → mux (innermost).
	var httpMetrics *middleware.HTTPMetrics
	var recoveryMetrics *middleware.RecoveryMetrics
	if s.metrics != nil {
		httpMetrics = s.metrics.HTTP
		recoveryMetrics = s.metrics.Recovery
	}

	handler := middleware.Recovery(recoveryMetrics)(
		middleware.RequestID(
			middleware.Logging(httpMetrics)(s.mux),
		),
	)
	s.server.Handler = handler

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

	// Shut down the HTTP server first, then close the store.
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	if err := s.store.Close(); err != nil {
		slog.Error("store close error", "error", err)
	}

	return nil
}
