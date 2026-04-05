package sqlite

// All SQL statements used by the SQLite store, organised by domain.
//
// Keeping SQL as Go constants (rather than embedded files) gives us:
//   - Single-file overview of every query in the system
//   - Compile-time string safety (no runtime file-read failures)
//   - Easy grep/search without leaving Go tooling
//   - No embed.FS machinery for simple 2–5 line queries
//
// If queries grow large or numerous, they can be extracted to individual
// .sql files with go:embed — the domain method files won't change.

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const sqlSchema = `
CREATE TABLE IF NOT EXISTS oauth_sessions (
    account_did TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    data        BLOB NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_did, session_id)
);

CREATE TABLE IF NOT EXISTS oauth_auth_requests (
    state      TEXT PRIMARY KEY,
    data       BLOB NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS oidc_keys (
    kid        TEXT PRIMARY KEY,
    key_data   BLOB NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS oidc_clients (
    client_id  TEXT PRIMARY KEY,
    data       BLOB NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

const sqlSessionGet = `
SELECT data FROM oauth_sessions
WHERE account_did = ? AND session_id = ?;`

const sqlSessionUpsert = `
INSERT INTO oauth_sessions (account_did, session_id, data, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(account_did, session_id) DO UPDATE SET
    data       = excluded.data,
    updated_at = CURRENT_TIMESTAMP;`

const sqlSessionDelete = `
DELETE FROM oauth_sessions
WHERE account_did = ? AND session_id = ?;`

// ---------------------------------------------------------------------------
// Auth Requests
// ---------------------------------------------------------------------------

const sqlAuthRequestGet = `
SELECT data FROM oauth_auth_requests
WHERE state = ?;`

const sqlAuthRequestCreate = `
INSERT INTO oauth_auth_requests (state, data, created_at)
VALUES (?, ?, CURRENT_TIMESTAMP);`

const sqlAuthRequestDelete = `
DELETE FROM oauth_auth_requests
WHERE state = ?;`

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

const sqlKeyGet = `
SELECT key_data FROM oidc_keys
WHERE kid = ?;`

const sqlKeyUpsert = `
INSERT INTO oidc_keys (kid, key_data, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(kid) DO UPDATE SET
    key_data   = excluded.key_data,
    updated_at = CURRENT_TIMESTAMP;`

const sqlKeyList = `
SELECT kid FROM oidc_keys
ORDER BY kid ASC;`

// ---------------------------------------------------------------------------
// Clients
// ---------------------------------------------------------------------------

const sqlClientGet = `
SELECT data FROM oidc_clients
WHERE client_id = ?;`

const sqlClientUpsert = `
INSERT INTO oidc_clients (client_id, data, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(client_id) DO UPDATE SET
    data       = excluded.data,
    updated_at = CURRENT_TIMESTAMP;`

const sqlClientList = `
SELECT data FROM oidc_clients
ORDER BY client_id ASC;`

const sqlClientDelete = `
DELETE FROM oidc_clients
WHERE client_id = ?;`
