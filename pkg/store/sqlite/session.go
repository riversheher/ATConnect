package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// --- ClientAuthStore: session methods ---

// GetSession retrieves an OAuth session for the given DID and session ID.
// Returns a non-nil error if the session does not exist.
func (s *Store) GetSession(ctx context.Context, did syntax.DID, sessionID string) (*indigooauth.ClientSessionData, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, sqlSessionGet, did.String(), sessionID).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite: session not found: %s/%s", did, sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get session: %w", err)
	}

	var sess indigooauth.ClientSessionData
	if err := unmarshalJSON(blob, &sess); err != nil {
		return nil, fmt.Errorf("sqlite: get session: %w", err)
	}
	return &sess, nil
}

// SaveSession upserts an OAuth session. If a session with the same DID and
// session ID already exists, it is replaced (upsert semantics per
// ClientAuthStore contract).
func (s *Store) SaveSession(ctx context.Context, sess indigooauth.ClientSessionData) error {
	blob, err := marshalJSON(sess)
	if err != nil {
		return fmt.Errorf("sqlite: save session: %w", err)
	}

	_, err = s.db.ExecContext(ctx, sqlSessionUpsert, sess.AccountDID.String(), sess.SessionID, blob)
	if err != nil {
		return fmt.Errorf("sqlite: save session: %w", err)
	}
	return nil
}

// DeleteSession removes the session for the given DID and session ID.
// It is not an error to delete a non-existent session.
func (s *Store) DeleteSession(ctx context.Context, did syntax.DID, sessionID string) error {
	_, err := s.db.ExecContext(ctx, sqlSessionDelete, did.String(), sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete session: %w", err)
	}
	return nil
}
