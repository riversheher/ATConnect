package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// --- KeyStore implementation ---

// SaveKey stores an OIDC signing key identified by kid. Uses upsert semantics
// so that rotating a key replaces the previous value.
func (s *Store) SaveKey(ctx context.Context, kid string, keyData []byte) error {
	_, err := s.db.ExecContext(ctx, sqlKeyUpsert, kid, keyData)
	if err != nil {
		return fmt.Errorf("sqlite: save key: %w", err)
	}
	return nil
}

// GetKey retrieves a signing key by kid. Returns an error if the key does
// not exist.
func (s *Store) GetKey(ctx context.Context, kid string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, sqlKeyGet, kid).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite: key not found: %s", kid)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get key: %w", err)
	}
	return data, nil
}

// ListKeys returns all stored key IDs, ordered alphabetically.
func (s *Store) ListKeys(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, sqlKeyList)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list keys: %w", err)
	}
	defer rows.Close()

	var kids []string
	for rows.Next() {
		var kid string
		if err := rows.Scan(&kid); err != nil {
			return nil, fmt.Errorf("sqlite: list keys scan: %w", err)
		}
		kids = append(kids, kid)
	}
	return kids, rows.Err()
}
