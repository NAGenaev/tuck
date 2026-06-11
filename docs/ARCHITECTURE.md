# Tuck — Architecture

## What is Tuck

A minimalist secrets manager written in Go. Anti-Vault: one binary, no external DB, no Consul, no etcd. Designed to run in Kubernetes next to workloads.

**Principles:**
- Single external runtime dependency: bbolt (embedded BoltDB). Raft mode adds Hashicorp Raft (pure Go, no external process).
- No gRPC frameworks, no client-go in server runtime.
- Everything stored is AES-256-GCM encrypted — physical storage never sees plaintext.

---

## Package map

```
cmd/
  tuck/             — HTTP server (entry point)
  tuckcli/          — CLI client
  tuck-operator/    — Kubernetes operator (TuckSecret CRD)
  tuck-injector/    — MutatingWebhook server
  tuck-agent/       — init-container: fetches secrets, writes to tmpfs

deploy/
  crd/              — TuckSecret CRD manifest
  server/           — Tuck server in K8s (dev seal) + RBAC
  operator/         — operator deployment
  webhook/          — MutatingWebhookConfiguration + cert-manager cert
  helm/tuck/        — Helm chart (server + operator + optional injector)

internal/
  physical/         — physical layer: bbolt backend, in-memory (tests)
  physical/raft/    — Raft-replicated backend (3–5 node HA cluster)
  barrier/          — cryptographic barrier: AES-256-GCM, sealed/unsealed state
  seal/             — root-key lifecycle: dev / shamir / transit
  shamir/           — Shamir's Secret Sharing in GF(256): Split/Combine
  core/             — orchestration: all engine managers, KV, identity
  token/            — token model: generation, TTL, storage in barrier
  policy/           — ACL: policies, glob path matching, capability check
  kvv2/             — KV v2: versioned secrets, CAS, soft-delete, destroy
  api/              — HTTP layer: routing, middleware, serialization
  audit/            — tamper-evident audit log (SHA-256 hash chain)
  metrics/          — Prometheus metrics
  ratelimit/        — per-IP token bucket
  telemetry/        — OpenTelemetry tracing (OTLP)
  tlsutil/          — TLS helpers (self-signed ECDSA, custom cert)
  ui/               — embedded web dashboard (go:embed)
  k8s/              — Kubernetes TokenReview client + RoleStore
  operator/         — TuckSecret CRD controller (leader election, conditions)
  injector/         — MutatingWebhook: pod mutation, JSON Patch builder
  auth/
    jwt/            — JWT / OIDC auth (JWKS fetcher, role matching)
    approle/        — AppRole auth (role_id + secret_id)
  dynamic/
    database/       — Database engine: PostgreSQL / MySQL dynamic creds
    pki/            — PKI engine: X.509 CA, role-based cert issuance, CRL
    transit/        — Transit engine: versioned keys, encrypt/decrypt/sign/HMAC
    ssh/            — SSH engine: CA-mode certificate signing
    totp/           — TOTP engine: RFC 6238 OTP generation + validation

pkg/
  client/           — typed Go SDK for the full Tuck API

build/
  Dockerfile.*      — distroless images (uid 65532)
```

---

## Layers and data flow

```
Client (curl / tuckcli / SDK / operator)
        │  HTTPS
        ▼
  api.Server
    TLS termination
    Rate limiter (per-IP token bucket)
    Audit middleware (hash chain)
    Prometheus middleware
    requireToken() → core.Authenticate()
        │
        ▼
  core.Core
    Authenticate → EnforceAccess (policy glob match)
    Routes to the correct engine or store:
      ┌── KV v1/v2 (barrier.Get/Put/Delete/List)
      ├── Auth (token / k8s / jwt / approle)
      ├── Database engine
      ├── PKI engine
      ├── Transit engine
      ├── SSH engine
      └── TOTP engine
        │
        ▼
  barrier.Barrier
    IsSealed() → 503 on any operation
    Get/Put/Delete/List: AES-256-GCM encrypt/decrypt per entry
        │
        ▼
  physical.Backend
    bbolt (single .db file) — only ciphertext ever touches disk
    OR physraft.Backend — Raft-replicated, same ciphertext guarantee
```

---

## Cryptography

```
root key (32 bytes, in memory only — provided by seal)
     │
     └──▶ barrier DEK (AES-256 key, stored as: AES-256-GCM(root_key, DEK))
               │
               └──▶ AES-256-GCM(DEK, nonce, plaintext) ──▶ physical backend
```

**Envelope encryption:** root key encrypts DEK; DEK encrypts data entries.

On restart: `seal.Unseal()` → root key → `barrier.Unseal()` → DEK decrypted → ready.

**Key rotation** (`POST /v1/sys/rotate`): generates new root key via seal, re-wraps DEK with new root key. No data re-encryption needed; only the keyring entry changes.

---

## Seal types

### dev (development only)
- Root key stored **in plaintext** in a local file.
- Auto-unseals on startup.
- **Never use in production.**

### shamir (multi-operator, on-prem)
- Root key split into N shares using Shamir's Secret Sharing over GF(256).
- Each share: `base64url(x || f(x))` — standalone share reveals nothing.
- Server starts sealed; operators submit shares one at a time via `POST /v1/sys/unseal`.
- After K shares: barrier unseals automatically.
- Shares are never persisted — process repeats on every restart.

### transit (cloud, auto-unseal)
- Root key **wrapped** by an external KMS (Vault Transit-compatible API) at init time.
- Wrapped blob stored locally.
- On startup: read blob → POST to unwrap endpoint → root key → auto-unseal.

---

## Physical storage layout (barrier keys)

| Logical key | Contents |
|---|---|
| `barrier/keyring` | DEK encrypted with root key |
| `auth/token/<id>` | JSON token record |
| `auth/policy/<name>` | JSON policy |
| `auth/k8s/role/<ns>/<sa>` | K8s role binding |
| `auth/jwt/config` | JWKS config |
| `auth/jwt/roles/<name>` | JWT role |
| `auth/approle/roles/<name>` | AppRole role |
| `auth/approle/secrets/<id>` | AppRole secret-id |
| `secret/<path>` | KV v1 value (raw bytes) |
| `kvv2/<path>/meta` | KV v2 version metadata |
| `kvv2/<path>/v/<n>` | KV v2 version data |
| `dynamic/database/config/<name>` | DB connection config |
| `dynamic/database/roles/<name>` | DB role (SQL templates) |
| `dynamic/database/leases/<id>` | DB credential lease |
| `dynamic/pki/ca` | PKI CA cert + encrypted private key |
| `dynamic/pki/roles/<name>` | PKI role |
| `dynamic/pki/certs/<serial>` | Issued cert record (no private key) |
| `dynamic/transit/keys/<name>` | Transit key record (all versions, encrypted) |
| `dynamic/ssh/ca` | SSH CA key pair (private key encrypted by barrier) |
| `dynamic/ssh/roles/<name>` | SSH role |
| `dynamic/totp/keys/<name>` | TOTP secret + config (secret encrypted by barrier) |
| `audit/last_hash` | Latest audit log hash |

---

## Dynamic secrets engines

Each engine is isolated in its own package and talks to the barrier through a minimal local `barrier` interface (Get / Put / Delete / List). No engine receives the full `physical.Backend`.

### Database engine (`internal/dynamic/database`)
- Registers named database configs (PostgreSQL / MySQL DSN + pool settings).
- Roles carry `creation_statements` and `revocation_statements` with `{{username}}`, `{{password}}`, `{{expiry}}` templates.
- `GenerateCreds` → executes creation SQL, returns credentials, records a `Lease`.
- Background GC (`RevokeExpired`) revokes leases via the revocation SQL.

### PKI engine (`internal/dynamic/pki`)
- Generates or imports a root CA (ECDSA P-256 default, or RSA).
- Roles constrain: `allowed_domains`, `allow_subdomains`, `allow_ip_sans`, `allow_localhost`, `key_type`, `key_bits`, `default_ttl`, `max_ttl`, `server_flag`, `client_flag`.
- `IssueCert` validates CN + SANs, generates a new key pair, signs the leaf cert, persists a `CertRecord` (no private key stored), returns the cert + private key once.
- `RevokeCert` marks a cert revoked; `GetCRL` generates a fresh signed CRL on demand.
- CA cert and CRL endpoints are unauthenticated — clients can build trust stores without a token.

### Transit engine (`internal/dynamic/transit`)
- Versioned keys: `aes256-gcm96`, `ecdsa-p256`, `ed25519`, `rsa-2048`, `rsa-4096`.
- Ciphertext/signature format: `vault:v{N}:{base64url-payload}` — version embedded for unambiguous routing.
- AES: 12-byte random nonce prepended to GCM output.
- `Rotate` adds a new key version; old versions remain available down to `min_decryption_version`.
- `Rewrap` = decrypt with old version + encrypt with latest version.
- Keys are not deletable by default; `UpdateKey` sets `deletable=true` first.

### SSH engine (`internal/dynamic/ssh`)
- Generates or imports an SSH CA (Ed25519 default, or RSA-4096).
- Roles control: `allowed_users` (empty = any principal), `cert_type` (user / host), `default_ttl`, `max_ttl`, `default_extensions`.
- `SignPublicKey` validates principals against the role, caps TTL at `max_ttl`, signs the certificate with a random 64-bit serial.
- Host certs have no extensions (SSH spec). User certs default to the five standard `permit-*` extensions.
- CA public key endpoint is unauthenticated — hosts fetch it for `TrustedUserCAKeys` without a token.

### TOTP engine (`internal/dynamic/totp`)
- Stores TOTP secrets in the barrier (encrypted at rest).
- Implements RFC 6238 / RFC 4226: `HOTP(key, floor(unix/period))` with dynamic truncation.
- Supports SHA1 (default), SHA256, SHA512; 6 or 8 digits; configurable period and skew window.
- `CreateKey` generates a random 20-byte secret and returns an `otpauth://` URI for QR import.
- `GenerateCode` returns the current code and its expiry timestamp.
- `ValidateCode` checks the submitted code against `skew` adjacent time windows on each side.

---

## Tokens

Format: `tuck_` + base64url(32 random bytes)

Fields: `id`, `display_name`, `policies []string`, `created_at`, `expires_at`

**Root token:** the only token with the `root` policy. Created at first startup, printed once. The root policy is hardcoded and cannot be deleted via the API.

Token GC runs every 15 minutes (background goroutine) and removes entries where `expires_at < now`.

---

## Policies (ACL)

```json
{
  "name": "db-readwrite",
  "rules": [
    {"path": "secret/db/*",     "capabilities": ["read", "write", "delete"]},
    {"path": "secret/shared/*", "capabilities": ["read"]}
  ]
}
```

Capabilities: `read`, `write`, `delete`, `list`.

Path matching is glob-based: `secret/db/*` matches `secret/db/password` but not `secret/db/sub/key`. Use `secret/**` for recursive depth. The root policy matches everything with all capabilities.

---

## Kubernetes operator (TuckSecret CRD)

The operator syncs secrets from Tuck into native Kubernetes Secrets.

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: production
spec:
  tuckPath: db/password          # path in Tuck (without /v1/secret/)
  secretName: db-credentials     # target K8s Secret name
  secretKey: password            # key within K8s Secret .data
  refreshInterval: "5m"
  deletionPolicy: Retain         # Retain (default) or Delete
```

**Lifecycle:**
1. Operator authenticates via K8s SA token (`POST /v1/auth/kubernetes/login`).
2. Tuck token cached (TTL 4 min, refreshed 30s before expiry).
3. Lease-based leader election — only the leader pod reconciles.
4. On ADDED / MODIFIED: GET secret from Tuck → apply K8s Secret.
5. `status.conditions` updated: `Synced: True` or error message.
6. On DELETED with `deletionPolicy: Delete`: finalizer deletes K8s Secret before GC.

---

## Webhook Agent Injector

Bypasses etcd entirely. Secrets exist only in Pod memory on a tmpfs volume.

**Flow:**
1. `MutatingWebhookConfiguration` intercepts Pod creation in annotated namespaces.
2. `injector.Handler` calls `BuildPatch` to produce a RFC 6902 JSON Patch:
   - Adds a `tuck-secrets` `emptyDir{medium: Memory}` volume.
   - Adds a `tuck-agent` init container.
   - Mounts the volume read-only into every app container at `/tuck/secrets/`.
3. `tuck-agent` (init container):
   - Reads `TUCK_ADDR`, `TUCK_TOKEN_FILE`, `TUCK_SECRETS`.
   - Fetches each secret via `pkg/client`.
   - Writes files atomically (`tmpfile → rename`) with mode `0400`.
   - Fails fast → Pod creation blocked if any secret is missing.

---

## Raft HA backend

```
Node 1 (leader)          Node 2            Node 3
┌──────────────┐        ┌───────────┐     ┌───────────┐
│  Tuck server │──Raft──│ Tuck server│────│ Tuck server│
│  fsm.db      │        │  fsm.db   │     │  fsm.db   │
│  raft.db     │        │  raft.db  │     │  raft.db  │
└──────────────┘        └───────────┘     └───────────┘
```

- All writes go through the Raft log (leader → majority quorum → commit).
- Only AES-256-GCM ciphertext is replicated — Raft adds consensus, not plaintext.
- `FSM` is bbolt-backed; applies `put` / `delete` commands from the committed log.
- Writes on a follower return `503 not leader` so clients can retry against the leader.
- Online membership changes: `AddVoter` / `RemoveServer` via the leader's HTTP API.

---

## Audit log

Every API call is logged as a JSON line with:
- `timestamp`, `method`, `path`, `token_id` (accessor, not the raw token), `status_code`
- `prev_hash` + `hash = SHA256(prev_hash || entry_json)` — forms a tamper-evident chain
- Secret **values are never logged**

---

## Test strategy

- Unit tests in every package; each uses a local `memBarrier` struct (no shared state).
- `go test -race ./...` is clean — all tests pass the race detector.
- Integration tests in `internal/api/` spin up a full in-memory Core + HTTP server.
- RFC 6238 known test vectors validated in `internal/dynamic/totp/totp_test.go`.
- x509 certificate chain verification in `internal/dynamic/pki/pki_test.go`.
- SSH certificate chain verification via `gossh.CertChecker` in `internal/dynamic/ssh/ssh_test.go`.
- Raft consensus tested with a 3-node in-process cluster in `internal/physical/raft/`.
