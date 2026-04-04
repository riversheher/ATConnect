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

# Run a specific test by name
go test ./internal/middleware/ -v -run TestRecovery_CatchesPanic
```

---

## Unit Tests

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

## What Is Not Tested (and Why)

| Area | Reason | When to Add |
|------|--------|-------------|
| Prometheus metric increments | Requires importing `prometheus/testutil` (non-stdlib). Better suited for integration tests with a real registry. | Phase 3 conformance tests or dedicated metrics integration test. |
| `slog` output format verification (JSON vs text structure) | `InitLogger` writes to `os.Stderr`; testing output format requires either dependency injection of `io.Writer` or stderr capture. Low value for the complexity. | If format bugs surface, refactor `InitLogger` to accept `io.Writer`. |
| Concurrent middleware safety | Middleware is stateless (no shared mutable state). The `responseWriter` wrapper is per-request. Race conditions are unlikely. | Add `-race` flag to CI pipeline (Phase 5, step 5.6). |
| `generateID()` randomness quality | Uses `crypto/rand`; testing randomness properties is not meaningful in a unit test. | N/A — trust stdlib. |
| CORS middleware | Not yet implemented (deferred to Phase 4). | Phase 4, step 4.7. |

---

## Manual Testing

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

## Running in CI

```bash
# All tests with race detector
go test -race -count=1 ./...

# With coverage summary
go test -cover ./internal/middleware/ ./internal/observability/
```

Target: all tests pass, no race conditions detected. Coverage tracking is
informational at this stage — not gated.
