# Tuck — Global Analysis & v1.0 Requirements

> Составлено: 2026-06-12 | Статус проекта: M0–M30 выполнены (v0.30.0)
> Следующие шаги: M31 → M32 → SEC-1 fix → QA → v1.0 GA

---

## 1. Что такое Tuck и зачем он нужен

**Tuck** — встроенный менеджер секретов для Kubernetes.

### Проблема, которую он решает

HashiCorp Vault — отраслевой стандарт, но требует:
- отдельного кластера (3–5 нод) с Consul или Integrated Storage
- DBA-уровня операционных знаний
- ~2–4 часов на initial setup даже для опытной команды
- платного Enterprise для ряда продуктовых фич (Namespaces, MFA, DR)

**Tuck решает эту проблему: один статический бинарь, `kubectl apply` — и работает.**

### Целевая аудитория

| Роль | Использование |
|---|---|
| Platform Engineer | Централизованное управление секретами k8s-кластера |
| DevOps / SRE | Альтернатива Vault без операционной сложности |
| Startup CTO | Быстрый старт без выделенной инфраструктуры для секретов |
| Security Engineer | Аудит-лог, policy-driven ACL, dynamic credentials |

### Ключевые дифференциаторы

1. **Zero external deps** — только bbolt (embedded), без etcd/Consul/PostgreSQL
2. **Single binary ~20 MB** — `kubectl apply` в 5 минут vs 4 часа на Vault
3. **k8s-native** — TuckSecret CRD + operator + webhook agent injector
4. **Vault-compatible mental model** — те же концепции (токены, политики, движки), иная операционная модель

---

## 2. Инвентаризация функций (текущее состояние M30)

### 2.1 Ядро системы

| Компонент | Статус | Описание |
|---|---|---|
| Envelope encryption (AES-256-GCM barrier) | ✅ | Всё хранилище зашифровано, plaintext никогда не пишется в bbolt |
| bbolt storage backend | ✅ | Встроенная BoltDB, single-file, ACID транзакции |
| Raft HA backend | ✅ | 3–5 нодовый кластер, leader election, snapshotting |
| Seal/Unseal: Dev mode | ✅ | Авто-unseal, для разработки |
| Seal/Unseal: Shamir SSS | ✅ | N-of-M разделение ключей |
| Seal/Unseal: Transit (Vault-compatible) | ✅ | Auto-unseal через внешний Transit-сервер |
| Seal/Unseal: AWS KMS | ✅ | IRSA, CMK, auto-unseal |
| Seal/Unseal: GCP Cloud KMS | ✅ | Workload Identity, auto-unseal |
| Seal/Unseal: Azure Key Vault | ✅ | Managed Identity, auto-unseal |

### 2.2 Secret Engines

| Движок | Статус | Ключевые возможности |
|---|---|---|
| KV v1 | ✅ | Простое key-value хранилище |
| KV v2 | ✅ | Версии, CAS, soft-delete, max_versions, undelete/destroy |
| Cubbyhole | ✅ | Приватное хранилище токена, auto-purge при revoke |
| Response Wrapping | ✅ | Одноразовые wrapping tokens (tuck_wrap_), TTL |
| Database | ✅ | PostgreSQL/MySQL, dynamic creds, auto-revoke by lease |
| AWS | ✅ | IAM user + assumed_role, dynamic creds |
| GCP | ✅ | SA key + access_token, dynamic creds |
| Azure | ✅ | AD client_secret, Graph API, dynamic creds |
| PKI | ✅ | X.509 CA (generate/import), role-based cert issuance, CRL |
| Transit | ✅ | AES-GCM, ECDSA, Ed25519, RSA-PSS, rewrap, HMAC |
| SSH | ✅ | CA-режим, user/host certificates, role-based signing |
| TOTP | ✅ | RFC 6238, SHA1/256/512, 6/8 цифр, validate |

### 2.3 Auth Methods

| Метод | Статус | Возможности |
|---|---|---|
| Token | ✅ | TTL, GC, accessor, renewable, MaxTTL, max_uses (M29) |
| Kubernetes SA | ✅ | TokenReview API, role по namespace+SA |
| JWT / OIDC | ✅ | JWKS, bound_subject/claims/audiences, role matching |
| AppRole | ✅ | role_id + secret_id, secret-id TTL/num_uses |
| LDAP / Active Directory | ✅ | Bind auth, group mapping, role matching |

### 2.4 Безопасность и операции

| Функция | Статус | Детали |
|---|---|---|
| TLS (auto self-signed или custom cert) | ✅ | ECDSA P-256, cert-manager совместимость |
| Audit log (SHA-256 hash chain) | ✅ | Tamper-evident, значения секретов не логируются |
| ACL политики (glob + deny rules) | ✅ | Deny-первый порядок, deny-wins (M27) |
| Rate limiting (per-IP token bucket) | ✅ | Auth + unseal endpoints |
| Graceful shutdown (SIGTERM drain) | ✅ | 30s дрейн, barrier seal при выходе |
| Backup/restore (bbolt snapshot) | ✅ | GET /v1/sys/snapshot |
| Key rotation (POST /v1/sys/rotate) | ✅ | Без остановки сервиса |
| Token GC (background reaper) | ✅ | Каждые 15 мин, истёкшие токены/leases |
| Prometheus metrics (/metrics) | ✅ | Счётчики, гистограммы, gauge tuck_sealed |
| OpenTelemetry tracing (OTLP) | ✅ | Опциональный экспорт спанов |
| Structured logging (slog + JSON) | ✅ | Request-ID, без секретов в логах |
| HTTP server hardening | ✅ | Таймауты, MaxHeaderBytes, Slowloris-защита |

### 2.5 Kubernetes-интеграция

| Компонент | Статус |
|---|---|
| TuckSecret CRD + operator | ✅ |
| Leader election (coordination.k8s.io/Lease) | ✅ |
| Status conditions (Synced, Ready, observedGeneration) | ✅ |
| Webhook Agent Injector (tmpfs, минует etcd) | ✅ |
| Helm chart | ✅ |

### 2.6 Release Engineering

| Компонент | Статус |
|---|---|
| CI (GitHub Actions: build, test -race, lint, gosec, govulncheck) | ✅ |
| goreleaser (linux/darwin/windows × amd64/arm64, cosign, SBOM) | ✅ |
| Distroless образы (non-root, RO-FS) | ✅ |
| CHANGELOG.md (keep-a-changelog) | ✅ |
| LICENSE (Apache-2.0) | ✅ |
| SECURITY.md + THREAT_MODEL.md | ✅ |
| CONTRIBUTING.md + CODE_OF_CONDUCT.md | ✅ |
| DR Runbook (docs/RUNBOOK.md) | ✅ |

### 2.7 UX

| Компонент | Статус | Покрытие |
|---|---|---|
| Embedded web dashboard (/ui/) | ⚠️ PARTIAL | ~60% API (M30 добавил Auth + Dynamic Secrets) |
| CLI client (tuckcli) | ⚠️ PARTIAL | ~50% команд (базовые kv/token/policy/sys) |
| Go SDK (pkg/client) | ✅ | 70+ методов, полное покрытие основных операций |
| OpenAPI 3.0 spec (/openapi.json) | ✅ | Актуально до M29 включительно |

---

## 3. Открытые проблемы до v1.0

### 3.1 КРИТИЧЕСКАЯ: SEC-1 — Token IDs в plaintext в хранилище

**Проблема**: Токены хранятся в bbolt под ключом `auth/token/<raw-token-id>`.
Поскольку token ID является одновременно bearer-credential, дамп `tuck.db`
раскрывает все валидные токены — даже несмотря на то, что значения зашифрованы
через barrier.

**Масштаб**: Злоумышленник с доступом к файлу `tuck.db` (например, через
скомпрометированный PVC) получает все активные токены.

**Исправление**: Хранить токены под `auth/token/SHA-256(token_id)`.
Token accessor (M26) уже существует как aliae для lookup/revoke без raw ID.
Нужно только изменить ключ хранения.

**Приоритет**: P0 — блокер для production-деплоя.

**Оценка**: 2–3 часа.

### 3.2 СРЕДНИЙ: OPS-6 — Конфигурация только через флаги

Все параметры передаются флагами командной строки. Transit-token и другие
чувствительные параметры видны в `ps aux` / через `/proc/PID/cmdline`.

**Исправление**: Поддержать конфиг-файл (YAML/HCL) и env vars.
Transit token только из env или файла, не из флага.

**Приоритет**: P1 — желательно перед GA.

**Оценка**: 3–4 часа.

### 3.3 НИЗКИЙ: SEC-6 — Нет mlockall

Memory zeroing есть. `mlockall` (защита от свопа) не реализован.

**Исправление**: `syscall.Mlockall` на Linux при флаге `--mlock`.

**Приоритет**: P2 — v1.x.

### 3.4 НИЗКИЙ: QA-2 — Нет нагрузочного тестирования

Нет k6/vegeta soak-тестов. Профиль производительности неизвестен.

**Исправление**: Написать k6 скрипт, провести 24h soak. Задокументировать RPS/p99.

**Приоритет**: P1 — желательно перед GA.

**Оценка**: 1 день.

### 3.5 ВНЕШНИЙ: QA-3 — Нет external security audit

Обязательное требование для v1.0. Никакой внутренний анализ не заменяет
независимый аудит крипто-ядра (barrier/seal/shamir/auth).

**Приоритет**: P0 — gate на GA.

---

## 4. Нужен ли UI?

**Ответ: Да, но не как основной интерфейс.**

### Для чего UI нужен (must-have)

| Сценарий | Почему UI лучше CLI/API |
|---|---|
| Первоначальная настройка (unseal, root token) | Визуальный feedback критичен при установке |
| Отладка политик и ролей | Итеративная работа, много форм |
| Мониторинг статуса кластера | Dashboard-вид удобнее curl |
| Просмотр и отзыв leases | Таблица с массовыми операциями |
| PKI — выпуск и отзыв сертификатов | Форма с валидацией X.509 полей |
| TOTP — QR-код для добавления в authenticator | QR нельзя показать в CLI |
| Onboarding новых операторов | Снижает порог входа |

### Для чего UI не нужен (nice-to-have, CLI достаточно)

| Сценарий | Почему CLI достаточно |
|---|---|
| CI/CD пайплайны | Автоматизация требует CLI/SDK, не UI |
| Dynamic creds (db/aws/gcp) в prod | Скрипты, Terraform провайдер |
| Массовые операции с секретами | CLI + bash скрипты |
| k8s-оператор sync | Полностью автоматизирован |

### Текущее покрытие UI и пробелы

| Страница | Статус после M30 |
|---|---|
| Status (seal/unseal/rotate) | ✅ |
| KV Secrets (v1) | ✅ |
| Tokens (CRUD + renew + accessor) | ✅ |
| Policies (CRUD + deny rules) | ✅ |
| Auth Methods (AppRole/JWT/LDAP/K8s) | ✅ M30 |
| Dynamic Secrets (DB/AWS/GCP/Azure + Leases) | ✅ M30 |
| **PKI (CA, issue, revoke, CRL)** | ❌ M31 |
| **Transit (encrypt/decrypt/sign)** | ❌ M31 |
| **SSH (CA, sign cert)** | ❌ M31 |
| **TOTP (create, QR, live code, validate)** | ❌ M31 |
| KV v2 (versions, CAS, metadata) | ❌ P2/v1.x |
| Response Wrapping | ❌ P2/v1.x |
| Cluster management (Raft nodes) | ❌ P2/v1.x |
| System Snapshot (backup/restore UI) | ❌ P2/v1.x |

**Вывод**: M31 закрывает последние значимые пробелы. После M31 UI покрывает ~85%
всех use-cases оператора.

---

## 5. Нужен ли CLI?

**Ответ: Да, CLI критичен.**

CLI — единственный способ использовать Tuck в:
- CI/CD пайплайнах (GitHub Actions, GitLab CI, Jenkins)
- скриптах инициализации агентов
- автоматизации rotate/unseal
- интерактивной отладке из терминала

### Текущее покрытие CLI

| Группа команд | Статус |
|---|---|
| kv get/put/delete/list | ✅ |
| token create/get/revoke/renew/list | ✅ |
| policy get/put/delete/list | ✅ |
| status/unseal/seal/rotate | ✅ |
| **token lookup-self, renew-self** | ❌ M32 |
| **db/aws/gcp/azure creds** | ❌ M32 |
| **pki issue/revoke** | ❌ M32 |
| **transit encrypt/decrypt** | ❌ M32 |
| **ssh sign** | ❌ M32 |
| **totp code** | ❌ M32 |
| **auth approle/ldap/jwt login** | ❌ M32 |

**14 команд** отсутствуют. Без них половина функционала недоступна из CI/CD.

---

## 6. Требования к v1.0 GA

### 6.1 Функциональные требования (все реализованы в M0–M30)

- [x] Encrypted-at-rest key-value secret store
- [x] Versioned secrets (KV v2)
- [x] Dynamic credentials: DB, AWS, GCP, Azure
- [x] Crypto engines: PKI, Transit, SSH, TOTP
- [x] Auth: Token, K8s, JWT/OIDC, AppRole, LDAP
- [x] ACL policies with deny rules
- [x] Token lifecycle: TTL, renewal, accessor, MaxTTL, max_uses
- [x] Cubbyhole + Response Wrapping
- [x] Kubernetes CRD operator + webhook injector

### 6.2 Безопасность (P0 блокеры)

- [ ] **SEC-1**: Token IDs не должны быть в plaintext в storage
- [x] TLS everywhere
- [x] Audit log with hash chain
- [x] Rate limiting on auth endpoints
- [x] Tamper-evident storage (barrier encryption)
- [ ] **QA-3**: External security audit пройден

### 6.3 UX (P1 для GA)

- [x] Web dashboard (Status, Secrets, Tokens, Policies, Auth, Dynamic Secrets)
- [ ] **M31**: Web dashboard Crypto Engines (PKI, Transit, SSH, TOTP)
- [x] CLI: базовые операции
- [ ] **M32**: CLI: dynamic creds, crypto ops, auth logins
- [x] Go SDK: полное покрытие

### 6.4 Операционные требования

- [x] Single-binary deployment
- [x] Helm chart
- [x] HA mode (Raft 3–5 nodes)
- [x] Auto-unseal (AWS KMS, GCP KMS, Azure Key Vault)
- [x] Backup/restore (bbolt snapshot)
- [x] Prometheus metrics + OTel tracing
- [x] Graceful shutdown
- [x] Distroless non-root container
- [x] Multi-arch release (linux/darwin/windows × amd64/arm64)

### 6.5 Документация

- [x] THREAT_MODEL.md
- [x] SECURITY.md + vulnerability disclosure
- [x] RUNBOOK.md (DR procedures)
- [x] CONTRIBUTING.md + CODE_OF_CONDUCT.md
- [ ] **Getting Started Guide** (нет пошагового tutorial)
- [ ] **API Reference** (OpenAPI spec нужно обновить до M30)

---

## 7. Граница v1.0 — что включено, что нет

### Включено в v1.0

```
M0–M30   ← всё реализованное ядро (194 HTTP endpoints)
M31      ← UI: Crypto Engines (PKI/Transit/SSH/TOTP)
M32      ← CLI completeness (14 новых команд)
SEC-1    ← token storage hashing (security fix, P0)
OPS-6    ← config file support (P1)
QA-3     ← external security audit (gate)
Getting Started Guide
OpenAPI spec update
```

### НЕ включено в v1.0 (переносится в v1.x)

```
GitHub Auth (JWT/OIDC покрывает большинство случаев)
Entity & Identity system (groups, aliases)
Namespace isolation / Multi-tenancy
CSI Provider
Terraform provider
KV v2 UI
Response Wrapping UI
Cluster management UI
mlockall (SEC-6)
govulncheck в CI (сейчас есть gosec)
```

---

## 8. План реализации до v1.0

### Текущий спринт (немедленно)

| # | Задача | Оценка | Приоритет |
|---|---|---|---|
| M31 | UI: Crypto Engines (PKI/Transit/SSH/TOTP) | 1 день | P1 |
| M32 | CLI: 14 новых команд | 1 день | P1 |
| SEC-1 | Token storage hashing | 3 часа | P0 |

### Следующий спринт

| # | Задача | Оценка | Приоритет |
|---|---|---|---|
| OPS-6 | Config file (YAML) | 4 часа | P1 |
| QA-2 | k6 load test + soak | 1 день | P1 |
| Docs | Getting Started Guide | 4 часа | P1 |
| Docs | OpenAPI spec update | 2 часа | P1 |

### Gate: v1.0-rc

После завершения обоих спринтов:
1. Провести внешний security audit (QA-3)
2. Устранить находки
3. Тег v1.0-rc, замороженный API
4. 72h soak test

### v1.0 GA

Выпуск после прохождения QA-3 без High/Critical findings.

---

## 9. Архитектурные принципы (не нарушать)

1. **Один бинарь** — никаких runtime-зависимостей
2. **Всё через барьер** — plaintext никогда не касается bbolt
3. **Узкий barrierIface** — каждый движок видит только Get/Put/Delete/List
4. **11-endpoint паттерн** — все dynamic engines: config CRUD + role CRUD + creds + lease CRUD
5. **Фоновый GC каждые 15 минут** — истёкшие токены/leases/wrapping tokens
6. **Deny rules wins** — deny в любой политике блокирует независимо от allow
7. **Ключевой материал зануляется** — после использования, не держать в памяти
8. **Audit перед операцией** — fail-closed: нет audit → нет операции

---

## 10. Метрики готовности к v1.0

| Измерение | Цель | Текущее |
|---|---|---|
| API coverage в UI | 85% | ~60% |
| CLI coverage | 100% documented ops | ~50% |
| Security: token storage | Hardened | ⚠️ plaintext IDs |
| Test coverage: api pkg | ≥80% | ~75% (интеграционные) |
| External audit | Passed | ⏳ не проводился |
| Getting Started: time to first secret | < 10 min | ~15 min |
| Single-node startup time | < 2 sec | ~0.3 sec ✅ |
| p99 latency (kv get, single node) | < 5ms | ~1ms ✅ |
