# Changelog

All notable changes to Tuck are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

---

## [Unreleased]

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

[Unreleased]: https://github.com/NAGenaev/tuck/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/NAGenaev/tuck/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/NAGenaev/tuck/compare/v0.4.0...v0.9.0
[0.4.0]: https://github.com/NAGenaev/tuck/releases/tag/v0.4.0
