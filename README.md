# ATConnect

An OIDC Identity Provider that extends ATProto's OAuth implementation to standards-compliant OpenID Connect. Users authenticate with their ATProto PDS, and this service issues OIDC ID Tokens containing their DID and handle — making ATProto identities usable with any OIDC-compatible service such as Cloudflare Access, AWS IAM Identity Center, etc.

---

## Current State

**This project is in early development and is not yet functional as an OIDC provider.**

What currently works:
- **CLI** — completes a real ATProto OAuth flow end-to-end: opens a browser, handles the callback, and prints the authenticated DID and session info. A minimal implementation of the ATProto OAuth Flow.
- **Server** — starts and serves the ATProto OAuth callback endpoint (`/callback`) and a health check (`/health`). Useful for verifying the OAuth flow works in a server context.

Current limitations:
- **No OIDC endpoints.** `/authorize`, `/token`, `/userinfo`, `/.well-known/openid-configuration`, and `/jwks` do not exist yet. The server cannot act as an OIDC provider.
- **Sessions are not persisted.** The only store backend is in-memory. All sessions are lost on restart.
- **No relying party support.** There is no client registration, no token issuance, and no way to connect a service like Cloudflare Access.
- **Not production-ready.** No request logging, no metrics, no rate limiting, no TLS.

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
| `ATCONNECT_STORE_BACKEND` | Storage backend (`memory`) | `memory` |
| `ATCONNECT_ISSUER_URL` | Public issuer URL for OIDC discovery | `http://localhost:8080` |

---

## Project Structure

```
cmd/cli/        # Interactive dev tool — single OAuth flow, then exits
cmd/server/     # Production server entry point
pkg/            # Public, importable packages (models, store interfaces, errors)
internal/       # Application wiring (config, oauth client, HTTP server)
```

---

## What's Next

**Phase 2 — Observability:** Structured request logging, panic recovery middleware, Prometheus metrics (`/metrics`), and health endpoints (`/healthz`, `/readyz`).

**Phase 3 — SQLite Store:** Persistent session and key storage via SQLite — enabling single-binary production deployments with no external database.

**Phase 4 — OIDC Provider:** The core feature. Full OpenID Connect endpoints (`/authorize`, `/token`, `/userinfo`, `/.well-known/openid-configuration`, `/jwks`) bridging ATProto authentication to OIDC relying parties.
