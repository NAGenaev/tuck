# Tuck — Production Roadmap

> Состояние: M0–M29 завершены (v0.29.0). Следующий шаг: M30-M32 (UX completeness) → security audit → v1.0 GA.

---

## Что такое Tuck

Tuck — встроенный менеджер секретов для Kubernetes. Анти-Vault: один статический бинарь (~20 MB),
без Consul, без etcd, без внешних зависимостей. `kubectl apply` — и работает.

**Целевая аудитория:** платформенные инженеры, DevOps-команды, SRE, которым нужна
альтернатива HashiCorp Vault без операционной сложности.

---

## Граница v1.0 (чёткое определение)

v1.0 GA = функциональная полнота + операционная зрелость + внешний security audit.

**Включено в v1.0:**
- M0–M28 (реализовано) — вся функциональность ядра
- M29 — Token MaxUses (безопасность: одноразовые токены)
- M30 — UI: Auth Methods + Dynamic Secrets (операторский UI)
- M31 — UI: Crypto Engines (PKI, Transit, SSH, TOTP в браузере)
- M32 — CLI: dynamic creds + crypto ops (полный CLI)
- Внешний security audit (QA-3)

**Не входит в v1.0 (v1.x):**
- GitHub Auth (JWT/OIDC его покрывает)
- Entity & Identity system
- Namespace isolation
- Multi-tenant architecture
- CSI Provider

---

## Текущее состояние (v0.28.0) — 194 HTTP-эндпоинта

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
| **UX** | Embedded web dashboard (/ui/) — частично (~20%) | ⚠️ |
| **UX** | CLI client (tuckcli) — частично (kv/token/policy/sys) | ⚠️ |
| **UX** | Go SDK (pkg/client) — полный (70+ методов) | ✅ |
| **UX** | OpenAPI 3.0 spec (/openapi.json) | ✅ |
| **Release** | goreleaser (linux/darwin/windows × amd64/arm64, cosign, SBOM) | ✅ |
| **Release** | CI pipeline (GitHub Actions: build, test -race, lint, gosec) | ✅ |
| **Docs** | Threat model (docs/THREAT_MODEL.md) | ✅ |
| **Docs** | Community files (SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md) | ✅ |

---

## Реальные пробелы до v1.0

### UI — только 20% API доступно через браузер

| Страница | Состояние |
|---|---|
| Status (seal/unseal, rotate key) | ✅ |
| KV v1 secrets | ✅ |
| Tokens | ✅ |
| Policies | ✅ |
| KV v2 (версии, CAS, metadata) | ❌ |
| AppRole / LDAP / JWT / K8s конфигурация | ❌ |
| Dynamic secrets (DB/AWS/GCP/Azure) + leases | ❌ |
| PKI (CA status, issue cert, revoke, CRL) | ❌ |
| SSH CA + sign | ❌ |
| Transit (encrypt/decrypt/sign через форму) | ❌ |
| TOTP (QR-код, validate) | ❌ |
| Response Wrapping | ❌ |
| Cluster management (Raft status, join/remove) | ❌ |

### CLI — только базовые операции

| Команды | Состояние |
|---|---|
| `kv get/put/delete/list` | ✅ |
| `token create/get/revoke/renew/list` | ✅ |
| `policy get/put/delete/list` | ✅ |
| `status/unseal/seal/rotate` | ✅ |
| `token lookup-self`, `token renew-self` | ❌ |
| `db creds <role>`, `aws creds <role>`, etc. | ❌ |
| `pki issue <role>`, `pki revoke <serial>` | ❌ |
| `transit encrypt/decrypt <key>` | ❌ |
| `ssh sign <role> <pubkey>` | ❌ |
| `totp code <key>` | ❌ |
| `auth approle login`, `auth ldap login` | ❌ |

### Token MaxUses — отсутствует

Нет ограничения числа использований токена (`num_uses`). Критично для:
- Bootstrap-токенов (одноразовые для инициализации агентов)
- AppRole-паттернов, где секрет выдаётся один раз
- Минимизации blast radius при компрометации

---

## Дорожная карта к v1.0

### M29 — Token MaxUses (v0.29)

Токены с ограничением числа использований. `max_uses: 1` = одноразовый токен,
который умирает после первого аутентифицированного API-вызова.

**Изменения:**
- `Token.MaxUses int` (0 = без ограничений), `Token.UseCount int` (счётчик использований)
- `Authenticate` инкрементирует UseCount; при UseCount > MaxUses — токен отзывается
- `WithMaxUses(n) TokenOpt` для `CreateToken`
- `POST /v1/auth/token` принимает `max_uses` в запросе
- `tuckcli token create --max-uses=N`

### M30 — UI: Auth Methods + Dynamic Secrets (v0.30)

Добавить в embedded dashboard:
- **Auth Methods:** AppRole (создание ролей, генерация secret-id), K8s (роли), LDAP/JWT (конфигурация)
- **Dynamic Secrets:** DB/AWS/GCP/Azure (конфигурация движков, создание ролей, генерация credentials)
- **Leases:** общий список активных leases с возможностью ревокации

### M31 — UI: Crypto Engines (v0.31)

Добавить в embedded dashboard:
- **PKI:** статус CA, выпуск/отзыв сертификатов, просмотр CRL
- **Transit:** encrypt/decrypt/sign через форму, ротация ключей
- **SSH:** CA pubkey, подпись публичного ключа
- **TOTP:** создание ключей, QR-код, валидация кода

### M32 — CLI Completeness (v0.32)

Расширить `tuckcli`:
- `tuckcli db creds <role>` — получить credentials из Database engine
- `tuckcli aws creds <role>`, `gcp creds <role>`, `azure creds <role>`
- `tuckcli pki issue <role> --cn=...` — выпустить TLS-сертификат
- `tuckcli transit encrypt <key> <plaintext>` / `decrypt <key> <ciphertext>`
- `tuckcli ssh sign <role> <pubkey-file>` — подписать SSH-ключ
- `tuckcli totp code <key>` — получить текущий TOTP-код
- `tuckcli token lookup-self`, `tuckcli token renew-self`
- `tuckcli auth approle login --role-id=... --secret-id=...`

### v1.0-rc — Hardening + Audit

- Обновить OpenAPI spec с новыми эндпоинтами (M29)
- Написать Getting Started guide
- Провести внешний security audit (QA-3)
- **v1.0 GA** после прохождения аудита

---

## Оценка времени

| Milestone | Что | Оценка |
|---|---|---|
| **M29** | Token MaxUses | 2-3 часа |
| **M30** | UI Auth + Dynamic Secrets | 1 день |
| **M31** | UI Crypto Engines | 1 день |
| **M32** | CLI completeness | 1 день |
| **v1.0-rc** | Docs, hardening | 0.5 дня |
| **v1.0 GA** | После security audit | — |

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
