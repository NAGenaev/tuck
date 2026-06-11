# Tuck

> The simplest Kubernetes-native secrets manager. Tuck a secret away — no ceremony.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.9_pre--GA-orange)]()

Tuck is an open-source secrets manager built for Kubernetes. The pitch: **anti-Vault** — a single static binary, no external database, and auto-unseal by default. `kubectl apply` and it runs.

[Документация на русском →](README_RU.md)

---

## Why Tuck?

[HashiCorp Vault](https://www.vaultproject.io) is powerful but operationally heavy: Consul or Raft quorum, manual unseal on every restart, a complex ACL model. [OpenBao](https://openbao.org) inherits that complexity. [Infisical](https://infisical.com) requires a database.

Tuck's wedge is **operational simplicity**:

| | Vault | Tuck |
|---|---|---|
| Dependencies | Consul / Raft + DB | none — single binary |
| Storage | External | Embedded bbolt |
| Unseal on restart | Manual (Shamir quorum) | Auto (dev/transit) |
| Kubernetes operator | External (ESO) | Built-in |
| Binary size | ~300 MB | ~20 MB |

Tuck is not trying to replace Vault for large enterprises. It targets **small-to-medium Kubernetes platforms** where operational overhead matters more than federation and dynamic secrets.

---

## Features

- **AES-256-GCM envelope encryption** — root key → DEK → secret data; key rotation re-wraps only the DEK, never re-encrypts data
- **Three seal types:** dev (auto-unseal, local only), Shamir (n-of-k quorum), Transit (KMS via Vault-compatible API)
- **Full REST API** — KV secrets, token auth, ACL policies, LIST endpoints, binary-safe values
- **Kubernetes operator** — `TuckSecret` CRD syncs Tuck secrets into native K8s Secrets with status conditions
- **Kubernetes SA auth** — workloads authenticate via `TokenReview` API, no sidecars
- **CLI client** (`tuckcli`) — get/put/delete/list secrets, manage tokens and policies
- **Embedded web dashboard** at `/ui/` — no build step
- **Prometheus metrics** at `/metrics`
- **Tamper-evident audit log** — SHA-256 hash chain, values never logged
- **Per-IP rate limiting** — token bucket, exponential backoff on auth failures
- **TLS** — ECDSA P-256 self-signed for dev, or bring your own cert
- **Graceful shutdown** — 30-second drain + seal on exit
- **HA operator** — Kubernetes `Lease`-based leader election

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

On first start, Tuck prints the root token and 5 Shamir shares. Distribute the shares to separate operators — none of them alone can unseal the server.

After a restart, submit any 3 shares:

```sh
tuckcli unseal <share-1>
tuckcli unseal <share-2>
tuckcli unseal <share-3>   # "unsealed successfully"
```

---

## Kubernetes

### Install the operator

```sh
kubectl apply -f deploy/crd/
kubectl apply -f deploy/operator/
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
```

The operator watches the CRD and creates/updates a native `Secret`. Status conditions show `Synced: True` or an error message.

### Kubernetes workload auth

```sh
tuckcli token create \
  --name=my-app \
  --policy=app-policy \
  --k8s-sa=my-app/default
```

The workload exchanges its ServiceAccount token for a Tuck token via `POST /v1/auth/k8s`.

---

## CLI Reference

```
tuckcli status                              # seal status
tuckcli unseal <share>                      # submit a Shamir share
tuckcli seal                                # seal the server

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

tuckcli rotate                              # rotate root key (root token required)
```

Environment variables: `TUCK_ADDR` (default `https://127.0.0.1:8200`), `TUCK_TOKEN`.

---

## Architecture

```
┌──────────────────────────────────────────────┐
│  HTTP API  (net/http, no framework)           │
│  TLS · Auth middleware · Rate limiter         │
│  Audit log (hash chain) · Metrics             │
├──────────────────────────────────────────────┤
│  Core  (orchestration, logical KV)            │
│  Token store · Policy store                   │
├──────────────────────────────────────────────┤
│  Barrier  (AES-256-GCM envelope encryption)   │
│  root key → DEK → ciphertext                  │
├──────────────────────────────────────────────┤
│  Physical backend                             │
│  bbolt (single file) | in-memory (tests)      │
└──────────────────────────────────────────────┘
     ▲
  Seal (dev | shamir | transit)
```

Key rotation: `POST /v1/sys/rotate` generates a new root key, re-wraps the DEK, and returns new Shamir shares — no data re-encryption needed.

---

## Development

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck

go test ./...              # all tests
go test -race ./...        # with race detector
go build ./cmd/tuck        # server binary
go build ./cmd/tuckcli     # CLI binary
go build ./cmd/tuck-operator  # operator binary
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines, [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for a deeper design walkthrough, and [docs/RUNBOOK.md](docs/RUNBOOK.md) for operational procedures.

### Load testing

```sh
k6 run \
  --env TUCK_ADDR=https://127.0.0.1:8200 \
  --env TUCK_TOKEN=$TUCK_TOKEN \
  --duration 1m --vus 200 \
  test/load/k6_soak.js
```

---

## API

All endpoints require `X-Tuck-Token` header except `/v1/sys/seal-status` and `/v1/sys/unseal`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/sys/seal-status` | Seal state |
| `POST` | `/v1/sys/unseal` | Submit Shamir share |
| `POST` | `/v1/sys/seal` | Seal the server |
| `POST` | `/v1/sys/rotate` | Rotate root key |
| `GET` | `/v1/sys/snapshot` | Download bbolt snapshot |
| `GET` | `/v1/health` | Liveness probe |
| `GET` | `/v1/sys/ready` | Readiness probe (503 if sealed) |
| `GET/PUT/DELETE` | `/v1/secret/{path}` | KV operations |
| `LIST` | `/v1/secret/{prefix}` | List keys |
| `POST` | `/v1/auth/token` | Create token |
| `GET/DELETE` | `/v1/auth/token/{id}` | Get / revoke token |
| `POST` | `/v1/auth/token/{id}/renew` | Renew token |
| `LIST` | `/v1/auth/token/` | List tokens |
| `GET/PUT/DELETE` | `/v1/policy/{name}` | Policy operations |
| `LIST` | `/v1/policy/` | List policies |
| `POST` | `/v1/auth/k8s` | Kubernetes SA login |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/ui/` | Web dashboard |

---

## Security

See [SECURITY.md](SECURITY.md) for the vulnerability disclosure policy and [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) for the threat model.

To report a vulnerability: **genaevlive@gmail.com** (coordinated disclosure, 90-day window).

---

## Status & Roadmap

| Milestone | Status |
|---|---|
| M0 — Crypto core (barrier, bbolt, dev seal, KV API) | ✅ |
| M1 — Token auth, ACL policies | ✅ |
| M2 — Kubernetes ServiceAccount auth | ✅ |
| M3 — TuckSecret CRD + operator | ✅ |
| M4 — Production seals (Shamir, Transit) | ✅ |
| M5 — Security & DevOps baseline (TLS, graceful shutdown, CI) | ✅ |
| M6 — Reliability & Observability (audit log, metrics, backup) | ✅ |
| M7 — API completeness (LIST, token renew, key rotation, CLI) | ✅ |
| M8 — Pre-GA hardening (HA operator, embedded UI, threat model, community) | ✅ |
| v1.0 GA — External security audit | 🔜 |

Post-GA (v1.x): Raft HA backend, KV v2 (versioning), Webhook Agent Injector, OpenTelemetry.

---

## License

Apache-2.0. See [LICENSE](LICENSE).
