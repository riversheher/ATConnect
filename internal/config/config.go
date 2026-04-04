package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Log    LogConfig    `yaml:"log"`
	Store  StoreConfig  `yaml:"store"`
	OIDC   OIDCConfig   `yaml:"oidc"`
	OAuth  OAuthConfig  `yaml:"oauth"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `yaml:"level"`
}

// StoreConfig holds data store settings.
type StoreConfig struct {
	Backend string `yaml:"backend"`
}

// OIDCConfig holds OIDC provider settings.
type OIDCConfig struct {
	IssuerURL string `yaml:"issuer_url"`
	Enabled   bool   `yaml:"enabled"`
}

// OAuthConfig holds ATProto OAuth client settings.
type OAuthConfig struct {
	Scopes []string `yaml:"scopes"`
}

// DefaultConfig returns a Config with sensible defaults.
// These defaults are suitable for local development.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddress: ":8080",
		},
		Log: LogConfig{
			Level: "info",
		},
		Store: StoreConfig{
			Backend: "memory",
		},
		OIDC: OIDCConfig{
			IssuerURL: "http://localhost:8080",
			Enabled:   false,
		},
		OAuth: OAuthConfig{
			Scopes: []string{"atproto"},
		},
	}
}

// Load reads configuration from a YAML file (if path is non-empty),
// applies sensible defaults for unset fields, then applies environment
// variable overrides. Returns the merged configuration.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variable overrides take highest precedence.
	if v := os.Getenv("ATCONNECT_LISTEN_ADDRESS"); v != "" {
		cfg.Server.ListenAddress = v
	}
	if v := os.Getenv("ATCONNECT_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("ATCONNECT_STORE_BACKEND"); v != "" {
		cfg.Store.Backend = v
	}
	if v := os.Getenv("ATCONNECT_ISSUER_URL"); v != "" {
		cfg.OIDC.IssuerURL = v
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration has all required fields
// and that values are within acceptable ranges.
func (c *Config) Validate() error {
	if c.Server.ListenAddress == "" {
		return fmt.Errorf("server.listen_address is required")
	}
	if len(c.OAuth.Scopes) == 0 {
		return fmt.Errorf("oauth.scopes must contain at least one scope")
	}
	return nil
}
