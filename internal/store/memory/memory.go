package memory

import (
	"context"
	"fmt"
	"sync"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"

	"github.com/riversheher/atproto-oidc/internal/models"
	"github.com/riversheher/atproto-oidc/internal/store"
)

// Compile-time check that MemoryStore implements store.Store.
var _ store.Store = (*MemoryStore)(nil)

// MemoryStore is an in-memory implementation of store.Store.
// It embeds indigo's MemStore for ClientAuthStore methods and adds
// map-based storage for keys and OIDC clients.
//
// Safe for concurrent use.
type MemoryStore struct {
	*indigooauth.MemStore

	mu      sync.RWMutex
	keys    map[string][]byte
	clients map[string]models.OIDCClient
}

// New returns an initialised MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		MemStore: indigooauth.NewMemStore(),
		keys:     make(map[string][]byte),
		clients:  make(map[string]models.OIDCClient),
	}
}

// --- KeyStore implementation ---

func (m *MemoryStore) SaveKey(_ context.Context, kid string, keyData []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[kid] = keyData
	return nil
}

func (m *MemoryStore) GetKey(_ context.Context, kid string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", kid)
	}
	return data, nil
}

func (m *MemoryStore) ListKeys(_ context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	kids := make([]string, 0, len(m.keys))
	for kid := range m.keys {
		kids = append(kids, kid)
	}
	return kids, nil
}

// --- OIDCClientStore implementation ---

func (m *MemoryStore) SaveClient(_ context.Context, client models.OIDCClient) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client.ClientID] = client
	return nil
}

func (m *MemoryStore) GetClient(_ context.Context, clientID string) (*models.OIDCClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.clients[clientID]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	return &client, nil
}

func (m *MemoryStore) ListClients(_ context.Context) ([]models.OIDCClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clients := make([]models.OIDCClient, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	return clients, nil
}

func (m *MemoryStore) DeleteClient(_ context.Context, clientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, clientID)
	return nil
}
