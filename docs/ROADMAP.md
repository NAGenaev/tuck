# Tuck — Production Roadmap

> **Состояние: v1.0.0 GA выпущен 2026-06-12.** M0–M32 + security audit завершены. Следующий цикл: v1.x.

---

## Что такое Tuck

Tuck — встроенный менеджер секретов для Kubernetes. Анти-Vault: один статический бинарь (~20 MB),
без Consul, без etcd, без внешних зависимостей. `kubectl apply` — и работает.

**Целевая аудитория:** платформенные инженеры, DevOps-команды, SRE, которым нужна
альтернатива HashiCorp Vault без операционной сложности.

---

## v1.0.0 GA — выпущен 2026-06-12 ✅

**Релиз:** https://github.com/NAGenaev/tuck/releases/tag/v1.0.0

**Вошло в v1.0:**
- M0–M28 — вся функциональность ядра ✅
- M29 — Token MaxUses (одноразовые токены) ✅
- M30 — UI: Auth Methods + Dynamic Secrets ✅
- M31 — UI: Crypto Engines (PKI, Transit, SSH, TOTP) ✅
- M32 — CLI: полный охват API ✅
- OPS-6 — YAML config file ✅
- QA-2 — Load testing (Go benchmarks + k6) ✅
- SEC — govulncheck 0 CVE, gosec 0 issues (Go 1.25.11) ✅

**Не входит в v1.0 (перенесено в v1.x):**
- GitHub Auth (JWT/OIDC его покрывает)
- Entity & Identity system
- Namespace isolation
- Multi-tenant architecture
- CSI Provider

---

## Состояние v1.0.0 — 194 HTTP-эндпоинта

| Категория | Компонент | Статус |
|---|---|---|
| **Криптография** | Envelope encryption AES-256-GCM (barrier) | ✅ |
| **Хранилище** | bbolt backend + Raft HA (3–5 нод) | ✅ |
| **Seal** | Dev / Shamir / Transit | ✅ |
| **Seal** | AWS KMS (CMK, IRSA, auto-unseal) | ✅ |
| **Seal** | GCP Cloud KMS (Workload Identity, auto-unseal) | ✅ |
| **Seal** | Azure Key Vault (Managed Identity, auto-unseal) | ✅ |
| **Secrets** | KV v1 (простое хранилище) | ✅ |
| **Secrets** | KV v2 (версии, CAS, soft-delete, max_versions) | ✅ |
| **Secrets** | Cubbyhole (приватное хранилище токена, auto-purge) | ✅ |
| **Secrets** | Response Wrapping (одноразовые токены, tuck_wrap_) | ✅ |
| **Crypto** | Transit engine (AES-GCM, ECDSA, Ed25519, RSA-PSS, rewrap) | ✅ |
| **Crypto** | PKI engine (X.509 CA, CRL, role-based issuance) | ✅ |
| **Crypto** | SSH engine (CA-режим, user/host certs) | ✅ |
| **Crypto** | TOTP engine (RFC 6238, SHA1/256/512, 6/8 цифр) | ✅ |
| **Dynamic** | Database engine (PostgreSQL/MySQL, auto-revoke) | ✅ |
| **Dynamic** | AWS dynamic secrets (iam_user + assumed_role) | ✅ |
| **Dynamic** | GCP dynamic secrets (SA key + access_token) | ✅ |
| **Dynamic** | Azure dynamic secrets (AD client_secret, Graph API) | ✅ |
| **Auth** | Token auth (TTL, GC, accessor, renewable, MaxTTL) | ✅ |
| **Auth** | Kubernetes SA auth (TokenReview) | ✅ |
| **Auth** | JWT / OIDC auth (JWKS, role matching) | ✅ |
| **Auth** | AppRole auth (role_id + secret_id) | ✅ |
| **Auth** | LDAP / Active Directory auth | ✅ |
| **Policy** | ACL политики (glob-matching + deny rules) | ✅ |
| **K8s** | TuckSecret CRD + operator (leader election, conditions) | ✅ |
| **K8s** | Webhook Agent Injector (tmpfs, минует etcd) | ✅ |
| **K8s** | Helm chart | ✅ |
| **Observability** | Prometheus metrics (/metrics) | ✅ |
| **Observability** | OpenTelemetry tracing (OTLP) | ✅ |
| **Observability** | Audit log (SHA-256 hash chain, values never logged) | ✅ |
| **Operations** | TLS (self-signed ECDSA P-256 или custom cert) | ✅ |
| **Operations** | Graceful shutdown (30s drain + seal) | ✅ |
| **Operations** | Rate limiting (per-IP token bucket) | ✅ |
| **Operations** | Backup/restore (bbolt snapshot) | ✅ |
| **Operations** | Key rotation (POST /v1/sys/rotate) | ✅ |
| **UX** | Embedded web dashboard (/ui/) — PKI/Transit/SSH/TOTP/Auth/Dynamic | ✅ |
| **UX** | CLI client (tuckcli) — полный (kv/token/policy/sys/pki/transit/ssh/totp/auth/dynamic) | ✅ |
| **UX** | Go SDK (pkg/client) — полный (70+ методов) | ✅ |
| **UX** | OpenAPI 3.0 spec (/openapi.json) | ✅ |
| **Release** | goreleaser (linux/darwin/windows × amd64/arm64, cosign, SBOM) | ✅ |
| **Release** | CI pipeline (GitHub Actions: build, test -race, lint, gosec) | ✅ |
| **Docs** | Threat model (docs/THREAT_MODEL.md) | ✅ |
| **Docs** | Community files (SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md) | ✅ |

---

## UI — покрытие в v1.0

| Страница | Статус |
|---|---|
| Status (seal/unseal, rotate key) | ✅ |
| KV v1 secrets | ✅ |
| Tokens | ✅ |
| Policies | ✅ |
| AppRole / LDAP / JWT / K8s конфигурация | ✅ |
| Dynamic secrets (DB/AWS/GCP/Azure) + leases | ✅ |
| PKI (CA, issue cert, revoke) | ✅ |
| SSH CA + sign | ✅ |
| Transit (encrypt/decrypt/sign/verify) | ✅ |
| TOTP (create, code, validate) | ✅ |
| KV v2 (версии, CAS, metadata) | ✅ v1.4.0 |
| Response Wrapping | ✅ v1.4.0 |
| Namespaces management | ✅ v1.4.0 |
| Token Roles management | ✅ v1.4.0 |
| Audit Sinks management | ✅ v1.4.0 |
| Cluster management (Raft status, join/remove) | ✅ v1.4.0 |

## CLI — полный охват API в v1.0

| Команды | Статус |
|---|---|
| `kv get/put/delete/list` | ✅ |
| `token create/get/revoke/renew/list` | ✅ |
| `policy get/put/delete/list` | ✅ |
| `status/unseal/seal/rotate` | ✅ |
| `token lookup-self`, `token renew-self` | ✅ |
| `db/aws/gcp/azure creds <role>` | ✅ |
| `pki issue <role>`, `pki revoke <serial>` | ✅ |
| `transit encrypt/decrypt <key>` | ✅ |
| `ssh sign <role> <pubkey>` | ✅ |
| `totp code <key>` | ✅ |
| `auth approle/ldap/jwt login` | ✅ |

---

## Планы v1.x

| Что | Приоритет | Статус |
|---|---|---|
| Entity & Identity system | Высокий | ✅ v1.1.0 |
| Namespace isolation / multi-tenancy | Высокий | ✅ v1.2.0 |
| Audit log streaming (webhook/syslog sinks) | Высокий | ✅ v1.3.0 |
| KV v2 + Response Wrapping в UI | Средний | ✅ v1.4.0 |
| Cluster management в UI | Средний | ✅ v1.4.0 |
| Namespaces / Token Roles / Audit Sinks в UI | Средний | ✅ v1.4.0 |
| `mlockall` (защита root key от свопа) | Средний | ✅ v1.4.0 |
| Audit log rotation (size-based) | Низкий | ✅ v1.4.0 |
| UX improvements (UI auto-load, hints, templates, copy) | Высокий | ✅ v1.5.0 |
| Integration test suite (32 сценария, minikube lab) | Высокий | ✅ v1.5.0 |
| Russian setup guide | Средний | ✅ v1.5.0 |
| CSI Provider | Средний | ✅ v1.6.0 |
| GitHub Auth | Низкий | Планируется |
| Rate limiting на KV/token endpoints | Низкий | Планируется |

---

## Архитектурные принципы (не нарушать)

1. **Один бинарь** — никаких внешних зависимостей в runtime
2. **Всё через барьер** — plaintext никогда не касается физического хранилища
3. **Узкий barrierIface** — каждый движок видит только Get/Put/Delete/List
4. **11-эндпоинт паттерн** — все dynamic engines: config CRUD + role CRUD + creds + lease CRUD
5. **Фоновый GC каждые 15 минут** — истёкшие токены/leases/wrapping tokens
6. **Deny rules побеждают** — deny в любой политике блокирует, независимо от allow

---

## История спринтов (выполнено)

| Sprint | Версия | Что реализовано |
|---|---|---|
| M0–M4 | v0.1–v0.5 | Крипто-ядро, KV, токены, K8s auth, TuckSecret CRD, Shamir/Transit seal |
| M5–M7 | v0.5–v0.7 | TLS, graceful shutdown, CI, audit, metrics, backup, rate limiting, CLI, LIST endpoints |
| M8–M9 | v0.8–v0.9 | HA operator, embedded UI (базовый), KV v2, OTel, OpenAPI, Prometheus, threat model |
| M10 | v0.10 | Go SDK, goreleaser, release pipeline |
| M11 | v0.11 | Raft HA backend (3–5 нод) |
| M12 | v0.12 | Webhook Agent Injector (tmpfs, минует etcd) |
| M13 | v0.13 | JWT/OIDC auth, Helm chart |
| M14 | v0.14 | AppRole auth, Database dynamic secrets |
| M15 | v0.15 | PKI engine (X.509 CA) |
| M16 | v0.16 | Transit engine |
| M17 | v0.17 | SSH engine (CA-режим) |
| M18 | v0.18 | TOTP engine |
| M19 | v0.19 | AWS KMS + GCP Cloud KMS seal |
| M20 | v0.20 | LDAP/AD auth, Azure Key Vault seal |
| M21 | v0.21 | AWS dynamic secrets |
| M22 | v0.22 | GCP dynamic secrets |
| M23 | v0.23 | Azure dynamic secrets |
| M24 | v0.24 | Response Wrapping |
| M25 | v0.25 | Cubbyhole engine |
| M26 | v0.26 | Token Accessor |
| M27 | v0.27 | Policy Deny Rules |
| M28 | v0.28 | Renewable Tokens with MaxTTL |
| M29 | v0.29 | Token MaxUses (одноразовые и ограниченные токены) |
| M30 | v0.30 | UI: Auth Methods (AppRole/JWT/LDAP/K8s) + Dynamic Secrets (DB/AWS/GCP/Azure) + Leases |
| M31 | v0.31 | UI: PKI, Transit, SSH, TOTP engines |
| M32 | v0.32 | CLI: полный охват (db/aws/gcp/azure/pki/transit/ssh/totp/auth + lookup-self/renew-self) |
| OPS-6 | v0.33 | YAML config file (tuck.yaml), Getting Started guide |
| QA-2 | v0.34 | Load testing: Go benchmarks (in-process) + k6 script (smoke/load/stress/soak) |
| SEC | v1.0-rc | Security audit: govulncheck 0 CVE (Go 1.25.11), gosec 0 issues, docs/AUDIT.md |
| **GA** | **v1.0.0** | **General Availability — выпущен 2026-06-12** |
