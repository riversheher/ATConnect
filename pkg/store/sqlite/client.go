package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/riversheher/atconnect/pkg/models"
)

// --- OIDCClientStore implementation ---

// SaveClient registers or updates an OIDC relying-party client.
// Uses upsert semantics — re-registering the same client_id overwrites it.
func (s *Store) SaveClient(ctx context.Context, client models.OIDCClient) error {
	blob, err := marshalJSON(client)
	if err != nil {
		return fmt.Errorf("sqlite: save client: %w", err)
	}

	_, err = s.db.ExecContext(ctx, sqlClientUpsert, client.ClientID, blob)
	if err != nil {
		return fmt.Errorf("sqlite: save client: %w", err)
	}
	return nil
}

// GetClient retrieves a client by client_id. Returns an error if the client
// does not exist.
func (s *Store) GetClient(ctx context.Context, clientID string) (*models.OIDCClient, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, sqlClientGet, clientID).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite: client not found: %s", clientID)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get client: %w", err)
	}

	var client models.OIDCClient
	if err := unmarshalJSON(blob, &client); err != nil {
		return nil, fmt.Errorf("sqlite: get client: %w", err)
	}
	return &client, nil
}

// ListClients returns all registered clients, ordered by client_id.
func (s *Store) ListClients(ctx context.Context) ([]models.OIDCClient, error) {
	rows, err := s.db.QueryContext(ctx, sqlClientList)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list clients: %w", err)
	}
	defer rows.Close()

	var clients []models.OIDCClient
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("sqlite: list clients scan: %w", err)
		}
		var client models.OIDCClient
		if err := unmarshalJSON(blob, &client); err != nil {
			return nil, fmt.Errorf("sqlite: list clients unmarshal: %w", err)
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

// DeleteClient removes a client by client_id.
// It is not an error to delete a non-existent client.
func (s *Store) DeleteClient(ctx context.Context, clientID string) error {
	_, err := s.db.ExecContext(ctx, sqlClientDelete, clientID)
	if err != nil {
		return fmt.Errorf("sqlite: delete client: %w", err)
	}
	return nil
}
