package store

import (
	"context"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"

	"github.com/riversheher/atconnect/pkg/models"
)

// Store is the top-level storage interface. It embeds indigo's ClientAuthStore
// (for ATProto OAuth session/auth-request persistence) and adds application-
// specific sub-stores for OIDC signing keys and registered clients.
//
// Every concrete adapter (memory, SQLite, Postgres, …) must implement this
// full interface.
type Store interface {
	indigooauth.ClientAuthStore
	KeyStore
	OIDCClientStore
}

// KeyStore handles persistence of OIDC signing keys (JWKs).
type KeyStore interface {
	// SaveKey stores a signing key identified by kid.
	SaveKey(ctx context.Context, kid string, keyData []byte) error
	// GetKey retrieves a signing key by kid.
	GetKey(ctx context.Context, kid string) ([]byte, error)
	// ListKeys returns all stored key IDs.
	ListKeys(ctx context.Context) ([]string, error)
}

// OIDCClientStore manages registered OIDC relying-party clients.
type OIDCClientStore interface {
	// SaveClient registers or updates an OIDC client.
	SaveClient(ctx context.Context, client models.OIDCClient) error
	// GetClient retrieves a client by client_id.
	GetClient(ctx context.Context, clientID string) (*models.OIDCClient, error)
	// ListClients returns all registered clients.
	ListClients(ctx context.Context) ([]models.OIDCClient, error)
	// DeleteClient removes a client by client_id.
	DeleteClient(ctx context.Context, clientID string) error
}
