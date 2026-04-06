// Package keys provides signing key management for OIDC token issuance.
//
// The default LocalKeyManager generates an EC P-256 key pair on first run,
// persists it to the KeyStore, and loads it on subsequent starts. The
// KeyManager interface is designed so that a KMS-backed adapter (AWS KMS,
// HashiCorp Vault) can be added later without changing callers.
package keys

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"

	"github.com/riversheher/atconnect/pkg/store"
)

// defaultKeyID is the key identifier used for the initial signing key.
// Future key rotation would introduce additional key IDs.
const defaultKeyID = "atconnect-signing-key-1"

// Manager handles OIDC signing key lifecycle: generation, persistence,
// and retrieval. It loads or generates an EC P-256 key pair at
// initialisation time and provides read-only access thereafter.
//
// Safe for concurrent use after construction (all fields are immutable).
type Manager struct {
	store      store.KeyStore
	signingKey *ecdsa.PrivateKey
	keyID      string
	logger     *slog.Logger
}

// NewManager creates a key manager that loads an existing signing key from
// the store, or generates a new EC P-256 key pair if none exists.
//
// This should be called once at server startup. The returned Manager is
// safe for concurrent read access.
func NewManager(ctx context.Context, s store.KeyStore) (*Manager, error) {
	m := &Manager{
		store:  s,
		keyID:  defaultKeyID,
		logger: slog.Default(),
	}

	if err := m.loadOrGenerate(ctx); err != nil {
		return nil, fmt.Errorf("key manager init: %w", err)
	}

	return m, nil
}

// SigningKey returns the ECDSA private key for signing JWTs.
func (m *Manager) SigningKey() *ecdsa.PrivateKey {
	return m.signingKey
}

// PublicKey returns the ECDSA public key for token verification / JWKS.
func (m *Manager) PublicKey() *ecdsa.PublicKey {
	return &m.signingKey.PublicKey
}

// KeyID returns the key identifier (kid) used in JWT headers and JWKS.
func (m *Manager) KeyID() string {
	return m.keyID
}

// loadOrGenerate attempts to load an existing key from the store. If no
// key is found, it generates a new EC P-256 key pair and persists it.
func (m *Manager) loadOrGenerate(ctx context.Context) error {
	// Try to load existing key.
	keyData, err := m.store.GetKey(ctx, m.keyID)
	if err == nil {
		key, parseErr := parsePrivateKey(keyData)
		if parseErr != nil {
			return fmt.Errorf("parsing stored key %q: %w", m.keyID, parseErr)
		}
		m.signingKey = key
		m.logger.Info("loaded existing signing key", "kid", m.keyID)
		return nil
	}

	// Key not found — generate a new one.
	m.logger.Info("no existing signing key found, generating new EC P-256 key", "kid", m.keyID)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generating EC P-256 key: %w", err)
	}

	keyData, err = marshalPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling private key: %w", err)
	}

	if err := m.store.SaveKey(ctx, m.keyID, keyData); err != nil {
		return fmt.Errorf("persisting signing key: %w", err)
	}

	m.signingKey = key
	m.logger.Info("signing key generated and persisted", "kid", m.keyID)
	return nil
}

// marshalPrivateKey encodes an ECDSA private key as PEM (SEC 1 / EC PRIVATE KEY).
func marshalPrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal EC private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	}), nil
}

// parsePrivateKey decodes a PEM-encoded EC private key.
func parsePrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in key data")
	}
	if block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("unexpected PEM block type: %s", block.Type)
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
