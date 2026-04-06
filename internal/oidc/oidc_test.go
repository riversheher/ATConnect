package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/riversheher/atconnect/internal/keys"
	"github.com/riversheher/atconnect/pkg/models"
	"github.com/riversheher/atconnect/pkg/store/memory"
)

// --- Test helpers ---

// testProvider creates a Provider with an in-memory store, an EC P-256
// signing key, and a pre-registered OIDC client.
//
// The oauthClient field is nil — tests must not exercise paths that
// call into ATProto OAuth (HandleAuthorize with login_hint, HandleCallback).
func testProvider(t *testing.T) *Provider {
	t.Helper()

	s := memory.New()

	// Generate a real signing key via the key manager (backed by memory store).
	km, err := keys.NewManager(context.Background(), s)
	if err != nil {
		t.Fatalf("create key manager: %v", err)
	}

	// Register a test client.
	_ = s.SaveClient(context.Background(), models.OIDCClient{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURIs: []string{"https://rp.example.com/callback"},
		Name:         "Test RP",
	})

	return &Provider{
		issuerURL:       "https://provider.example.com",
		keyManager:      km,
		oauthClient:     nil, // no ATProto in unit tests
		store:           s,
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		pendingRequests: make(map[string]*pendingAuthorization),
		codeGrants:      make(map[string]*codeGrant),
	}
}

// decodeJWTClaims extracts the claims from a JWT without signature
// verification. This is necessary because the indigo library registers a
// custom ES256 signing method whose Verify() expects indigo's own key
// wrapper, not a bare *ecdsa.PublicKey.
func decodeJWTClaims(t *testing.T, tokenString string) map[string]any {
	t.Helper()
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT should have 3 parts, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal JWT claims: %v", err)
	}
	return claims
}

// insertCodeGrant manually inserts a codeGrant into the provider's map.
// This lets us test HandleToken without exercising ATProto auth.
func insertCodeGrant(p *Provider, grant *codeGrant) {
	p.codeGrantsMu.Lock()
	p.codeGrants[grant.Code] = grant
	p.codeGrantsMu.Unlock()
}

// --- Discovery endpoint ---

func TestDiscovery_ReturnsCorrectMetadata(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	p.HandleDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var meta models.ProviderMetadata
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if meta.Issuer != "https://provider.example.com" {
		t.Errorf("issuer = %q, want %q", meta.Issuer, "https://provider.example.com")
	}
	if meta.AuthorizationEndpoint != "https://provider.example.com/authorize" {
		t.Errorf("authorization_endpoint = %q", meta.AuthorizationEndpoint)
	}
	if meta.TokenEndpoint != "https://provider.example.com/token" {
		t.Errorf("token_endpoint = %q", meta.TokenEndpoint)
	}
	if meta.JWKSURI != "https://provider.example.com/jwks" {
		t.Errorf("jwks_uri = %q", meta.JWKSURI)
	}
	if len(meta.IDTokenSigningAlg) != 1 || meta.IDTokenSigningAlg[0] != "ES256" {
		t.Errorf("id_token_signing_alg_values_supported = %v", meta.IDTokenSigningAlg)
	}
}

// --- JWKS endpoint ---

func TestJWKS_ReturnsValidJWKSet(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jwks", nil)
	p.HandleJWKS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
			Kid string `json:"kid"`
			Use string `json:"use"`
			Alg string `json:"alg"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&set); err != nil {
		t.Fatalf("decode JWKS: %v", err)
	}

	if len(set.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(set.Keys))
	}

	k := set.Keys[0]
	if k.Kty != "EC" {
		t.Errorf("kty = %q, want EC", k.Kty)
	}
	if k.Crv != "P-256" {
		t.Errorf("crv = %q, want P-256", k.Crv)
	}
	if k.Alg != "ES256" {
		t.Errorf("alg = %q, want ES256", k.Alg)
	}
	if k.Use != "sig" {
		t.Errorf("use = %q, want sig", k.Use)
	}
	if k.Kid != p.keyManager.KeyID() {
		t.Errorf("kid = %q, want %q", k.Kid, p.keyManager.KeyID())
	}

	// Coordinates must be valid base64url and 32 bytes (P-256).
	for _, coord := range []struct {
		name string
		val  string
	}{{"x", k.X}, {"y", k.Y}} {
		b, err := base64.RawURLEncoding.DecodeString(coord.val)
		if err != nil {
			t.Errorf("%s: invalid base64url: %v", coord.name, err)
		}
		if len(b) != 32 {
			t.Errorf("%s: expected 32 bytes, got %d", coord.name, len(b))
		}
	}
}

// --- Authorize endpoint validation ---

func TestAuthorize_MissingClientID(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/authorize", nil)
	p.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid_request" {
		t.Errorf("error = %q, want invalid_request", body["error"])
	}
}

func TestAuthorize_UnknownClientID(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/authorize?client_id=unknown", nil)
	p.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAuthorize_UnregisteredRedirectURI(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?client_id=test-client&redirect_uri=https://evil.example.com/callback", nil)
	p.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid_request" {
		t.Errorf("error = %q, want invalid_request", body["error"])
	}
}

func TestAuthorize_UnsupportedResponseType(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?client_id=test-client&redirect_uri=https://rp.example.com/callback&response_type=token&scope=openid", nil)
	p.HandleAuthorize(rec, req)

	// Should redirect with error (not render error page — redirect_uri is valid).
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}

	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Query().Get("error") != "unsupported_response_type" {
		t.Errorf("error = %q, want unsupported_response_type", loc.Query().Get("error"))
	}
}

func TestAuthorize_MissingLoginHint_ShowsForm(t *testing.T) {
	p := testProvider(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?client_id=test-client&redirect_uri=https://rp.example.com/callback&response_type=code&scope=openid", nil)
	p.HandleAuthorize(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "login_hint") {
		t.Error("login form should contain login_hint input")
	}
}

// --- Token endpoint ---

func TestToken_ValidExchange(t *testing.T) {
	p := testProvider(t)

	// Manually inject a code grant (simulates a completed ATProto auth).
	insertCodeGrant(p, &codeGrant{
		Code:        "valid-code-123",
		ClientID:    "test-client",
		RedirectURI: "https://rp.example.com/callback",
		Nonce:       "test-nonce",
		DID:         "did:plc:testuser123",
		Handle:      "alice.bsky.social",
		PDS:         "https://pds.example.com",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"valid-code-123"},
		"redirect_uri":  {"https://rp.example.com/callback"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	p.HandleToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp tokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}

	if resp.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", resp.TokenType)
	}
	if resp.IDToken == "" {
		t.Fatal("id_token is empty")
	}
	if resp.AccessToken == "" {
		t.Fatal("access_token is empty")
	}

	// Decode the JWT claims manually. The indigo library registers a custom
	// "ES256" signing method (signingMethodAtproto) that replaces the standard
	// jwt ECDSA verifier — its Verify() expects indigo's own key type, not
	// *ecdsa.PublicKey. We verify the claims structurally here; the JWKS test
	// confirms the public key is correctly served for real RPs to verify.
	claims := decodeJWTClaims(t, resp.IDToken)

	if claims["iss"] != "https://provider.example.com" {
		t.Errorf("iss = %v", claims["iss"])
	}
	if claims["sub"] != "did:plc:testuser123" {
		t.Errorf("sub = %v", claims["sub"])
	}
	if claims["aud"] != "test-client" {
		t.Errorf("aud = %v", claims["aud"])
	}
	if claims["nonce"] != "test-nonce" {
		t.Errorf("nonce = %v", claims["nonce"])
	}
	if claims["preferred_username"] != "alice.bsky.social" {
		t.Errorf("preferred_username = %v", claims["preferred_username"])
	}
	if claims["atproto_pds"] != "https://pds.example.com" {
		t.Errorf("atproto_pds = %v", claims["atproto_pds"])
	}
}

func TestToken_CodeIsSingleUse(t *testing.T) {
	p := testProvider(t)

	insertCodeGrant(p, &codeGrant{
		Code:        "single-use-code",
		ClientID:    "test-client",
		RedirectURI: "https://rp.example.com/callback",
		DID:         "did:plc:testuser123",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"single-use-code"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}

	// First exchange succeeds.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader(form.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	p.HandleToken(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first exchange: expected 200, got %d", rec1.Code)
	}

	// Second exchange with same code fails.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	p.HandleToken(rec2, req2)

	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("second exchange: expected 400, got %d", rec2.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec2.Body).Decode(&errResp)
	if errResp["error"] != "invalid_grant" {
		t.Errorf("error = %q, want invalid_grant", errResp["error"])
	}
}

func TestToken_ExpiredCode(t *testing.T) {
	p := testProvider(t)

	insertCodeGrant(p, &codeGrant{
		Code:        "expired-code",
		ClientID:    "test-client",
		RedirectURI: "https://rp.example.com/callback",
		DID:         "did:plc:testuser123",
		ExpiresAt:   time.Now().Add(-1 * time.Minute), // already expired
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"expired-code"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	p.HandleToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "invalid_grant" {
		t.Errorf("error = %q, want invalid_grant", errResp["error"])
	}
}

func TestToken_WrongClientSecret(t *testing.T) {
	p := testProvider(t)

	insertCodeGrant(p, &codeGrant{
		Code:        "secret-test-code",
		ClientID:    "test-client",
		RedirectURI: "https://rp.example.com/callback",
		DID:         "did:plc:testuser123",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"secret-test-code"},
		"client_id":     {"test-client"},
		"client_secret": {"wrong-secret"},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	p.HandleToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "invalid_client" {
		t.Errorf("error = %q, want invalid_client", errResp["error"])
	}
}

// --- Utility functions ---

func TestGenerateRandomHex_Length(t *testing.T) {
	hex := generateRandomHex()

	// 32 random bytes → 64 hex characters.
	if len(hex) != 64 {
		t.Fatalf("expected 64-char hex string, got %d chars: %q", len(hex), hex)
	}
}

func TestGenerateRandomHex_Unique(t *testing.T) {
	a := generateRandomHex()
	b := generateRandomHex()
	if a == b {
		t.Fatal("two calls produced identical values — extremely unlikely with crypto/rand")
	}
}

func TestExtractStateFromURL(t *testing.T) {
	state, err := extractStateFromURL("https://pds.example.com/auth?state=abc123&code=xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "abc123" {
		t.Fatalf("state = %q, want abc123", state)
	}
}

func TestExtractStateFromURL_Missing(t *testing.T) {
	_, err := extractStateFromURL("https://pds.example.com/auth?code=xyz")
	if err == nil {
		t.Fatal("expected error for missing state")
	}
}

func TestBuildRedirectURL(t *testing.T) {
	got := buildRedirectURL("https://rp.example.com/callback", "code-abc", "state-xyz")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	if u.Query().Get("code") != "code-abc" {
		t.Errorf("code = %q", u.Query().Get("code"))
	}
	if u.Query().Get("state") != "state-xyz" {
		t.Errorf("state = %q", u.Query().Get("state"))
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		s, word string
		want    bool
	}{
		{"openid profile", "openid", true},
		{"profile openid", "openid", true},
		{"openid", "openid", true},
		{"openid-connect", "openid", false},
		{"profile email", "openid", false},
		{"", "openid", false},
	}

	for _, tt := range tests {
		got := containsWord(tt.s, tt.word)
		if got != tt.want {
			t.Errorf("containsWord(%q, %q) = %v, want %v", tt.s, tt.word, got, tt.want)
		}
	}
}

func TestPadToN(t *testing.T) {
	// Shorter than n → left-padded with zeroes.
	result := padToN([]byte{0x01, 0x02}, 4)
	if len(result) != 4 {
		t.Fatalf("expected length 4, got %d", len(result))
	}
	if result[0] != 0 || result[1] != 0 || result[2] != 1 || result[3] != 2 {
		t.Errorf("unexpected padding: %v", result)
	}

	// Already correct length → returned as-is.
	input := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	result = padToN(input, 4)
	if len(result) != 4 || result[0] != 0xAA {
		t.Errorf("expected identity for matching length: %v", result)
	}
}
