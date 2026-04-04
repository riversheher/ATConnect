// Package observability provides application-level infrastructure for logging,
// metrics, and health checks. All components are initialised once at startup
// and shared across the application.
package observability

import (
	"log/slog"
	"os"

	"github.com/riversheher/atconnect/internal/config"
)

// InitLogger creates a configured *slog.Logger from the application's LogConfig.
//
// It also sets slog.SetDefault() so that any code using the package-level slog
// functions (slog.Info, slog.Error, etc.) automatically uses this logger.
//
// The returned logger can be injected into structs that prefer explicit
// dependencies over the global default.
func InitLogger(cfg config.LogConfig) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// parseLevel converts a string log level to the corresponding slog.Level.
// Unrecognised values default to Info.
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
