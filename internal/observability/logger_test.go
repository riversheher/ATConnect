package observability_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/observability"
)

func TestInitLogger_ParsesLevels(t *testing.T) {
	tests := []struct {
		level     string
		wantLevel slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			logger := observability.InitLogger(config.LogConfig{
				Level:  tt.level,
				Format: "text",
			})
			if !logger.Enabled(nil, tt.wantLevel) {
				t.Errorf("expected level %v to be enabled for config level %q", tt.wantLevel, tt.level)
			}
		})
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func TestInitLogger_JSONFormat(t *testing.T) {
	logger := observability.InitLogger(config.LogConfig{
		Level:  "info",
		Format: "json",
	})
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	})

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	if slog.Default() == nil {
		t.Fatal("expected slog.Default() to be set")
	}
}

func TestInitLogger_TextFormat(t *testing.T) {
	logger := observability.InitLogger(config.LogConfig{
		Level:  "info",
		Format: "text",
	})
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	})

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}
