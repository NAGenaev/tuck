# Tuck

> The simplest Kubernetes-native secrets manager. Tuck a secret away — no ceremony.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.18.0-green)](https://github.com/NAGenaev/tuck/releases)

Tuck is an open-source secrets manager built for Kubernetes. The pitch: **anti-Vault** — a single static binary, no external database, auto-unseal by default. `kubectl apply` and it runs.

[Документация на русском →](README_RU.md)

---

## Why Tuck?

[HashiCorp Vault](https://www.vaultproject.io) is powerful but operationally heavy: Consul or Raft quorum, manual unseal on every restart, a complex ACL model. [OpenBao](https://openbao.org) inherits that complexity. [Infisical](https://infisical.com) requires a database.

Tuck's wedge is **operational simplicity**:

| | Vault | Tuck |
|---|---|---|
| Dependencies | Consul / Raft + DB | none — single binary |
| Storage | External | Embedded bbolt or built-in Raft |
| Unseal on restart | Manual (Shamir quorum) | Auto (dev / transit) |
| Kubernetes operator | External (ESO) | Built-in |
| Secrets engines | PKI, Transit, SSH, Database, TOTP | Same |
| Auth methods | Token, K8s, JWT, AppRole | Same |
| HA | Vault Enterprise / OSS Raft | Built-in Raft (3–5 nodes) |
| Binary size | ~300 MB | ~20 MB |

---

## Features

### Core

- **AES-256-GCM envelope encryption** — root key → DEK → ciphertext; key rotation re-wraps only the DEK, no data re-encryption
- **Three seal types:** dev (auto-unseal, local), Shamir (n-of-k quorum), Transit (KMS via Vault-compatible API)
- **KV v1** — simple key-value secrets with ACL enforcement
- **KV v2** — versioned secrets: CAS (check-and-set), soft-delete, undelete, destroy, configurable `max_versions`
- **Tamper-evident audit log** — SHA-256 hash chain, secret values never logged
- **Per-IP rate limiting** — token bucket, exponential backoff on auth failures
- **TLS** — ECDSA P-256 self-signed for dev, or bring your own cert
- **Graceful shutdown** — 30-second drain + seal on exit
- **Backup/restore** — `GET /v1/sys/snapshot` streams a live bbolt snapshot
- **Key rotation** — `POST /v1/sys/rotate` generates a new root key, re-wraps the DEK

### Auth methods

| Method | Description |
|--------|-------------|
| **Token** | Root token on init; create scoped tokens with TTL and policies |
| **Kubernetes SA** | Workloads exchange ServiceAccount JWT via `TokenReview` API |
| **JWT / OIDC** | Any OIDC provider — Keycloak, Auth0, GitHub Actions, Google |
| **AppRole** | Machine-to-machine auth via `role_id` + `secret_id` pairs |

### Dynamic secrets engines

| Engine | Description |
|--------|-------------|
| **Database** | On-demand PostgreSQL / MySQL credentials; auto-revoked at lease expiry |
| **PKI** | Internal X.509 CA; issue short-lived TLS certificates per role |
| **Transit** | Encryption-as-a-service; versioned keys (AES-256-GCM, ECDSA, Ed25519, RSA-PSS); sign/verify/HMAC; rewrap after rotation |
| **SSH** | CA-mode SSH certificates; sign user or host public keys; `TrustedUserCAKeys` workflow |
| **TOTP** | Store TOTP secrets and validate / generate time-based OTP codes; `otpauth://` URL output |

### Kubernetes

- **Operator** — `TuckSecret` CRD syncs Tuck secrets into native K8s Secrets; status conditions (`Synced`, `Ready`); Lease-based leader election; deletion policy (`Retain` / `Delete`)
- **Webhook Agent Injector** — MutatingWebhook injects an init container that writes secrets to a tmpfs volume; bypasses etcd entirely
- **Helm chart** — single `helm install` deploys server + operator + optional injector

### Operations

- **Raft HA** — built-in 3–5 node cluster; embedded consensus; no external coordination service
- **Prometheus metrics** at `/metrics`
- **OpenTelemetry tracing** (OTLP exporter)
- **Embedded web dashboard** at `/ui/`
- **CLI client** (`tuckcli`) — full KV, token, policy management
- **Go SDK** (`pkg/client`) — typed Go client for the full API
- **OpenAPI 3.0 spec** at `/openapi.json`

---

## Quickstart

### Run locally (dev seal)

```sh
go run ./cmd/tuck --seal-type=dev
# tuck: unsealed (dev seal), serving on https://127.0.0.1:8200
# ROOT TOKEN (shown once): tuck_...
```

### Store and retrieve a secret

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_...

tuckcli kv put db/password s3cr3t
tuckcli kv get db/password
# {"path":"db/password","value":"s3cr3t"}

tuckcli kv list db/
# {"keys":["password"]}
```

### Or with curl

```sh
curl -k -X PUT https://127.0.0.1:8200/v1/secret/db/password \
  -H "X-Tuck-Token: $TUCK_TOKEN" -d 's3cr3t'

curl -k https://127.0.0.1:8200/v1/secret/db/password \
  -H "X-Tuck-Token: $TUCK_TOKEN"
```

---

## Production (Shamir seal)

```sh
tuck \
  --seal-type=shamir \
  --seal-shamir-n=5 \
  --seal-shamir-k=3 \
  --addr=0.0.0.0:8200 \
  --tls-cert=/etc/tuck/tls.crt \
  --tls-key=/etc/tuck/tls.key \
  --data-dir=/var/lib/tuck
```

On first start, Tuck prints the root token and 5 Shamir shares. Distribute the shares to separate operators — none of them alone can unseal.

After a restart, submit any 3 shares:

```sh
tuckcli unseal <share-1>
tuckcli unseal <share-2>
tuckcli unseal <share-3>   # "unsealed successfully"
```

---

## Kubernetes

### Helm install

```sh
helm install tuck deploy/helm/tuck \
  --namespace tuck-system --create-namespace \
  --set server.sealType=shamir \
  --set server.shamirSeal.n=5,server.shamirSeal.k=3
```

### Declare a secret

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-creds
  namespace: my-app
spec:
  tuckPath: db/password
  secretName: db-secret
  secretKey: password
  deletionPolicy: Retain
```

### Webhook injection (bypasses etcd)

```yaml
metadata:
  annotations:
    tuck.io/inject: "true"
    tuck.io/addr: "https://tuck.tuck-system:8200"
    tuck.io/secrets: "db/password:password.txt,api/key:api-key.txt"
```

Secrets are written to `/tuck/secrets/` on a tmpfs volume before app containers start.

---

## Dynamic Secrets Examples

### PKI — issue a TLS certificate

```sh
# Generate root CA
curl -XPOST https://tuck:8200/v1/pki/generate/root \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"common_name":"Tuck Internal CA","ttl":"87600h"}'

# Create a role
curl -XPUT https://tuck:8200/v1/pki/roles/web \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"allowed_domains":["svc.cluster.local"],"allow_subdomains":true,"default_ttl":"72h"}'

# Issue a cert
curl -XPOST https://tuck:8200/v1/pki/issue/web \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"common_name":"api.svc.cluster.local"}'
```

### SSH — sign an SSH public key

```sh
# Set up target host (once): add CA pubkey to TrustedUserCAKeys
curl https://tuck:8200/v1/ssh/ca/public-key | jq -r .public_key \
  | sudo tee /etc/ssh/trusted_user_ca_keys

# Sign a user's key
curl -XPOST https://tuck:8200/v1/ssh/sign/ops \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"public_key":"ssh-ed25519 AAAA...","valid_principals":["ubuntu"],"ttl":"24h"}'
```

### Transit — encrypt/decrypt without handling keys

```sh
# Create an AES key
curl -XPOST https://tuck:8200/v1/transit/keys/payments \
  -H "X-Tuck-Token: $TOKEN" -d '{"type":"aes256-gcm96"}'

# Encrypt
CIPHER=$(curl -s -XPOST https://tuck:8200/v1/transit/encrypt/payments \
  -H "X-Tuck-Token: $TOKEN" \
  -d "{\"plaintext\":\"$(echo -n 'card:4242' | base64)\"}" | jq -r .ciphertext)

# Rotate and rewrap
curl -XPOST https://tuck:8200/v1/transit/keys/payments/rotate \
  -H "X-Tuck-Token: $TOKEN"
curl -XPOST https://tuck:8200/v1/transit/rewrap/payments \
  -H "X-Tuck-Token: $TOKEN" -d "{\"ciphertext\":\"$CIPHER\"}"
```

### TOTP — 2FA validation

```sh
# Create a key (returns secret + otpauth:// URL for QR import)
curl -XPOST https://tuck:8200/v1/totp/keys/myapp \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"issuer":"ACME Corp","account":"user@example.com"}'

# Validate user-submitted code
curl -XPOST https://tuck:8200/v1/totp/code/myapp \
  -H "X-Tuck-Token: $TOKEN" -d '{"code":"123456"}'
# → {"valid":true}
```

---

## API Reference

All endpoints require `X-Tuck-Token` header unless marked **public**.

### System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/sys/seal-status` | public | Seal state |
| GET | `/v1/sys/ready` | public | Readiness (503 if sealed) |
| GET | `/v1/health` | public | Liveness |
| POST | `/v1/sys/unseal` | public | Submit Shamir share |
| POST | `/v1/sys/seal` | token | Seal the server |
| POST | `/v1/sys/rotate` | token | Rotate root key |
| GET | `/v1/sys/snapshot` | token | Download bbolt backup |
| GET | `/v1/sys/cluster` | token | Raft cluster status |
| POST | `/v1/sys/cluster/join` | token | Add a Raft voter |
| DELETE | `/v1/sys/cluster/node/{id}` | token | Remove a Raft voter |

### KV v1

| Method | Path | Description |
|--------|------|-------------|
| GET/PUT/DELETE | `/v1/secret/{path}` | Read / write / delete |
| LIST | `/v1/secret/{prefix}` | List keys |

### KV v2 (versioned)

| Method | Path | Description |
|--------|------|-------------|
| GET/PUT/DELETE | `/v2/secret/{path}` | Read / write / soft-delete |
| LIST | `/v2/secret/{prefix}` | List keys |
| POST | `/v2/secret/undelete/{path}` | Restore a soft-deleted version |
| POST | `/v2/secret/destroy/{path}` | Permanent delete |
| GET/PUT/DELETE | `/v2/secret/metadata/{path}` | Version metadata |

### Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/auth/token` | Create token |
| GET/DELETE | `/v1/auth/token/{id}` | Get / revoke |
| POST | `/v1/auth/token/{id}/renew` | Renew |
| LIST | `/v1/auth/token/` | List |
| POST | `/v1/auth/kubernetes/login` | K8s SA login (public) |
| PUT/GET/DELETE | `/v1/auth/kubernetes/role/{ns}/{sa}` | K8s role binding |
| POST | `/v1/auth/jwt/login` | JWT/OIDC login (public) |
| GET/PUT | `/v1/auth/jwt/config` | JWKS config |
| PUT/GET/DELETE | `/v1/auth/jwt/role/{name}` | JWT role |
| LIST | `/v1/auth/jwt/role/` | List JWT roles |
| POST | `/v1/auth/approle/login` | AppRole login (public) |
| PUT/GET/DELETE | `/v1/auth/approle/role/{name}` | AppRole role |
| LIST | `/v1/auth/approle/role/` | List roles |
| POST | `/v1/auth/approle/role/{name}/secret-id` | Generate secret-id |
| GET/DELETE | `/v1/auth/approle/role/{name}/secret-id/{id}` | Inspect / destroy |

### Policies

| Method | Path | Description |
|--------|------|-------------|
| PUT/GET/DELETE | `/v1/policy/{name}` | Manage policy |
| LIST | `/v1/policy/` | List |

### Database engine

| Method | Path | Description |
|--------|------|-------------|
| PUT/GET/DELETE | `/v1/database/config/{name}` | Connection config |
| LIST | `/v1/database/config/` | List configs |
| PUT/GET/DELETE | `/v1/database/role/{name}` | Role (creation/revocation SQL) |
| LIST | `/v1/database/role/` | List roles |
| POST | `/v1/database/creds/{role}` | Generate ephemeral credentials |
| GET/DELETE | `/v1/database/lease/{id}` | Inspect / revoke lease |
| LIST | `/v1/database/lease/` | List leases |

### PKI engine

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/pki/generate/root` | token | Generate root CA |
| POST | `/v1/pki/import/ca` | token | Import existing CA |
| GET | `/v1/pki/ca/pem` | **public** | CA certificate (for client trust stores) |
| GET | `/v1/pki/crl/pem` | **public** | Current CRL |
| PUT/GET/DELETE | `/v1/pki/roles/{name}` | token | Manage roles |
| LIST | `/v1/pki/roles/` | token | List roles |
| POST | `/v1/pki/issue/{role}` | token | Issue certificate |
| POST | `/v1/pki/revoke/{serial}` | token | Revoke certificate |
| GET | `/v1/pki/certs/{serial}` | token | Get cert record |
| LIST | `/v1/pki/certs/` | token | List certs |

### Transit engine

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/transit/keys/{name}` | Create key |
| GET/DELETE | `/v1/transit/keys/{name}` | Get / delete |
| LIST | `/v1/transit/keys/` | List |
| POST | `/v1/transit/keys/{name}/rotate` | Rotate (new version) |
| POST | `/v1/transit/keys/{name}/config` | Set `min_decryption_version`, `deletable` |
| POST | `/v1/transit/encrypt/{name}` | Encrypt |
| POST | `/v1/transit/decrypt/{name}` | Decrypt |
| POST | `/v1/transit/rewrap/{name}` | Rewrap ciphertext with latest key |
| POST | `/v1/transit/sign/{name}` | Sign |
| POST | `/v1/transit/verify/{name}` | Verify signature |
| POST | `/v1/transit/hmac/{name}` | Compute HMAC |

### SSH engine

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/ssh/generate/ca` | token | Generate CA key pair |
| POST | `/v1/ssh/import/ca` | token | Import existing CA |
| GET | `/v1/ssh/ca/public-key` | **public** | CA public key (for `TrustedUserCAKeys`) |
| PUT/GET/DELETE | `/v1/ssh/roles/{name}` | token | Manage roles |
| LIST | `/v1/ssh/roles/` | token | List roles |
| POST | `/v1/ssh/sign/{role}` | token | Sign a public key → SSH certificate |

### TOTP engine

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/totp/keys/{name}` | Create key (returns `otpauth://` URL) |
| GET/DELETE | `/v1/totp/keys/{name}` | Get metadata / delete |
| LIST | `/v1/totp/keys/` | List |
| GET | `/v1/totp/code/{name}` | Generate current code |
| POST | `/v1/totp/code/{name}` | Validate code → `{"valid":true}` |

### Other

| Path | Description |
|------|-------------|
| `GET /metrics` | Prometheus metrics |
| `GET /ui/` | Embedded web dashboard |
| `GET /openapi.json` | OpenAPI 3.0 spec |

---

## CLI Reference

```sh
tuckcli status                              # seal status
tuckcli unseal <share>                      # submit Shamir share
tuckcli seal                                # seal the server
tuckcli rotate                              # rotate root key

tuckcli kv get <path>                       # read a secret
tuckcli kv put <path> <value>               # write a secret
tuckcli kv delete <path>                    # delete a secret
tuckcli kv list <prefix/>                   # list keys

tuckcli token create --name=x --policy=y --ttl=24h
tuckcli token get <id>
tuckcli token renew <id> 48h
tuckcli token revoke <id>
tuckcli token list

tuckcli policy put <name> <json-rules>
tuckcli policy get <name>
tuckcli policy delete <name>
tuckcli policy list
```

Environment variables: `TUCK_ADDR` (default `https://127.0.0.1:8200`), `TUCK_TOKEN`.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  HTTP API  (net/http, no framework)                      │
│  TLS · Auth middleware · Rate limiter · Audit log        │
│  Prometheus metrics · OpenTelemetry · OpenAPI            │
├─────────────────────────────────────────────────────────┤
│  Core  (orchestration + logical operations)              │
│  Token store · Policy store · KV v1/v2                   │
├────────────────┬────────────────────────────────────────┤
│  Auth engines  │  Dynamic secrets engines               │
│  · K8s SA      │  · Database (PostgreSQL / MySQL)       │
│  · JWT / OIDC  │  · PKI (X.509 CA)                      │
│  · AppRole     │  · Transit (encryption-as-a-service)   │
│                │  · SSH (CA-mode certificates)           │
│                │  · TOTP (time-based OTP)               │
├────────────────┴────────────────────────────────────────┤
│  Barrier  (AES-256-GCM envelope encryption)             │
│  root key → DEK → ciphertext                            │
├─────────────────────────────────────────────────────────┤
│  Physical backend                                        │
│  bbolt (single file) | Raft HA (3–5 nodes, embedded)    │
└─────────────────────────────────────────────────────────┘
               ▲
         Seal (dev | shamir | transit)
```

---

## Go SDK

```go
import "github.com/NAGenaev/tuck/pkg/client"

c := client.New("https://tuck:8200", client.WithToken("tuck_..."))

// KV
c.Put(ctx, "secret/db/password", []byte("s3cr3t"))
val, _ := c.Get(ctx, "secret/db/password")

// KV v2
c.V2Write(ctx, "app/config", map[string]string{"key": "val"})
```

---

## Development

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck

go test ./...              # all tests
go test -race ./...        # with race detector
go build ./cmd/tuck        # server binary
go build ./cmd/tuckcli     # CLI binary
go build ./cmd/tuck-operator
go build ./cmd/tuck-injector
go build ./cmd/tuck-agent
```

See [CONTRIBUTING.md](CONTRIBUTING.md), [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/RUNBOOK.md](docs/RUNBOOK.md).

---

## Status

| Milestone | Version | Status |
|-----------|---------|--------|
| M0 — Crypto core, bbolt, KV API | v0.1 | ✅ |
| M1 — Token auth, ACL policies | v0.2 | ✅ |
| M2 — Kubernetes SA auth | v0.3 | ✅ |
| M3 — TuckSecret CRD + operator | v0.4 | ✅ |
| M4 — Shamir + Transit seals | v0.5 | ✅ |
| M5 — TLS, graceful shutdown, CI | v0.5 | ✅ |
| M6 — Audit log, metrics, backup, rate limiting | v0.6 | ✅ |
| M7 — LIST endpoints, token renew, key rotation, CLI | v0.7 | ✅ |
| M8 — HA operator, embedded UI, threat model | v0.9 | ✅ |
| M9 — KV v2, OpenTelemetry, OpenAPI spec, embedded dashboard | v0.9 | ✅ |
| M10 — Go SDK, goreleaser, release pipeline | v0.10 | ✅ |
| M11 — Raft HA backend (3–5 node cluster) | v0.11 | ✅ |
| M12 — Webhook Agent Injector (tmpfs, bypasses etcd) | v0.12 | ✅ |
| M13 — JWT/OIDC auth, Helm chart | v0.13 | ✅ |
| M14 — AppRole auth, Database dynamic secrets | v0.14 | ✅ |
| M15 — PKI secrets engine (internal X.509 CA) | v0.15 | ✅ |
| M16 — Transit secrets engine (encryption-as-a-service) | v0.16 | ✅ |
| M17 — SSH secrets engine (CA-mode certificates) | v0.17 | ✅ |
| M18 — TOTP secrets engine (2FA / OTP validation) | v0.18 | ✅ |
| v1.0 GA — External security audit | — | 🔜 |

---

## Security

See [SECURITY.md](SECURITY.md) for the vulnerability disclosure policy and [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) for the threat model.

Report vulnerabilities: **genaevlive@gmail.com** (coordinated disclosure, 90-day window).

---

## License

Apache-2.0. See [LICENSE](LICENSE).
