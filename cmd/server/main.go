package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/internal/observability"
	"github.com/riversheher/atconnect/internal/server"
	"github.com/riversheher/atconnect/pkg/store"
	"github.com/riversheher/atconnect/pkg/store/memory"
	"github.com/riversheher/atconnect/pkg/store/sqlite"
)

func newStore(cfg *config.Config) (store.Store, error) {
	switch cfg.Store.Backend {
	case "memory":
		return memory.New(), nil
	case "sqlite":
		s, err := sqlite.New(cfg.Store.SQLite.Path)
		if err != nil {
			return nil, fmt.Errorf("initializing sqlite store: %w", err)
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported store backend: %s", cfg.Store.Backend)
	}
}

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	flag.Parse()

	// Load configuration (file → env overrides → defaults)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialise structured logging (replaces the duplicated setup code).
	observability.InitLogger(cfg.Log)

	// Register Prometheus metrics.
	metrics := observability.NewMetrics()

	// Create store from configured backend.
	store, err := newStore(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// Create OAuth client with callback URL derived from server config.
	callbackURL := fmt.Sprintf("http://localhost%s/callback", cfg.Server.ListenAddress)
	oauthClient := oauth.NewClient(callbackURL, cfg.OAuth.Scopes, store)

	// Create and configure server.
	srv := server.New(cfg, store, metrics)
	srv.RegisterRoutes(oauthClient)

	// Run server with graceful shutdown.
	slog.Info("atconnect server starting",
		"listen_address", cfg.Server.ListenAddress,
		"store_backend", cfg.Store.Backend,
	)

	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
