package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/internal/server"
	"github.com/riversheher/atconnect/pkg/store/memory"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	flag.Parse()

	// Load configuration (file → env overrides → defaults)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set up structured logging
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	// Create store (memory for Phase 1; future: sqlite, postgres)
	store := memory.New()

	// Create OAuth client with callback URL derived from server config
	callbackURL := fmt.Sprintf("http://localhost%s/callback", cfg.Server.ListenAddress)
	oauthClient := oauth.NewClient(callbackURL, cfg.OAuth.Scopes, store)

	// Create and configure server
	srv := server.New(cfg)
	srv.RegisterRoutes(oauthClient)

	// Run server with graceful shutdown
	slog.Info("atconnect server starting",
		"listen_address", cfg.Server.ListenAddress,
		"store_backend", cfg.Store.Backend,
	)

	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
