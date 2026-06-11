# Changelog

All notable changes to Tuck are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

---

## [Unreleased]

---

## [0.11.0] — 2026-06-11

### Added

#### HA-1 — Raft-replicated backend (`internal/physical/raft`)
- New `physraft.Backend` implementing `physical.Backend` via Hashicorp Raft consensus.
- **All writes replicated** through the Raft log — AES-256-GCM ciphertext is still the only thing that ever hits storage; Raft adds consensus on top, not cleartext.
- **FSM** backed by bbolt (`fsm.db`): applies `put`/`delete` commands committed by the cluster leader. Snapshot/restore support for log compaction.
- **Persistent stores**: Raft log + stable store in `raft.db` (raft-boltdb/v2), file-based snapshot store.
- **TCP transport** with configurable `BindAddr` and `AdvertiseAddr` for multi-node setups.
- `ErrNotLeader` — write operations on followers return a typed error; the HTTP layer maps it to `503 not leader`.
- `Backend.Status()` — real-time cluster topology (leader ID, leader addr, all servers, suffrage).
- `Backend.AddVoter` / `Backend.RemoveServer` — online membership changes from the leader.

#### Cluster HTTP API (`/v1/sys/cluster`)
- `GET /v1/sys/cluster` — returns cluster topology (is_leader, leader, servers).
- `POST /v1/sys/cluster/join` — adds a voter to a running cluster (`{"node_id","raft_addr"}`); must be called against the leader.
- `DELETE /v1/sys/cluster/node/{id}` — removes a voter from the cluster.

#### Server flags (`tuck --cluster ...`)
- `--cluster` — enable Raft HA backend (replaces bbolt).
- `--cluster-node-id` — stable node identity (defaults to hostname).
- `--cluster-bind-addr` — Raft RPC listen address (default `0.0.0.0:8201`).
- `--cluster-advertise` — advertised Raft address for peer discovery.
- `--cluster-dir` — data directory for Raft logs + FSM state (default `tuck-raft/`).
- `--cluster-bootstrap` — bootstrap a fresh cluster (first node only; idempotent on restart).
- `--cluster-peers` — comma-separated `id=raftAddr` list for multi-node bootstrap.
- `--cluster-join` — auto-join an existing cluster by POSTing to the leader's HTTP API.

---

## [0.10.0] — 2026-06-11

### Added
- **Go SDK** (`pkg/client`) — typed Go client for the full Tuck API: seal management, KV v1/v2, tokens, policies. Supports `WithInsecure()` and `WithHTTPClient()` options.
- **goreleaser** (`.goreleaser.yaml`) — automated release pipeline: linux/darwin/windows × amd64/arm64 binaries, Docker images (`ghcr.io/nagenaev/tuck`, `ghcr.io/nagenaev/tuck-operator`), SHA-256 checksums, cosign keyless signing, syft SBOM.
- **GitHub Actions release workflow** (`.github/workflows/release.yml`) — triggered on `v*` tags; runs goreleaser, signs artifacts with cosign, publishes to GHCR.

---

## [0.10.0-beta.1] — 2026-06-11

Pre-release for M10 testing.

---

## [0.9.0] — 2026-06-11 (M8 + M9)

### Added

#### KV v2 — Versioned secrets (`/v2/secret/*`)
- Every write creates a new immutable version (auto-incremented version number).
- **CAS** (check-and-set) via `?cas=N` — atomic conditional write.
- **Soft-delete** (`DELETE ?versions=1,2`) and **undelete** (`POST /v2/secret/undelete/`).
- **Destroy** (`POST /v2/secret/destroy/`) — permanent, unrecoverable data removal.
- **max_versions** — configurable retention limit; oldest versions auto-destroyed.
- Version metadata (`GET/PUT/DELETE /v2/secret/metadata/`).

#### Operator — HA & reliability
- **Leader election** (`--leader-elect`) — `coordination.k8s.io/v1` Lease-based; only the leader pod reconciles.
- **Status conditions** — `Synced` and `Ready` conditions on `TuckSecret.status`.
- **Deletion policy** — `spec.deletionPolicy: Retain | Delete`; finalizer `tuck.io/cleanup` ensures cleanup runs before garbage collection.
- Exponential backoff in the watch-reconcile loop.

#### Observability & API
- **OpenTelemetry tracing** (`--otel-endpoint`) — OTLP HTTP exporter; noop when empty.
- **OpenAPI 3.0 spec** — embedded in binary, served at `GET /openapi.json`.
- **Embedded web dashboard** at `/ui/` — login, secrets browser, token & policy management.
- **Prometheus metrics** at `/metrics`.

#### Security & operations
- **Threat model** (`docs/THREAT_MODEL.md`).
- **Tamper-evident audit log** — SHA-256 hash chain; values never logged.
- **Backup/restore** — `GET /v1/sys/snapshot` (bbolt `Tx.WriteTo`).
- **Key rotation** — `POST /v1/sys/rotate` re-wraps DEK; no data re-encryption.
- **Per-IP rate limiting** — token bucket.
- **TLS** — ECDSA P-256 self-signed (`--tls-auto`) or external cert.
- **Graceful shutdown** — 30-second drain + seal on exit.

#### CLI (`tuckcli`)
- Full KV, token, and policy management.
- `TUCK_ADDR` / `TUCK_TOKEN` env vars.

#### Community
- `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `CODEOWNERS`.
- Issue templates (bug report, feature request).
- Operations runbook (`docs/RUNBOOK.md`).
- k6 load test script (`test/load/k6_soak.js`).

---

## [0.4.0] — M0–M4 (foundation)

### Added
- **M0** — AES-256-GCM envelope encryption (barrier), bbolt backend, dev seal, KV HTTP API.
- **M1** — Token authentication, path-based ACL policies (glob-matching).
- **M2** — Kubernetes ServiceAccount auth via `TokenReview` API.
- **M3** — `TuckSecret` CRD + operator controller; `deploy/` manifests.
- **M4** — Production seals: Shamir secret sharing (n-of-k), Transit (Vault-compatible KMS).

---

[Unreleased]: https://github.com/NAGenaev/tuck/compare/v0.11.0...HEAD
[0.11.0]: https://github.com/NAGenaev/tuck/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/NAGenaev/tuck/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/NAGenaev/tuck/compare/v0.4.0...v0.9.0
[0.4.0]: https://github.com/NAGenaev/tuck/releases/tag/v0.4.0
