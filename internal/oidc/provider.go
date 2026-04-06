// Package oidc implements the OpenID Connect provider layer for ATConnect.
//
// The Provider bridges ATProto OAuth authentication to OIDC: it accepts
// authorization requests from relying parties, authenticates users via
// ATProto OAuth (through indigo's ClientApp), and issues OIDC ID Tokens
// containing the user's DID and handle.
//
// # Authorization Code Flow
//
// The MVP implements the OIDC Authorization Code Flow:
//
//  1. RP redirects user to GET /authorize with client_id, redirect_uri,
//     state, nonce, response_type=code, scope=openid, and optionally
//     login_hint (ATProto handle).
//  2. If login_hint is missing, a simple HTML form prompts for the handle.
//  3. The provider initiates ATProto OAuth against the user's PDS.
//  4. On successful ATProto auth callback, an authorization code is generated.
//  5. The user is redirected back to the RP's redirect_uri with the code.
//  6. The RP exchanges the code at POST /token for an ID Token (JWT).
package oidc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/riversheher/atconnect/internal/keys"
	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/pkg/store"
)

// authCodeTTL is the lifetime of an authorization code.
// Per OIDC Core §3.1.3.7, the code should have a short lifetime
// (recommended maximum of 10 minutes).
const authCodeTTL = 10 * time.Minute

// pendingAuthorization holds the relying party's authorization parameters
// while the user completes ATProto authentication. Keyed by ATProto state.
type pendingAuthorization struct {
	ClientID    string
	RedirectURI string
	State       string // RP's state parameter (echoed back)
	Nonce       string
	LoginHint   string // ATProto handle (from login_hint param)
	CreatedAt   time.Time
}

// codeGrant represents the data associated with an issued authorization
// code — the user identity, RP parameters, and expiry. The Code field is
// the opaque string sent to the RP; the remaining fields are used when
// the RP exchanges the code at /token.
type codeGrant struct {
	Code        string    // Opaque authorization code string
	ClientID    string    // RP that initiated the authorization
	RedirectURI string    // RP redirect URI (must match at /token)
	Nonce       string    // OIDC nonce (echoed into ID Token)
	DID         string    // ATProto DID (from successful auth)
	Handle      string    // ATProto handle (from login_hint)
	PDS         string    // PDS host URL
	ExpiresAt   time.Time // Code validity deadline
}

// Provider is the OIDC provider that bridges ATProto OAuth to OIDC.
type Provider struct {
	issuerURL   string
	keyManager  *keys.Manager
	oauthClient *oauth.Client
	store       store.Store
	logger      *slog.Logger

	// In-memory state for the authorization code flow.
	// These are ephemeral and do not survive server restarts.
	// See oidc_concerns.md OC-1 for discussion.
	pendingMu       sync.RWMutex
	pendingRequests map[string]*pendingAuthorization // ATProto state → pending request

	codeGrantsMu sync.RWMutex
	codeGrants   map[string]*codeGrant // code string → grant data
}

// NewProvider creates an OIDC provider.
//
// Parameters:
//   - issuerURL: the public-facing URL of this provider (no trailing slash)
//   - km: key manager with a loaded signing key
//   - oauthClient: ATProto OAuth client for user authentication
//   - s: data store for client lookups
func NewProvider(issuerURL string, km *keys.Manager, oauthClient *oauth.Client, s store.Store) *Provider {
	return &Provider{
		issuerURL:       issuerURL,
		keyManager:      km,
		oauthClient:     oauthClient,
		store:           s,
		logger:          slog.Default(),
		pendingRequests: make(map[string]*pendingAuthorization),
		codeGrants:      make(map[string]*codeGrant),
	}
}

// HandleAuthorize handles GET /authorize — the OIDC authorization endpoint.
//
// Required query parameters:
//   - client_id: registered OIDC client identifier
//   - redirect_uri: must match a registered redirect URI for the client
//   - response_type: must be "code"
//   - scope: must include "openid"
//
// Optional parameters:
//   - state: opaque value echoed back to the RP (recommended)
//   - nonce: binds the ID token to the client session
//   - login_hint: ATProto handle — if omitted, a login form is shown
func (p *Provider) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	scope := q.Get("scope")
	state := q.Get("state")
	nonce := q.Get("nonce")
	loginHint := q.Get("login_hint")

	// --- Validate client_id and redirect_uri first (must not redirect on failure) ---

	if clientID == "" {
		p.renderError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	client, err := p.store.GetClient(r.Context(), clientID)
	if err != nil {
		p.logger.Warn("authorize: unknown client_id", "client_id", clientID, "error", err)
		p.renderError(w, http.StatusBadRequest, "invalid_request", "unknown client_id")
		return
	}

	if redirectURI == "" {
		p.renderError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	if !containsString(client.RedirectURIs, redirectURI) {
		p.logger.Warn("authorize: redirect_uri not registered",
			"client_id", clientID, "redirect_uri", redirectURI,
			"registered", client.RedirectURIs)
		p.renderError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered for this client")
		return
	}

	// --- From here, errors can be redirected back to the RP ---

	if responseType != "code" {
		p.redirectWithError(w, r, redirectURI, state, "unsupported_response_type",
			"only response_type=code is supported")
		return
	}

	if scope != "openid" && !containsWord(scope, "openid") {
		p.redirectWithError(w, r, redirectURI, state, "invalid_scope",
			"scope must include openid")
		return
	}

	// --- If no login_hint, show a login form ---

	if loginHint == "" {
		p.renderLoginForm(w, r)
		return
	}

	// --- Start ATProto OAuth flow ---

	authURL, err := p.oauthClient.StartFlow(r.Context(), loginHint)
	if err != nil {
		p.logger.Error("authorize: failed to start ATProto OAuth flow",
			"error", err, "login_hint", loginHint)
		p.redirectWithError(w, r, redirectURI, state, "server_error",
			"failed to start authentication")
		return
	}

	// Extract ATProto state from the auth URL to link callback to this request.
	atprotoState, err := extractStateFromURL(authURL)
	if err != nil {
		p.logger.Error("authorize: failed to extract state from auth URL",
			"error", err, "auth_url", authURL)
		p.redirectWithError(w, r, redirectURI, state, "server_error",
			"internal error during authentication setup")
		return
	}

	// Store the pending OIDC request keyed by ATProto state.
	pending := &pendingAuthorization{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		State:       state,
		Nonce:       nonce,
		LoginHint:   loginHint,
		CreatedAt:   time.Now(),
	}

	p.pendingMu.Lock()
	p.pendingRequests[atprotoState] = pending
	p.pendingMu.Unlock()

	p.logger.Info("authorize: redirecting to ATProto auth",
		"client_id", clientID, "login_hint", loginHint)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /callback — the ATProto OAuth redirect.
//
// This replaces the simple callback handler from Phase 1. When a pending
// OIDC request is found (keyed by ATProto state), the callback completes
// the OIDC authorization code flow by redirecting to the RP. Otherwise
// it falls back to a plain success message (for non-OIDC usage).
func (p *Provider) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Process the ATProto OAuth callback.
	sess, err := p.oauthClient.HandleCallback(r.Context(), r.URL.Query())
	if err != nil {
		p.logger.Error("callback: ATProto OAuth failed", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	p.logger.Info("callback: ATProto auth successful", "did", sess.AccountDID)

	// Check for a pending OIDC authorization request.
	atprotoState := r.URL.Query().Get("state")

	p.pendingMu.Lock()
	pending, hasPending := p.pendingRequests[atprotoState]
	if hasPending {
		delete(p.pendingRequests, atprotoState)
	}
	p.pendingMu.Unlock()

	if !hasPending {
		// No OIDC request — plain ATProto callback (CLI or direct usage).
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Authentication successful!\nDID: %s\n", sess.AccountDID)
		return
	}

	// Check if pending request has expired.
	if time.Since(pending.CreatedAt) > authCodeTTL {
		p.logger.Warn("callback: pending OIDC request expired",
			"client_id", pending.ClientID, "age", time.Since(pending.CreatedAt))
		p.redirectWithError(w, r, pending.RedirectURI, pending.State,
			"server_error", "authorization request expired")
		return
	}

	// Generate authorization code.
	code := generateRandomHex()

	grant := &codeGrant{
		Code:        code,
		ClientID:    pending.ClientID,
		RedirectURI: pending.RedirectURI,
		Nonce:       pending.Nonce,
		DID:         string(sess.AccountDID),
		Handle:      pending.LoginHint,
		PDS:         sess.HostURL,
		ExpiresAt:   time.Now().Add(authCodeTTL),
	}

	p.codeGrantsMu.Lock()
	p.codeGrants[code] = grant
	p.codeGrantsMu.Unlock()

	p.logger.Info("callback: issuing authorization code",
		"client_id", pending.ClientID, "did", sess.AccountDID)

	// Redirect back to the RP with the authorization code.
	redirectURL := buildRedirectURL(pending.RedirectURI, code, pending.State)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleToken handles POST /token — the OIDC token endpoint.
//
// Accepts application/x-www-form-urlencoded with:
//   - grant_type: must be "authorization_code"
//   - code: the authorization code from the callback
//   - redirect_uri: must match the original authorization request
//   - client_id: the registered client identifier
//   - client_secret: the client's secret (required for confidential clients)
//
// Client credentials may also be provided via HTTP Basic Authentication.
func (p *Provider) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		p.tokenError(w, http.StatusBadRequest, "invalid_request", "failed to parse request body")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")

	// Extract client credentials (form body or Basic auth).
	clientID, clientSecret := extractClientCredentials(r)

	// --- Validate grant type ---

	if grantType != "authorization_code" {
		p.tokenError(w, http.StatusBadRequest, "unsupported_grant_type",
			"only grant_type=authorization_code is supported")
		return
	}

	if code == "" {
		p.tokenError(w, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}

	if clientID == "" {
		p.tokenError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	// --- Look up and consume the authorization code ---

	p.codeGrantsMu.Lock()
	grant, ok := p.codeGrants[code]
	if ok {
		delete(p.codeGrants, code) // Single-use: delete immediately.
	}
	p.codeGrantsMu.Unlock()

	if !ok {
		p.tokenError(w, http.StatusBadRequest, "invalid_grant", "unknown or already-used authorization code")
		return
	}

	// Check expiry.
	if time.Now().After(grant.ExpiresAt) {
		p.tokenError(w, http.StatusBadRequest, "invalid_grant", "authorization code has expired")
		return
	}

	// Validate client_id matches the code.
	if grant.ClientID != clientID {
		p.logger.Warn("token: client_id mismatch",
			"expected", grant.ClientID, "got", clientID)
		p.tokenError(w, http.StatusBadRequest, "invalid_grant", "client_id does not match authorization code")
		return
	}

	// Validate redirect_uri matches (OIDC Core §3.1.3.2).
	if redirectURI != "" && grant.RedirectURI != redirectURI {
		p.tokenError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match")
		return
	}

	// Validate client credentials.
	client, err := p.store.GetClient(r.Context(), clientID)
	if err != nil {
		p.tokenError(w, http.StatusUnauthorized, "invalid_client", "unknown client")
		return
	}

	// For confidential clients, verify the secret.
	if client.ClientSecret != "" {
		if clientSecret != client.ClientSecret {
			p.tokenError(w, http.StatusUnauthorized, "invalid_client", "invalid client credentials")
			return
		}
	}

	// --- Build and sign the ID Token ---

	idToken, err := p.buildIDToken(grant)
	if err != nil {
		p.logger.Error("token: failed to build ID token", "error", err)
		p.tokenError(w, http.StatusInternalServerError, "server_error", "failed to issue token")
		return
	}

	p.logger.Info("token: issued ID token",
		"client_id", clientID, "did", grant.DID)

	// --- Return the token response ---

	resp := tokenResponse{
		AccessToken: generateRandomHex(), // Opaque; /userinfo not yet implemented.
		TokenType:   "Bearer",
		ExpiresIn:   int(idTokenTTL.Seconds()),
		IDToken:     idToken,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(resp)
}

// tokenResponse is the JSON body returned by the token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token"`
}

// --- Error helpers ---

// renderError writes an error as an HTML page (for errors that must not
// redirect, e.g. invalid client_id or redirect_uri).
func (p *Provider) renderError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}

// redirectWithError redirects the user back to the RP with an error.
// Per OIDC Core §3.1.2.6.
func (p *Provider) redirectWithError(w http.ResponseWriter, r *http.Request,
	redirectURI, state, errCode, errDescription string) {

	u, err := url.Parse(redirectURI)
	if err != nil {
		// If redirect_uri is unparseable, fall back to a plain error.
		p.renderError(w, http.StatusBadRequest, errCode, errDescription)
		return
	}

	q := u.Query()
	q.Set("error", errCode)
	q.Set("error_description", errDescription)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

// tokenError writes a JSON error response for the token endpoint.
// Per OIDC Core §3.1.3.4.
func (p *Provider) tokenError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}

// --- Login form ---

// loginFormTemplate is a minimal HTML form shown when login_hint is not
// provided. It preserves the original authorization request parameters
// and prompts the user for their ATProto handle.
var loginFormTemplate = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Sign in — ATConnect</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
               display: flex; justify-content: center; align-items: center;
               min-height: 100vh; margin: 0; background: #f5f5f5; }
        .card { background: white; padding: 2rem; border-radius: 8px;
                box-shadow: 0 2px 4px rgba(0,0,0,0.1); max-width: 400px; width: 100%; }
        h1 { font-size: 1.5rem; margin: 0 0 1.5rem 0; }
        label { display: block; margin-bottom: 0.5rem; font-weight: 500; }
        input[type="text"] { width: 100%; padding: 0.5rem; border: 1px solid #ccc;
                             border-radius: 4px; font-size: 1rem; box-sizing: border-box; }
        button { margin-top: 1rem; padding: 0.75rem 1.5rem; background: #0066cc;
                 color: white; border: none; border-radius: 4px; font-size: 1rem;
                 cursor: pointer; width: 100%; }
        button:hover { background: #0052a3; }
        .hint { font-size: 0.85rem; color: #666; margin-top: 0.25rem; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Sign in with ATProto</h1>
        <form method="GET" action="/authorize">
            {{- range $key, $value := .Params }}
            <input type="hidden" name="{{ $key }}" value="{{ $value }}">
            {{- end }}
            <label for="login_hint">ATProto Handle</label>
            <input type="text" id="login_hint" name="login_hint"
                   placeholder="user.bsky.social" required autofocus>
            <p class="hint">Enter your Bluesky or ATProto handle to continue.</p>
            <button type="submit">Continue</button>
        </form>
    </div>
</body>
</html>`))

// renderLoginForm renders the login form, preserving all existing query
// parameters as hidden form fields.
func (p *Provider) renderLoginForm(w http.ResponseWriter, r *http.Request) {
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		if key != "login_hint" && len(values) > 0 {
			params[key] = values[0]
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	loginFormTemplate.Execute(w, struct {
		Params map[string]string
	}{Params: params})
}

// --- Utility functions ---

// generateRandomHex produces a cryptographically random 32-byte hex string.
// Used for authorization codes and opaque access tokens.
func generateRandomHex() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand never errors on Linux/macOS/Windows.
	return hex.EncodeToString(b)
}

// extractStateFromURL parses a URL string and extracts the "state" query parameter.
func extractStateFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing URL: %w", err)
	}
	state := u.Query().Get("state")
	if state == "" {
		return "", fmt.Errorf("no state parameter in URL")
	}
	return state, nil
}

// buildRedirectURL constructs the RP redirect URL with the authorization code
// and state parameter.
func buildRedirectURL(redirectURI, code, state string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		// This shouldn't happen — redirect_uri was validated earlier.
		return redirectURI
	}
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// extractClientCredentials extracts client_id and client_secret from the
// request. Checks HTTP Basic Authentication first, then falls back to
// form-encoded body parameters.
func extractClientCredentials(r *http.Request) (clientID, clientSecret string) {
	// Try HTTP Basic Auth first (RFC 6749 §2.3.1).
	if id, secret, ok := r.BasicAuth(); ok {
		return id, secret
	}
	// Fall back to form body.
	return r.FormValue("client_id"), r.FormValue("client_secret")
}

// containsString checks if a string slice contains a specific value.
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// containsWord checks if a space-separated string contains a specific word.
// Used for OIDC scope validation (scopes are space-delimited).
func containsWord(s, word string) bool {
	// Simple implementation: check if the word appears as a complete token.
	for i := 0; i < len(s); {
		// Skip leading spaces.
		for i < len(s) && s[i] == ' ' {
			i++
		}
		j := i
		for j < len(s) && s[j] != ' ' {
			j++
		}
		if s[i:j] == word {
			return true
		}
		i = j
	}
	return false
}
