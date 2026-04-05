// Package sqlite provides a SQLite-backed implementation of store.Store using
// the modernc.org/sqlite driver (pure Go, no cgo required).
//
// The package is organised by domain concern:
//
//   - store.go       — lifecycle (New, Ping, Close, schema init)
//   - queries.go     — all SQL statements as Go constants
//   - session.go     — OAuth session persistence (ClientAuthStore)
//   - auth_request.go — OAuth auth-request state (ClientAuthStore)
//   - key.go         — OIDC signing key persistence (KeyStore)
//   - client.go      — OIDC relying-party client persistence (OIDCClientStore)
//
// SQL is separated from Go logic but kept as constants in queries.go rather
// than individual embedded files — see .github/storage_reasoning.md for the
// rationale behind this choice.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riversheher/atconnect/pkg/store"

	_ "modernc.org/sqlite"
)

// Compile-time check that Store implements store.Store.
var _ store.Store = (*Store)(nil)

// Store is a SQLite-backed implementation of store.Store.
//
// All domain methods (sessions, auth requests, keys, clients) are defined
// directly on this type in their respective files. The Store holds a single
// *sql.DB connection which is shared across all operations.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dbPath, configures pragmas for
// safe concurrent access, applies the schema, and returns a ready-to-use Store.
func New(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite: db path is required")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening database: %w", err)
	}

	// SQLite performs best with a single writer connection in our usage
	// pattern. This avoids SQLITE_BUSY errors without requiring retry logic.
	// Can be relaxed later with connection pooling + busy_timeout.
	db.SetMaxOpenConns(1)

	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := applySchema(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping failed: %w", err)
	}

	return &Store{db: db}, nil
}

// applyPragmas sets SQLite pragmas for performance and correctness.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",  // Write-Ahead Logging for concurrent reads
		"PRAGMA foreign_keys=ON",   // Enforce FK constraints (off by default)
		"PRAGMA busy_timeout=5000", // Wait up to 5s on lock contention
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("sqlite: %s: %w", p, err)
		}
	}
	return nil
}

// applySchema creates tables if they don't already exist.
func applySchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, sqlSchema); err != nil {
		return fmt.Errorf("sqlite: applying schema: %w", err)
	}
	return nil
}

// --- Lifecycle ---

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Helpers ---

func marshalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return data, nil
}

func unmarshalJSON(data []byte, into any) error {
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

// isUniqueViolation checks whether an error is a SQLite UNIQUE constraint
// violation. Uses string matching as modernc.org/sqlite does not export
// typed error codes — see SC-1 in storage_concerns.md.
func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
