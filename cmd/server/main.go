package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/keys"
	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/internal/observability"
	"github.com/riversheher/atconnect/internal/oidc"
	"github.com/riversheher/atconnect/internal/server"
	"github.com/riversheher/atconnect/pkg/models"
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

// registerConfiguredClients saves OIDC clients from config into the store.
func registerConfiguredClients(ctx context.Context, s store.Store, clients []config.OIDCClientConfig) error {
	for _, c := range clients {
		client := models.OIDCClient{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURIs: c.RedirectURIs,
			Name:         c.Name,
			CreatedAt:    time.Now(),
		}
		if err := s.SaveClient(ctx, client); err != nil {
			return fmt.Errorf("registering client %q: %w", c.ClientID, err)
		}
		slog.Info("registered OIDC client from config", "client_id", c.ClientID, "name", c.Name)
	}
	return nil
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

	// Use issuer URL for the callback (public-facing URL).
	issuerURL := strings.TrimRight(cfg.OIDC.IssuerURL, "/")
	callbackURL := issuerURL + "/callback"
	oauthClient := oauth.NewClient(callbackURL, cfg.OAuth.Scopes, store)

	// Optionally set up OIDC provider.
	var oidcProvider *oidc.Provider
	if cfg.OIDC.Enabled {
		ctx := context.Background()

		// Register pre-configured OIDC clients.
		if err := registerConfiguredClients(ctx, store, cfg.OIDC.Clients); err != nil {
			log.Fatalf("Failed to register OIDC clients: %v", err)
		}

		// Initialise key manager (loads or generates signing key).
		km, err := keys.NewManager(ctx, store)
		if err != nil {
			log.Fatalf("Failed to initialize key manager: %v", err)
		}

		oidcProvider = oidc.NewProvider(issuerURL, km, oauthClient, store)
		slog.Info("OIDC provider enabled", "issuer_url", issuerURL)
	}

	// Create and configure server.
	srv := server.New(cfg, store, metrics)
	srv.RegisterRoutes(oauthClient, oidcProvider)

	// Run server with graceful shutdown.
	slog.Info("atconnect server starting",
		"listen_address", cfg.Server.ListenAddress,
		"store_backend", cfg.Store.Backend,
		"oidc_enabled", cfg.OIDC.Enabled,
	)

	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
