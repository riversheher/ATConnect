# Tests

Test tracking for the ATConnect project. Organizes all unit, integration, and
manual test cases with expected results.

---

## Test Runner

```bash
# Run all tests
go test ./... -v

# Run only middleware tests
go test ./internal/middleware/ -v

# Run only observability tests
go test ./internal/observability/ -v

# Run only SQLite store tests
go test ./pkg/store/sqlite/ -v

# Run only OIDC provider tests
go test ./internal/oidc/ -v

# Run a specific test by name
go test ./internal/middleware/ -v -run TestRecovery_CatchesPanic
```

---

## What Is Not Tested (and Why)

| Area | Reason | When to Add |
|------|--------|-------------|
| Prometheus metric increments | Requires importing `prometheus/testutil` (non-stdlib). Better suited for integration tests with a real registry. | Phase 3 conformance tests or dedicated metrics integration test. |
| `slog` output format verification (JSON vs text structure) | `InitLogger` writes to `os.Stderr`; testing output format requires either dependency injection of `io.Writer` or stderr capture. Low value for the complexity. | If format bugs surface, refactor `InitLogger` to accept `io.Writer`. |
| Concurrent middleware safety | Middleware is stateless (no shared mutable state). The `responseWriter` wrapper is per-request. Race conditions are unlikely. | Add `-race` flag to CI pipeline (Phase 5, step 5.6). |
| `generateID()` randomness quality | Uses `crypto/rand`; testing randomness properties is not meaningful in a unit test. | N/A — trust stdlib. |
| CORS middleware | Not yet implemented (deferred to Phase 5). | Phase 5, step 5.x. |
| Concurrent SQLite access | `MaxOpenConns(1)` serialises all DB access. No concurrent read/write races possible with current config. | When connection pooling is relaxed, add `-race` tests with parallel goroutines. |
| Schema migration / upgrade paths | No migration framework exists yet (SC-4). | Phase 5, when schema versioning is implemented. |
| Expired auth request garbage collection | No cleanup routine exists yet (SC-5). | When a GC routine is added. |
| JWT signature verification in OIDC tests | The indigo library registers a custom `signingMethodAtproto` for `"ES256"` that overrides `jwt.SigningMethodES256`, making `jwt.Parse` with `*ecdsa.PublicKey` fail during `Verify()`. OIDC tests decode JWT claims via base64 instead. Real RPs verify signatures externally using the JWKS endpoint. | If indigo exposes a verification helper, or if a test-only signing method reset is feasible. |
| OIDC full auth flow (authorize → ATProto → callback → token) | Requires a live ATProto PDS or a mock of indigo's `ClientApp`. The ATProto OAuth handshake (DPoP, PAR) is complex to stub. | When an ATProto mock/test server is available, or as a manual integration test. |

---

# Manual Testing

### Prerequisites

```bash
# Build and start the server
go run ./cmd/server -config config.example.yaml
```

The server starts on `:8080` by default (configurable via `server.listen_address` in config).

### 1. Liveness Probe

```bash
curl -s localhost:8080/livez | jq .
```

**Expected:**
```json
{
  "status": "ok"
}
```

**Verify:** Status code is `200 OK`.

### 2. Readiness Probe

```bash
curl -s -o /dev/null -w "%{http_code}" localhost:8080/readyz
curl -s localhost:8080/readyz | jq .
```

**Expected:**
```json
{
  "status": "ready",
  "checks": {
    "store": "ok"
  }
}
```

**Verify:** Status code is `200 OK`. With the in-memory store, Ping always succeeds.

### 3. Request ID Generation

```bash
curl -si localhost:8080/livez | grep -i x-request-id
```

**Expected:** `X-Request-ID: <32-char hex string>` (e.g., `X-Request-ID: a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6`).

### 4. Request ID Forwarding

```bash
curl -si -H "X-Request-ID: my-trace-123" localhost:8080/livez | grep -i x-request-id
```

**Expected:** `X-Request-ID: my-trace-123` — the upstream ID is preserved.

### 5. Prometheus Metrics Endpoint

```bash
curl -s localhost:8080/metrics | grep -E "^(http_requests_total|http_request_duration|panics_recovered)"
```

**Expected:** Prometheus metric lines appear. After making a few requests to `/livez`, you should see counter values:

```
http_requests_total{method="GET",path="/livez",status="200"} 2
```

### 6. Structured Log Output

After making requests, check the server's stderr output (the terminal where it's running).

**Expected log entry (text format):**
```
time=2026-04-04T... level=INFO msg="http request" method=GET path=/livez status=200 duration_ms=0 bytes=16 request_id=... remote_addr=127.0.0.1:...
```

**Verify:**
- `level=INFO` for 2xx responses.
- `request_id` is present and matches the `X-Request-ID` response header.
- `duration_ms`, `bytes`, `method`, `path`, `status` are all populated.

### 7. 404 Logging Level

```bash
curl -s localhost:8080/nonexistent
```

**Expected log level:** `WARN` (4xx status).

### 8. OAuth Flow (CLI)

```bash
go run ./cmd/cli -config config.example.yaml
```

**Expected:**
1. Prompts for ATProto handle.
2. Opens browser to PDS auth page.
3. A `WARN` log line appears: `msg="auth server request failed" request=PAR statusCode=400` — this is the expected DPoP nonce negotiation (see implementation_plan.md Phase 1 Lessons Learned #3).
4. After authorizing, prints DID, Session ID, and PDS Host.

---

## Manual Testing — SQLite Store Integration

These tests verify the SQLite store works correctly as part of the running server.

### Prerequisites

```bash
mkdir -p ./data
go run ./cmd/server -config config.example.yaml
```

Then edit `config.example.yaml` (or set env vars) to use SQLite:

```bash
export ATCONNECT_STORE_BACKEND=sqlite
export ATCONNECT_STORE_SQLITE_PATH=./data/test.db
go run ./cmd/server
```

### 9. SQLite Store — Server Starts Successfully

Verify the server boots with the SQLite backend and the log line shows `store_backend=sqlite`.

```bash
# Check for startup log
# Expected: level=INFO msg="atconnect server starting" listen_address=:8080 store_backend=sqlite
```

**Verify:** Server starts without errors. The database file (`./data/test.db`) is created on disk.

### 10. SQLite Store — Readiness Check

```bash
curl -s localhost:8080/readyz | jq .
```

**Expected:**
```json
{
  "status": "ready",
  "checks": {
    "store": "ok"
  }
}
```

**Verify:** `Ping()` succeeds against the SQLite database.

### 11. SQLite Store — Data Persists Across Restarts

1. Start the server with SQLite backend.
2. Run the CLI OAuth flow (`go run ./cmd/cli`) to create a session.
3. Stop the server (Ctrl+C).
4. Restart the server.
5. Verify the database file still exists and the server starts cleanly.

```bash
ls -la ./data/test.db
```

**Verify:** The database file persists and the server restarts without schema errors. (Session data is preserved in the file, though there is currently no endpoint to query it directly.)

### 12. SQLite Store — WAL Mode Active

After starting the server with SQLite, verify WAL journal files are created:

```bash
ls -la ./data/test.db*
```

**Expected:** You should see `test.db`, `test.db-wal`, and `test.db-shm` files — confirming WAL mode is active.

### 13. SQLite Store — Database Inspection

If `sqlite3` CLI is available, you can inspect the database directly:

```bash
sqlite3 ./data/test.db ".tables"
```

**Expected:**
```
oauth_auth_requests  oauth_sessions       oidc_clients         oidc_keys
```

```bash
sqlite3 ./data/test.db "PRAGMA journal_mode;"
```

**Expected:** `wal`

---

## Manual Testing — OIDC Integration

These tests verify the OIDC provider endpoints work as part of a running server. They require a config file with at least one registered OIDC client.

### Prerequisites

Create a config file (or use `config.example.yaml`) with an OIDC client:

```yaml
oidc:
  issuer_url: "http://localhost:8080"
  clients:
    - client_id: "manual-test"
      client_secret: "manual-test-secret"
      redirect_uris:
        - "http://localhost:9090/callback"
      name: "Manual Test RP"
```

Start the server:

```bash
go run ./cmd/server -config config.example.yaml
```

### 14. OIDC Discovery Document

```bash
curl -s localhost:8080/.well-known/openid-configuration | jq .
```

**Expected:**
```json
{
  "issuer": "http://localhost:8080",
  "authorization_endpoint": "http://localhost:8080/authorize",
  "token_endpoint": "http://localhost:8080/token",
  "jwks_uri": "http://localhost:8080/jwks",
  "scopes_supported": ["openid"],
  "response_types_supported": ["code"],
  "id_token_signing_alg_values_supported": ["ES256"],
  "subject_types_supported": ["public"],
  "claims_supported": ["iss","sub","aud","exp","iat","nonce","preferred_username","atproto_pds"]
}
```

**Verify:** All endpoints use the correct issuer URL prefix. `ES256` is listed as the signing algorithm.

### 15. JWKS Endpoint

```bash
curl -s localhost:8080/jwks | jq .
```

**Expected:**
```json
{
  "keys": [
    {
      "kty": "EC",
      "crv": "P-256",
      "x": "<base64url>",
      "y": "<base64url>",
      "kid": "atconnect-signing-key-1",
      "use": "sig",
      "alg": "ES256"
    }
  ]
}
```

**Verify:** One key returned. `kid` matches the default key ID. `x` and `y` are present. Calling the endpoint again returns the same key (key persistence).

### 16. Authorize — Missing Parameters

```bash
# No parameters at all
curl -si localhost:8080/authorize
```

**Expected:** Status `400` with JSON `{"error": "invalid_request", "error_description": "client_id is required"}`.

```bash
# Unknown client_id
curl -si "localhost:8080/authorize?client_id=nonexistent"
```

**Expected:** Status `400` with `"unknown client_id"`.

```bash
# Valid client but unregistered redirect_uri
curl -si "localhost:8080/authorize?client_id=manual-test&redirect_uri=https://evil.example.com/callback"
```

**Expected:** Status `400` with `"redirect_uri is not registered for this client"`.

### 17. Authorize — Login Form

```bash
curl -s "localhost:8080/authorize?client_id=manual-test&redirect_uri=http://localhost:9090/callback&response_type=code&scope=openid"
```

**Expected:** HTML page with a form containing a `login_hint` input field and hidden fields preserving the authorization parameters. This is the page a user would see in a browser.

### 18. Authorize — Full Flow (Browser)

Open in a browser:

```
http://localhost:8080/authorize?client_id=manual-test&redirect_uri=http://localhost:9090/callback&response_type=code&scope=openid&state=test-state&nonce=test-nonce&login_hint=<your-handle>.bsky.social
```

**Expected:**
1. Browser redirects to the ATProto PDS authorization page.
2. After authorizing, the PDS redirects back to the server's `/callback`.
3. The server issues an authorization code and redirects to `http://localhost:9090/callback?code=<hex>&state=test-state`.
4. (The redirect will fail since no RP is running on port 9090 — inspect the browser URL bar for the `code` and `state` parameters.)

### 19. Token Exchange (with code from step 18)

Using the `code` from the browser URL bar:

```bash
curl -s -X POST localhost:8080/token \
  -d "grant_type=authorization_code" \
  -d "code=<paste-code-here>" \
  -d "redirect_uri=http://localhost:9090/callback" \
  -d "client_id=manual-test" \
  -d "client_secret=manual-test-secret" | jq .
```

**Expected:**
```json
{
  "access_token": "<64-char hex>",
  "token_type": "Bearer",
  "expires_in": 3600,
  "id_token": "<JWT>"
}
```

**Verify:**
- `token_type` is `"Bearer"`.
- `expires_in` is `3600` (1 hour).
- `id_token` is a three-part JWT (split by `.`).

### 20. Decode the ID Token

Decode the JWT payload (middle segment):

```bash
echo "<id_token>" | cut -d. -f2 | base64 -d 2>/dev/null | jq .
```

**Expected claims:**
```json
{
  "iss": "http://localhost:8080",
  "sub": "did:plc:<your-did>",
  "aud": "manual-test",
  "exp": <unix-timestamp>,
  "iat": <unix-timestamp>,
  "nonce": "test-nonce",
  "preferred_username": "<your-handle>.bsky.social",
  "atproto_pds": "https://<your-pds-host>"
}
```

**Verify:**
- `iss` matches the server's issuer URL.
- `sub` is your ATProto DID.
- `aud` matches the `client_id` used in the authorization request.
- `nonce` matches the nonce from step 18.
- `preferred_username` is your handle.
- `exp` is approximately 1 hour after `iat`.

### 21. Token Exchange — Code Reuse Rejected

Re-use the same code from step 19:

```bash
curl -s -X POST localhost:8080/token \
  -d "grant_type=authorization_code" \
  -d "code=<same-code-again>" \
  -d "client_id=manual-test" \
  -d "client_secret=manual-test-secret" | jq .
```

**Expected:** `{"error": "invalid_grant", "error_description": "unknown or already-used authorization code"}`.

### 22. Token Exchange — Wrong Secret

```bash
curl -s -X POST localhost:8080/token \
  -d "grant_type=authorization_code" \
  -d "code=any-code" \
  -d "client_id=manual-test" \
  -d "client_secret=wrong" | jq .
```

**Expected:** `{"error": "invalid_client", "error_description": "invalid client credentials"}` (or `"invalid_grant"` if the code is also invalid — the code is validated first).

---


## Running in CI

```bash
# All tests with race detector
go test -race -count=1 ./...

# With coverage summary
go test -cover ./internal/middleware/ ./internal/observability/ ./pkg/store/sqlite/
```

Target: all tests pass, no race conditions detected. Coverage tracking is
informational at this stage — not gated.

---

# Unit Tests

### Middleware — `internal/middleware/middleware_test.go`

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|-----------------|
| 1 | `TestRequestID_GeneratesNewID` | When no `X-Request-ID` header is present, the middleware generates a 32-char hex ID, stores it in context, and sets it on the response header. | Context and response header contain matching 32-char hex string. |
| 2 | `TestRequestID_PropagatesExisting` | When an `X-Request-ID` header is present on the request, the middleware propagates it unchanged to context and response. | Context and response header both contain the original value. |
| 3 | `TestLogging_CapturesStatusAndBytes` | The logging middleware correctly captures the status code and response body size from the handler, and emits a structured log entry with those values. | JSON log entry contains `status: 201`, `bytes: 11`, `msg: "http request"`. |
| 4 | `TestLogging_LevelByStatus` | Log level is determined by HTTP status: 2xx/3xx → INFO, 4xx → WARN, 5xx → ERROR. | Status 200 → INFO, 301 → INFO, 404 → WARN, 500 → ERROR. |
| 5 | `TestLogging_NilMetricsDoesNotPanic` | Passing `nil` for the Prometheus metrics parameter does not cause a panic. | Handler completes with 200, no panic. |
| 6 | `TestRecovery_CatchesPanic` | When a handler panics, the recovery middleware returns 500 with a JSON error body and does not leak panic details. | Status 500, body contains `{"error": "internal_error", ...}`. |
| 7 | `TestRecovery_PassthroughOnNoPanic` | When a handler completes normally, the recovery middleware passes the response through unmodified. | Status 200, body `"ok"`. |
| 8 | `TestRecovery_LogsStackTrace` | When a panic is recovered, the log entry includes `msg: "panic recovered"` and a goroutine stack trace. | JSON log entry has `msg` = `"panic recovered"` and `stack` contains `"goroutine"`. |
| 9 | `TestFullStack_RequestIDAndRecovery` | The full middleware chain (recovery → requestid → logging) works end-to-end over a real `httptest.Server`. Normal requests get `X-Request-ID`, panicking handlers return 500. | `/ok` → 200 + `X-Request-ID` header present; `/panic` → 500. |

### Health Checks — `internal/observability/health_test.go`

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|-----------------|
| 10 | `TestLivez_AlwaysReturns200` | The liveness endpoint returns 200 unconditionally with `{"status": "ok"}`. | Status 200, JSON body `status` = `"ok"`. |
| 11 | `TestReadyz_HealthyStore` | When `Store.Ping()` succeeds, the readiness endpoint returns 200 with `{"status": "ready"}`. | Status 200, JSON body `status` = `"ready"`. |
| 12 | `TestReadyz_UnhealthyStore` | When `Store.Ping()` returns an error, the readiness endpoint returns 503 with `{"status": "not ready"}` and a checks object showing the store error. | Status 503, JSON body `status` = `"not ready"`, `checks.store` contains error text. |

### Logger Initialisation — `internal/observability/logger_test.go`

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|-----------------|
| 13 | `TestInitLogger_ParsesLevels` | All level strings (`debug`, `info`, `warn`, `error`) map to the correct `slog.Level`. Unknown and empty strings default to Info. | Each sub-test confirms `logger.Enabled()` at the expected level. |
| 14 | `TestInitLogger_JSONFormat` | `Format: "json"` produces a non-nil logger and sets `slog.Default()`. | Logger is non-nil, `slog.Default()` is non-nil. |
| 15 | `TestInitLogger_TextFormat` | `Format: "text"` produces a non-nil logger. | Logger is non-nil. |

---

## SQLite Store — `pkg/store/sqlite/sqlite_test.go`

Each test creates a fresh temporary database via `newTestStore(t)`. The database is automatically cleaned up when the test finishes.

### Lifecycle & Setup

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 16 | `TestStore_ImplementsStoreInterface` | `*sqlite.Store` satisfies the `store.Store` interface. | Compiles and assigns without error. |
| 17 | `TestNew_EmptyPath` | Passing an empty string for `dbPath` returns an error. | Error returned. |
| 18 | `TestNew_InvalidPath` | Passing a non-existent directory path returns an error. | Error returned. |
| 19 | `TestNew_CreatesFile` | `New()` creates the SQLite database file on disk. | File exists after call. |
| 20 | `TestPing` | `Ping()` succeeds on a freshly created store. | No error. |
| 21 | `TestClose_ThenPingFails` | After `Close()`, `Ping()` returns an error. | Error returned. |

### Sessions

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 22 | `TestSession_SaveAndGet` | Round-trip save and retrieve of a session. | All fields match (DID, session ID, access token). |
| 23 | `TestSession_GetNotFound` | Getting a non-existent session returns a "not found" error. | Error containing "not found". |
| 24 | `TestSession_UpsertOverwrites` | Saving a session with the same key overwrites the previous data (upsert). | Updated access token is returned on get. |
| 25 | `TestSession_MultipleSessions` | Two sessions for the same DID with different session IDs coexist independently. | Each returns its own access token. |
| 26 | `TestSession_Delete` | Deleting an existing session makes it unretrievable. | Get returns "not found" after delete. |
| 27 | `TestSession_DeleteNonExistent` | Deleting a session that doesn't exist does not error. | No error. |

### Auth Requests

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 28 | `TestAuthRequest_SaveAndGet` | Round-trip save and retrieve of an auth request. | State and PKCE verifier match. |
| 29 | `TestAuthRequest_GetNotFound` | Getting a non-existent auth request returns a "not found" error. | Error containing "not found". |
| 30 | `TestAuthRequest_CreateOnly_RejectsDuplicate` | Saving an auth request with a duplicate state token errors (create-only semantics). | Error containing "already exists". |
| 31 | `TestAuthRequest_Delete` | Deleting an existing auth request makes it unretrievable. | Get returns error after delete. |
| 32 | `TestAuthRequest_DeleteNonExistent` | Deleting a non-existent auth request does not error. | No error. |
| 33 | `TestAuthRequest_DeleteThenRecreate` | After deletion, the same state token can be reused for a new request. | Second save succeeds. |

### Keys

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 34 | `TestKey_SaveAndGet` | Round-trip save and retrieve of a signing key. | Key data matches. |
| 35 | `TestKey_GetNotFound` | Getting a non-existent key returns a "not found" error. | Error containing "not found". |
| 36 | `TestKey_UpsertOverwrites` | Saving a key with the same kid overwrites the previous value. | Updated data returned. |
| 37 | `TestKey_ListEmpty` | Listing keys on an empty store returns an empty slice. | Length 0. |
| 38 | `TestKey_ListMultiple` | Listing keys returns all kids in alphabetical order. | `[kid-a, kid-b, kid-c]`. |

### OIDC Clients

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 39 | `TestClient_SaveAndGet` | Round-trip save and retrieve of an OIDC client. | Client ID, name, redirect URIs match. |
| 40 | `TestClient_GetNotFound` | Getting a non-existent client returns a "not found" error. | Error containing "not found". |
| 41 | `TestClient_UpsertOverwrites` | Saving a client with the same client_id overwrites the previous data. | Updated name returned. |
| 42 | `TestClient_ListEmpty` | Listing clients on an empty store returns an empty slice. | Length 0. |
| 43 | `TestClient_ListMultiple` | Listing clients returns all clients ordered by client_id. | `[c-alpha, c-beta, c-gamma]`. |
| 44 | `TestClient_Delete` | Deleting an existing client makes it unretrievable. | Get returns error after delete. |
| 45 | `TestClient_DeleteNonExistent` | Deleting a non-existent client does not error. | No error. |

### Cross-Domain

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 46 | `TestCrossDomain_IndependentTables` | Deleting data in one domain does not affect data in other domains. | Auth request, key, and client survive session deletion. |

---

## OIDC Provider — `internal/oidc/oidc_test.go`

Each test creates a fresh `Provider` with an in-memory store, a real EC P-256 signing key (via `keys.NewManager`), and a pre-registered test client. The ATProto OAuth client is nil — tests that need token exchange insert `codeGrant` entries directly, bypassing the ATProto leg.

**Note:** JWT claims are verified by base64-decoding the payload rather than using `jwt.Parse` with signature verification. This is because the indigo library registers a custom `signingMethodAtproto` for "ES256" that replaces the standard `jwt.SigningMethodECDSA`. The custom method's `Verify()` expects indigo's key wrapper, not a bare `*ecdsa.PublicKey`. See "What Is Not Tested" for details.

### Discovery & JWKS

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 47 | `TestDiscovery_ReturnsCorrectMetadata` | `/.well-known/openid-configuration` returns valid JSON with correct issuer, endpoints, signing algorithms. | Status 200, `issuer` matches, `authorization_endpoint` / `token_endpoint` / `jwks_uri` populated, `ES256` in signing algs. |
| 48 | `TestJWKS_ReturnsValidJWKSet` | `/jwks` returns a JWK Set with one EC P-256 key containing valid base64url-encoded 32-byte coordinates. | Status 200, one key with `kty=EC`, `crv=P-256`, `alg=ES256`, `use=sig`, `kid` matches key manager, `x` and `y` decode to 32 bytes. |

### Authorize Endpoint Validation

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 49 | `TestAuthorize_MissingClientID` | `/authorize` with no `client_id` returns an error (not a redirect, since the RP is unknown). | Status 400, `error=invalid_request`. |
| 50 | `TestAuthorize_UnknownClientID` | `/authorize` with a non-registered `client_id` returns 400. | Status 400. |
| 51 | `TestAuthorize_UnregisteredRedirectURI` | `/authorize` with a `redirect_uri` not registered for the client returns 400 (must not redirect to untrusted URIs). | Status 400, `error=invalid_request`. |
| 52 | `TestAuthorize_UnsupportedResponseType` | `/authorize` with `response_type=token` (instead of `code`) redirects back to the RP with an error. | Status 302, redirect URL contains `error=unsupported_response_type`. |
| 53 | `TestAuthorize_MissingLoginHint_ShowsForm` | When all params are valid but `login_hint` is omitted, the provider renders an HTML login form. | Status 200, `Content-Type: text/html`, body contains `login_hint` input. |

### Token Endpoint

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 54 | `TestToken_ValidExchange` | Full happy-path code exchange: inserts a `codeGrant`, calls `POST /token`, verifies the returned JWT ID token contains correct claims (iss, sub, aud, nonce, preferred_username, atproto_pds). | Status 200, `token_type=Bearer`, `id_token` decodes to JWT with expected claims, `access_token` non-empty. |
| 55 | `TestToken_CodeIsSingleUse` | Authorization codes are deleted after first use. A second exchange with the same code fails. | First call → 200. Second call → 400, `error=invalid_grant`. |
| 56 | `TestToken_ExpiredCode` | An expired authorization code is rejected. | Status 400, `error=invalid_grant`. |
| 57 | `TestToken_WrongClientSecret` | Token exchange with an incorrect `client_secret` is rejected. | Status 401, `error=invalid_client`. |

### Utility Functions

| # | Test Name | What It Verifies | Expected Result |
|---|-----------|-----------------|----------------|
| 58 | `TestGenerateRandomHex_Length` | `generateRandomHex()` produces a 64-character hex string (32 random bytes). | Length 64. |
| 59 | `TestGenerateRandomHex_Unique` | Two consecutive calls to `generateRandomHex()` produce different values. | Values differ. |
| 60 | `TestExtractStateFromURL` | Correctly extracts the `state` query parameter from a URL. | Returns `"abc123"`. |
| 61 | `TestExtractStateFromURL_Missing` | Returns an error when the URL has no `state` parameter. | Error returned. |
| 62 | `TestBuildRedirectURL` | Constructs a redirect URL with `code` and `state` query parameters. | Parsed URL contains correct `code` and `state` values. |
| 63 | `TestContainsWord` | Space-delimited word matching: matches whole words, rejects substrings, handles edge cases. | `"openid profile"→true`, `"openid-connect"→false`, `""→false`. |
| 64 | `TestPadToN` | Left-pads short byte slices with zeroes to the target length; returns as-is when already correct. | `[0x01,0x02]` padded to 4 → `[0,0,1,2]`; `[AA,BB,CC,DD]` → identity. |

---