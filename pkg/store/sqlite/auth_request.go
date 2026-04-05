package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
)

// --- ClientAuthStore: auth-request methods ---

// GetAuthRequestInfo retrieves a pending auth request by its state token.
// Returns a non-nil error if the request does not exist.
func (s *Store) GetAuthRequestInfo(ctx context.Context, state string) (*indigooauth.AuthRequestData, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, sqlAuthRequestGet, state).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite: auth request not found: %s", state)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get auth request: %w", err)
	}

	var info indigooauth.AuthRequestData
	if err := unmarshalJSON(blob, &info); err != nil {
		return nil, fmt.Errorf("sqlite: get auth request: %w", err)
	}
	return &info, nil
}

// SaveAuthRequestInfo creates a new auth request. Per the ClientAuthStore
// contract, this is create-only — attempting to save a request with a
// duplicate state token returns an error.
func (s *Store) SaveAuthRequestInfo(ctx context.Context, info indigooauth.AuthRequestData) error {
	blob, err := marshalJSON(info)
	if err != nil {
		return fmt.Errorf("sqlite: save auth request: %w", err)
	}

	_, err = s.db.ExecContext(ctx, sqlAuthRequestCreate, info.State, blob)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("sqlite: auth request already exists: %s", info.State)
		}
		return fmt.Errorf("sqlite: save auth request: %w", err)
	}
	return nil
}

// DeleteAuthRequestInfo removes a pending auth request by state token.
// It is not an error to delete a non-existent request.
func (s *Store) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	_, err := s.db.ExecContext(ctx, sqlAuthRequestDelete, state)
	if err != nil {
		return fmt.Errorf("sqlite: delete auth request: %w", err)
	}
	return nil
}
