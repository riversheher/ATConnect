# ATConnect

An OIDC Identity Provider that extends ATProto's OAuth implementation to standards-compliant OpenID Connect. Users authenticate with their ATProto PDS, and this service issues OIDC ID Tokens containing their DID and handle — making ATProto identities usable with any OIDC-compatible service such as Cloudflare Access, AWS IAM Identity Center, etc.

---

## Current State

**This project is in early development and is not yet functional as an OIDC provider.**

What currently works:
- **CLI** — completes a real ATProto OAuth flow end-to-end: opens a browser, handles the callback, and prints the authenticated DID and session info.
- **Server** — starts and serves the ATProto OAuth callback endpoint (`/callback`), health checks (`/livez`, `/readyz`), and Prometheus metrics (`/metrics`).
- **Observability** — structured logging (slog), request ID propagation, panic recovery middleware, Prometheus request metrics.
- **Storage** — pluggable store backend with two implementations:
  - **Memory** — in-process, no persistence. Good for development.
  - **SQLite** — file-based persistent storage via `modernc.org/sqlite` (pure Go, no cgo). Supports sessions, auth requests, OIDC keys, and client registrations.

Current limitations:
- **No OIDC endpoints.** `/authorize`, `/token`, `/userinfo`, `/.well-known/openid-configuration`, and `/jwks` do not exist yet. The server cannot act as an OIDC provider.
- **No relying party support.** There is no client registration, no token issuance, and no way to connect a service like Cloudflare Access.
- **No schema migrations.** SQLite tables are created with `CREATE TABLE IF NOT EXISTS`. There is no versioned migration strategy yet.

---

## Requirements

- Go 1.25+

---

## Getting Started

```bash
git clone https://github.com/riversheher/atconnect.git
cd atconnect
go build ./...
```

### Run the CLI (development / testing)

Initiates a single ATProto OAuth flow in your browser and prints the resulting DID and session info.

```bash
go run ./cmd/cli
```

An optional config file can be provided:

```bash
cp config.example.yaml config.yaml
go run ./cmd/cli -config config.yaml
```

### Run the server

Starts an HTTP server that handles ATProto OAuth callbacks. This will become the full OIDC provider in upcoming phases.

```bash
go run ./cmd/server -config config.yaml
```

Configuration can also be set via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `ATCONNECT_LISTEN_ADDRESS` | Address to listen on | `:8080` |
| `ATCONNECT_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `ATCONNECT_STORE_BACKEND` | Storage backend (`memory`, `sqlite`) | `memory` |
| `ATCONNECT_STORE_SQLITE_PATH` | SQLite database file path | `./data/atconnect.db` |
| `ATCONNECT_ISSUER_URL` | Public issuer URL for OIDC discovery | `http://localhost:8080` |

---

## Project Structure

```
cmd/cli/              # Interactive dev tool — single OAuth flow, then exits
cmd/server/           # Production server entry point
pkg/models/           # Shared data types (OIDC claims, clients, users)
pkg/errors/           # Error codes and structured error types
pkg/store/            # Store interface + concrete adapters
pkg/store/memory/     #   In-memory adapter (embeds indigo's MemStore)
pkg/store/sqlite/     #   SQLite adapter (modernc.org/sqlite, pure Go)
internal/config/       # YAML + env var configuration loading
internal/oauth/        # ATProto OAuth client wrapper (indigo)
internal/server/       # HTTP server lifecycle and route registration
internal/middleware/   # Request ID, logging, panic recovery
internal/observability/ # Structured logger, Prometheus metrics, health checks
```

---

## What's Next

**Phase 4 — OIDC Provider:** The core feature. Full OpenID Connect endpoints (`/authorize`, `/token`, `/userinfo`, `/.well-known/openid-configuration`, `/jwks`) bridging ATProto authentication to OIDC relying parties.

**Phase 5 — Production Hardening:** Schema migrations, CORS, rate limiting, TLS, CI pipeline with race detector.
