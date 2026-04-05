package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"github.com/riversheher/atconnect/pkg/models"
	"github.com/riversheher/atconnect/pkg/store"
	"github.com/riversheher/atconnect/pkg/store/sqlite"
)

// newTestStore creates a temporary SQLite store for the duration of the test.
// The database file is automatically cleaned up when the test finishes.
func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ---------------------------------------------------------------------------
// Interface conformance
// ---------------------------------------------------------------------------

func TestStore_ImplementsStoreInterface(t *testing.T) {
	s := newTestStore(t)
	// Compile-time check is in store.go; this verifies at test time too.
	var _ store.Store = s
}

// ---------------------------------------------------------------------------
// New / lifecycle
// ---------------------------------------------------------------------------

func TestNew_EmptyPath(t *testing.T) {
	_, err := sqlite.New("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := sqlite.New("/this/path/does/not/exist/db.sqlite")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestNew_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "new.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestPing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose_ThenPingFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "close.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Ping(context.Background()); err == nil {
		t.Fatal("expected Ping to fail after Close")
	}
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func makeSession(did string, sessionID string) indigooauth.ClientSessionData {
	return indigooauth.ClientSessionData{
		AccountDID:    syntax.DID(did),
		SessionID:     sessionID,
		HostURL:       "https://pds.example.com",
		AuthServerURL: "https://auth.example.com",
		Scopes:        []string{"atproto"},
		AccessToken:   "access-" + sessionID,
		RefreshToken:  "refresh-" + sessionID,
	}
}

func TestSession_SaveAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sess := makeSession("did:plc:test123", "sess-1")

	if err := s.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := s.GetSession(ctx, syntax.DID("did:plc:test123"), "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AccountDID.String() != "did:plc:test123" {
		t.Errorf("AccountDID = %s, want did:plc:test123", got.AccountDID)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("SessionID = %s, want sess-1", got.SessionID)
	}
	if got.AccessToken != "access-sess-1" {
		t.Errorf("AccessToken = %s, want access-sess-1", got.AccessToken)
	}
}

func TestSession_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetSession(ctx, syntax.DID("did:plc:unknown"), "no-such-session")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestSession_UpsertOverwrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:upsert")

	sess := makeSession("did:plc:upsert", "s1")
	if err := s.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession (create): %v", err)
	}

	// Update the access token.
	sess.AccessToken = "updated-token"
	if err := s.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession (upsert): %v", err)
	}

	got, err := s.GetSession(ctx, did, "s1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AccessToken != "updated-token" {
		t.Errorf("AccessToken = %s, want updated-token", got.AccessToken)
	}
}

func TestSession_MultipleSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:multi")

	s1 := makeSession("did:plc:multi", "sess-a")
	s2 := makeSession("did:plc:multi", "sess-b")
	s2.AccessToken = "different-token"

	if err := s.SaveSession(ctx, s1); err != nil {
		t.Fatalf("SaveSession s1: %v", err)
	}
	if err := s.SaveSession(ctx, s2); err != nil {
		t.Fatalf("SaveSession s2: %v", err)
	}

	got1, err := s.GetSession(ctx, did, "sess-a")
	if err != nil {
		t.Fatalf("GetSession sess-a: %v", err)
	}
	got2, err := s.GetSession(ctx, did, "sess-b")
	if err != nil {
		t.Fatalf("GetSession sess-b: %v", err)
	}

	if got1.AccessToken == got2.AccessToken {
		t.Error("expected different access tokens for different sessions")
	}
}

func TestSession_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	did := syntax.DID("did:plc:delete")

	sess := makeSession("did:plc:delete", "s1")
	if err := s.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := s.DeleteSession(ctx, did, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := s.GetSession(ctx, did, "s1")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestSession_DeleteNonExistent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Deleting a non-existent session should not error.
	if err := s.DeleteSession(ctx, syntax.DID("did:plc:nope"), "no-such"); err != nil {
		t.Fatalf("DeleteSession non-existent: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Auth Requests
// ---------------------------------------------------------------------------

func makeAuthRequest(state string) indigooauth.AuthRequestData {
	return indigooauth.AuthRequestData{
		State:         state,
		AuthServerURL: "https://auth.example.com",
		Scopes:        []string{"atproto"},
		RequestURI:    "urn:ietf:params:oauth:request_uri:example",
		PKCEVerifier:  "pkce-verifier-" + state,
	}
}

func TestAuthRequest_SaveAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	info := makeAuthRequest("state-abc")
	if err := s.SaveAuthRequestInfo(ctx, info); err != nil {
		t.Fatalf("SaveAuthRequestInfo: %v", err)
	}

	got, err := s.GetAuthRequestInfo(ctx, "state-abc")
	if err != nil {
		t.Fatalf("GetAuthRequestInfo: %v", err)
	}
	if got.State != "state-abc" {
		t.Errorf("State = %s, want state-abc", got.State)
	}
	if got.PKCEVerifier != "pkce-verifier-state-abc" {
		t.Errorf("PKCEVerifier = %s, want pkce-verifier-state-abc", got.PKCEVerifier)
	}
}

func TestAuthRequest_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetAuthRequestInfo(ctx, "no-such-state")
	if err == nil {
		t.Fatal("expected error for missing auth request")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestAuthRequest_CreateOnly_RejectsDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	info := makeAuthRequest("dup-state")
	if err := s.SaveAuthRequestInfo(ctx, info); err != nil {
		t.Fatalf("SaveAuthRequestInfo (first): %v", err)
	}

	// Second save with same state must fail (create-only semantics).
	err := s.SaveAuthRequestInfo(ctx, info)
	if err == nil {
		t.Fatal("expected error on duplicate state")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

func TestAuthRequest_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	info := makeAuthRequest("state-del")
	if err := s.SaveAuthRequestInfo(ctx, info); err != nil {
		t.Fatalf("SaveAuthRequestInfo: %v", err)
	}

	if err := s.DeleteAuthRequestInfo(ctx, "state-del"); err != nil {
		t.Fatalf("DeleteAuthRequestInfo: %v", err)
	}

	_, err := s.GetAuthRequestInfo(ctx, "state-del")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestAuthRequest_DeleteNonExistent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.DeleteAuthRequestInfo(ctx, "no-such"); err != nil {
		t.Fatalf("DeleteAuthRequestInfo non-existent: %v", err)
	}
}

func TestAuthRequest_DeleteThenRecreate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	info := makeAuthRequest("state-reuse")
	if err := s.SaveAuthRequestInfo(ctx, info); err != nil {
		t.Fatalf("SaveAuthRequestInfo: %v", err)
	}
	if err := s.DeleteAuthRequestInfo(ctx, "state-reuse"); err != nil {
		t.Fatalf("DeleteAuthRequestInfo: %v", err)
	}
	// After deletion, the same state can be reused.
	if err := s.SaveAuthRequestInfo(ctx, info); err != nil {
		t.Fatalf("SaveAuthRequestInfo (recreate): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

func TestKey_SaveAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	keyData := []byte(`{"kty":"EC","crv":"P-256","x":"abc","y":"def"}`)
	if err := s.SaveKey(ctx, "kid-1", keyData); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}

	got, err := s.GetKey(ctx, "kid-1")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if string(got) != string(keyData) {
		t.Errorf("GetKey returned %s, want %s", got, keyData)
	}
}

func TestKey_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetKey(ctx, "no-such-kid")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestKey_UpsertOverwrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original := []byte(`{"version":1}`)
	updated := []byte(`{"version":2}`)

	if err := s.SaveKey(ctx, "kid-up", original); err != nil {
		t.Fatalf("SaveKey (create): %v", err)
	}
	if err := s.SaveKey(ctx, "kid-up", updated); err != nil {
		t.Fatalf("SaveKey (upsert): %v", err)
	}

	got, err := s.GetKey(ctx, "kid-up")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if string(got) != string(updated) {
		t.Errorf("GetKey = %s, want %s", got, updated)
	}
}

func TestKey_ListEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	kids, err := s.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(kids) != 0 {
		t.Errorf("ListKeys = %v, want empty", kids)
	}
}

func TestKey_ListMultiple(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, kid := range []string{"kid-c", "kid-a", "kid-b"} {
		if err := s.SaveKey(ctx, kid, []byte("data")); err != nil {
			t.Fatalf("SaveKey %s: %v", kid, err)
		}
	}

	kids, err := s.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(kids) != 3 {
		t.Fatalf("ListKeys: got %d keys, want 3", len(kids))
	}
	// Keys should be ordered alphabetically (ORDER BY kid ASC).
	if kids[0] != "kid-a" || kids[1] != "kid-b" || kids[2] != "kid-c" {
		t.Errorf("ListKeys = %v, want [kid-a kid-b kid-c]", kids)
	}
}

// ---------------------------------------------------------------------------
// OIDC Clients
// ---------------------------------------------------------------------------

func makeClient(id string) models.OIDCClient {
	return models.OIDCClient{
		ClientID:     id,
		ClientSecret: "secret-" + id,
		RedirectURIs: []string{"https://example.com/callback"},
		Name:         "Test Client " + id,
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestClient_SaveAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	client := makeClient("client-1")
	if err := s.SaveClient(ctx, client); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	got, err := s.GetClient(ctx, "client-1")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.ClientID != "client-1" {
		t.Errorf("ClientID = %s, want client-1", got.ClientID)
	}
	if got.Name != "Test Client client-1" {
		t.Errorf("Name = %s, want 'Test Client client-1'", got.Name)
	}
	if len(got.RedirectURIs) != 1 || got.RedirectURIs[0] != "https://example.com/callback" {
		t.Errorf("RedirectURIs = %v, want [https://example.com/callback]", got.RedirectURIs)
	}
}

func TestClient_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetClient(ctx, "no-such-client")
	if err == nil {
		t.Fatal("expected error for missing client")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestClient_UpsertOverwrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	client := makeClient("client-up")
	if err := s.SaveClient(ctx, client); err != nil {
		t.Fatalf("SaveClient (create): %v", err)
	}

	client.Name = "Updated Name"
	if err := s.SaveClient(ctx, client); err != nil {
		t.Fatalf("SaveClient (upsert): %v", err)
	}

	got, err := s.GetClient(ctx, "client-up")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name = %s, want 'Updated Name'", got.Name)
	}
}

func TestClient_ListEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	clients, err := s.ListClients(ctx)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(clients) != 0 {
		t.Errorf("ListClients = %v, want empty", clients)
	}
}

func TestClient_ListMultiple(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"c-beta", "c-alpha", "c-gamma"} {
		if err := s.SaveClient(ctx, makeClient(id)); err != nil {
			t.Fatalf("SaveClient %s: %v", id, err)
		}
	}

	clients, err := s.ListClients(ctx)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(clients) != 3 {
		t.Fatalf("ListClients: got %d, want 3", len(clients))
	}
	// Clients should be ordered by client_id ASC.
	if clients[0].ClientID != "c-alpha" || clients[1].ClientID != "c-beta" || clients[2].ClientID != "c-gamma" {
		t.Errorf("ListClients order = [%s %s %s], want [c-alpha c-beta c-gamma]",
			clients[0].ClientID, clients[1].ClientID, clients[2].ClientID)
	}
}

func TestClient_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	client := makeClient("client-del")
	if err := s.SaveClient(ctx, client); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	if err := s.DeleteClient(ctx, "client-del"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	_, err := s.GetClient(ctx, "client-del")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestClient_DeleteNonExistent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.DeleteClient(ctx, "no-such"); err != nil {
		t.Fatalf("DeleteClient non-existent: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cross-domain isolation
// ---------------------------------------------------------------------------

func TestCrossDomain_IndependentTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert data into every domain.
	if err := s.SaveSession(ctx, makeSession("did:plc:iso", "s1")); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := s.SaveAuthRequestInfo(ctx, makeAuthRequest("state-iso")); err != nil {
		t.Fatalf("SaveAuthRequestInfo: %v", err)
	}
	if err := s.SaveKey(ctx, "kid-iso", []byte("key-data")); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	if err := s.SaveClient(ctx, makeClient("client-iso")); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	// Deleting from one domain should not affect others.
	if err := s.DeleteSession(ctx, syntax.DID("did:plc:iso"), "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// Auth request should still exist.
	if _, err := s.GetAuthRequestInfo(ctx, "state-iso"); err != nil {
		t.Errorf("GetAuthRequestInfo after session delete: %v", err)
	}
	// Key should still exist.
	if _, err := s.GetKey(ctx, "kid-iso"); err != nil {
		t.Errorf("GetKey after session delete: %v", err)
	}
	// Client should still exist.
	if _, err := s.GetClient(ctx, "client-iso"); err != nil {
		t.Errorf("GetClient after session delete: %v", err)
	}
}
