# Tuck — Production Roadmap

> Состояние: M0–M22 завершены (v0.22.0). Следующий шаг: внешний security audit (QA-3) → v1.0 GA.

---

## Текущее состояние (v0.22.0)

| Компонент | Статус |
|---|---|
| Envelope encryption AES-256-GCM (barrier) | ✅ |
| bbolt backend + Raft HA backend (3–5 нод) | ✅ |
| Dev / Shamir / Transit seal | ✅ |
| KV v1 + KV v2 (версии, CAS, soft-delete, max_versions) | ✅ |
| Token auth, ACL политики (glob-matching) | ✅ |
| Kubernetes SA auth (TokenReview) | ✅ |
| JWT / OIDC auth (JWKS, role matching) | ✅ |
| AppRole auth (role_id + secret_id) | ✅ |
| TuckSecret CRD + operator (leader election, conditions, deletion policy) | ✅ |
| Webhook Agent Injector (tmpfs, минует etcd) | ✅ |
| Helm chart | ✅ |
| Database engine (PostgreSQL / MySQL, auto-revoke) | ✅ |
| PKI engine (X.509 CA, CRL, role-based issuance) | ✅ |
| Transit engine (AES-GCM, ECDSA, Ed25519, RSA-PSS, rewrap) | ✅ |
| SSH engine (CA-режим, user/host certs, TrustedUserCAKeys) | ✅ |
| TOTP engine (RFC 6238, SHA1/256/512, 6/8 цифр) | ✅ |
| AWS KMS seal (CMK, IRSA, auto-unseal) | ✅ |
| GCP Cloud KMS seal (Workload Identity, auto-unseal) | ✅ |
| Azure Key Vault seal (Managed Identity / DefaultAzureCredential, auto-unseal) | ✅ |
| LDAP / Active Directory auth (bind-search-bind, group→policy roles) | ✅ |
| AWS dynamic secrets (iam_user + assumed_role, auto-revoke, lease GC) | ✅ |
| GCP dynamic secrets (service_account_key + access_token, auto-revoke, lease GC) | ✅ |
| TLS (self-signed ECDSA P-256 или custom cert) | ✅ |
| Graceful shutdown (30s drain + seal) | ✅ |
| Audit log (SHA-256 hash chain, values never logged) | ✅ |
| Rate limiting (per-IP token bucket) | ✅ |
| Backup/restore (bbolt snapshot) | ✅ |
| Key rotation (POST /v1/sys/rotate) | ✅ |
| Prometheus metrics (/metrics) | ✅ |
| OpenTelemetry tracing (OTLP) | ✅ |
| Embedded web dashboard (/ui/) | ✅ |
| CLI client (tuckcli) | ✅ |
| Go SDK (pkg/client) | ✅ |
| OpenAPI 3.0 spec (/openapi.json) | ✅ |
| goreleaser (linux/darwin/windows × amd64/arm64, cosign, SBOM) | ✅ |
| CI pipeline (GitHub Actions: build, test -race, lint, gosec) | ✅ |
| Threat model (docs/THREAT_MODEL.md) | ✅ |
| Community files (SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md) | ✅ |

---

## Оператор: CRD vs Vault подход

Текущий оператор создаёт K8s Secret из TuckSecret CRD (паттерн external-secrets-operator).

**Vault** использует два других механизма:
- **Agent Injector** — MutatingWebhookConfiguration инжектит sidecar, секрет монтируется как файл в `/vault/secrets/` (tmpfs, минует etcd)
- **CSI Provider** — монтирует через Secret Store CSI Driver как volume

### Анализ текущего подхода (CRD → K8s Secret)

| Проблема | Серьёзность | Решение |
|---|---|---|
| Секрет попадает в etcd (видно в бэкапах) | ⚠️ Средняя | Документировать; etcd-at-rest шифрование в K8s по умолчанию есть |
| Нет leader election (2+ реплики → двойная запись) | ⚠️ Высокая | OP-1: Lease-based leader election |
| Оператор должен знать все пути | ⚠️ Средняя | Ограниченная политика в role binding |
| Нет условий Ready/Synced в status | ⚠️ Средняя | OP-2: Status conditions |

### Вывод

**CRD-подход достаточен для v1.0.** Webhook Injector — опциональный v1.x feature.

Etcd шифруется at-rest в K8s. Это контролируемое компромисс-решение. Audit-лог в Tuck отслеживает кто/когда читал. Нужны leader election и status conditions.

---

## Production Gaps

### P0 — Блокеры (без них нельзя в продакшн)

| ID | Что не хватает | Файл(ы) | Размер |
|---|---|---|---|
| **SEC-1** | Токены в bbolt по открытому ID → дамп БД раскрывает валидные токены; нужен HMAC-SHA256(token, barrierKey) как ключ хранения + accessor | `internal/token/store.go`, `internal/core/core.go` | M |
| **SEC-2** | HTTP — весь трафик plaintext; нужен TLS/mTLS | `cmd/tuck/main.go`, `internal/api/server.go` | M |
| **SEC-4** | Нет HTTP таймаутов (Slowloris-уязвимость) | `internal/api/server.go` | S |
| **OPS-1** | Нет graceful shutdown — процесс убивается без дренажа | `cmd/tuck/main.go`, `cmd/tuck-operator/main.go` | S |
| **OPS-2** | `/v1/health` не различает liveness vs readiness | `internal/api/server.go` | S |
| **REL-1** | Нет CI pipeline (build, test, lint, security scan) | `.github/workflows/` | M |
| **REL-2** | Нет LICENSE файла | корень проекта | S |

### P1 — GA v1.0

| ID | Что не хватает | Файл(ы) | Размер |
|---|---|---|---|
| **SEC-3** | Нет audit-лога (tamper-evident) — нельзя ответить "кто читал секрет X" | новый `internal/audit/` | L |
| **SEC-5** | Нет rate limiting на auth endpoints (brute-force токенов) | новый `internal/ratelimit/` | M |
| **SEC-6** | Root key не обнуляется в памяти после use (memory hygiene) | `internal/barrier/`, `internal/core/core.go` | M |
| **SEC-7** | Нет rotation: barrier rotate (новый DEK) и rekey (новые Shamir-шарды) | `internal/barrier/`, `internal/seal/shamir.go` | M |
| **SEC-8** | Нет Threat Model и Security Policy | `docs/THREAT_MODEL.md`, `SECURITY.md` | S |
| **OPS-3** | Нет backup/restore | `internal/api/sys.go`, physical layer | M |
| **OPS-4** | Истёкшие токены накапливаются (нет GC reaper) | `internal/token/store.go` | M |
| **OPS-5** | Конфигурация через флаги — sensitive values видны в `ps aux` | новый `internal/config/`, HCL/YAML | M |
| **OPS-6** | Shamir N и K захардкожены — нет флагов | `cmd/tuck/main.go`, `internal/seal/shamir.go` | S |
| **OBS-1** | Нет Prometheus метрик | новый `internal/metrics/`, `internal/api/server.go` | M |
| **OBS-2** | Неструктурированный лог (stdlib `log`) — нет request-id, нет JSON | весь код | M |
| **OP-1** | Оператор: нет leader election (2+ реплики → дублирование) | `internal/operator/controller.go` | M |
| **OP-2** | Оператор: статус CRD не отражает реальность (нет conditions) | `internal/operator/`, `deploy/crd.yaml` | M |
| **OP-3** | Оператор: fixed 5s retry без exponential backoff | `internal/operator/controller.go` | S |
| **API-1** | Binary secrets: значение идёт как JSON-строка → не-UTF8 байты ломаются | `internal/api/kv.go` | S |
| **API-3** | Нет LIST эндпоинтов (секреты, токены, политики) | `internal/api/`, `internal/core/core.go` | M |
| **API-4** | Нет token renew / lease renewal | `internal/token/`, `internal/core/core.go` | M |
| **API-5** | Нет CLI-клиента (только curl) | новый `cmd/tuck/` или `cmd/tuck-cli/` | M |
| **REL-3** | Нет release pipeline (подписанные бинари, образы, SBOM) | `.goreleaser.yaml`, GitHub Actions | M |
| **REL-4** | Docker образы: не distroless, root user | `build/Dockerfile.*` | S |
| **REL-5** | Нет semver, CHANGELOG, API stability policy | `CHANGELOG.md`, `cmd/tuck/main.go` (version flag) | S |
| **QA-1** | Нет race detector, fuzz, coverage gate в CI | `.github/workflows/` | M |
| **QA-2** | Нет load/soak тестов | `test/load/` | M |
| **QA-3** | Нет внешнего security audit | - | L |

### P2 — v1.x (после GA)

| ID | Что не хватает | Размер |
|---|---|---|
| **HA-1** | Raft-реплицируемый backend (встроенный, без etcd) | XL |
| **API-2** | KV v2: версии, undelete, check-and-set | L |
| **API-6** | Go SDK + OpenAPI spec | M |
| **OP-4** | Webhook Agent Injector (sidecar, минует etcd) | L |
| **OP-5** | Deletion policy на TuckSecret (Retain vs Delete K8s Secret) | S |
| **OBS-3** | OpenTelemetry tracing (OTLP) | M |
| **UI-1** | Embedded web dashboard | M |
| **REL-6** | Community файлы (CONTRIBUTING, CODE_OF_CONDUCT, CODEOWNERS) | S |
| **QA-4** | Disaster recovery runbook | S |

---

## Embedded Web UI (P2)

Простой dashboard встроенный в бинарь сервера.

**Технология:** Vanilla JS или Preact (~5KB), go:embed, сборка через esbuild без Node.js в runtime.

```
cmd/tuck/
  ui/
    index.html      ← single-page app
    app.js          ← компоненты (Seal status, KV, Tokens, Policies)
    api.js          ← обёртка над HTTP API
  main.go           ← добавить //go:embed ui/dist/*
```

**Функционал MVP:**
- Seal status с real-time polling, форма ввода Shamir-шарда
- Просмотр секретов (list + get, без values в логах)
- Управление токенами и политиками
- Viewer audit-лога
- Кнопка Manual Seal

**Аутентификация:** X-Tuck-Token в sessionStorage, auto-logout при 401.

---

## Milestones

### M5 — Security & DevOps Baseline (v0.5)

*Цель: минимум безопасности для early adopters.*

- **SEC-2:** TLS — флаги `-tls-cert`, `-tls-key`, `-tls-auto` (self-signed для dev); `http.ListenAndServeTLS`
- **SEC-4:** HTTP таймауты — `ReadHeaderTimeout: 5s`, `ReadTimeout: 30s`, `WriteTimeout: 30s`, `IdleTimeout: 5m`
- **OPS-1:** Graceful shutdown — `signal.NotifyContext` + `srv.Shutdown(ctx)` в обоих main.go
- **OPS-2:** Liveness vs readiness — `GET /v1/sys/health` (liveness), `GET /v1/sys/ready` (readiness, 503 если sealed)
- **OPS-6:** Shamir флаги — `-seal-type`, `-seal-shamir-n`, `-seal-shamir-k`, `-seal-transit-wrap-url`
- **REL-1:** CI — GitHub Actions: `go build ./...`, `go test -race ./...`, `golangci-lint`, `gosec`, `govulncheck`, kind e2e
- **REL-2:** LICENSE — Apache-2.0
- **REL-4:** Dockerfile hardening — distroless, uid 65532, read-only rootfs

**Результат:** K8s-ready, open-source ready. Можно деплоить с явными known limitations (нет audit-лога, нет backup).

---

### M6 — Reliability & Observability (v0.6)

*Цель: операционная полнота для production.*

- **SEC-3:** Audit-лог — `internal/audit/`: hash-chain (каждая запись = SHA256(prev_hash || entry)), JSON-строки, fail-closed (пишем в файл или stdout), **никогда не логировать values**
- **SEC-5:** Rate limiting — token bucket на `/v1/auth/*` и `/v1/sys/unseal`, exponential backoff после N неудач
- **SEC-6:** Memory hygiene — `secure.Zero()` на копии root key, опциональный `mlockall`
- **SEC-1:** HMAC-токены — ключ хранения `HMAC-SHA256(token_id, barrier_dek)` + accessor для lookup без знания токена
- **OPS-3:** Backup/restore — `GET /v1/sys/snapshot` (bbolt `Tx.WriteTo`), `POST /v1/sys/restore`
- **OPS-4:** Token GC — фоновый reaper каждые 15 мин, удаляет `expires_at < now`
- **OPS-5:** Config file — HCL или YAML, env vars для секретов (`TUCK_SEAL_TRANSIT_TOKEN`)
- **OBS-1:** Prometheus — `/metrics`: request latency/count по route+code, `tuck_barrier_sealed`, `tuck_auth_failures_total`, оператор: sync stats
- **OBS-2:** Structured logging — заменить `log` на `log/slog`, JSON output, request-id (X-Request-Id)

**Результат:** Уверенный production. Есть audit trail, backup, метрики, структурированные логи.

---

### M7 — Operator & API Completeness (v0.7–v0.8)

*Цель: полнофункциональный инструмент.*

- **OP-1:** Leader election — `coordination.k8s.io/v1 Lease`, только лидер реконсайлит
- **OP-2:** Status conditions — `.status.conditions[Synced/Ready]`, `observedGeneration`, `lastError`
- **OP-3:** Exponential backoff — `wait.ExponentialBackoff` в reconcile loop
- **API-1:** Binary-safe secrets — `{"value": "<base64>", "encoding": "base64"}` или `Content-Type: application/octet-stream`
- **API-3:** LIST эндпоинты — `GET /v1/secret/?prefix=db/` (список ключей, без values), LIST токенов/политик
- **API-4:** Token renew — `POST /v1/auth/token/renew`, `max_ttl`, каскадный revoke child-токенов
- **SEC-7:** Key rotation — `POST /v1/sys/rotate` (новый DEK + перешифровка keyring), `POST /v1/sys/rekey` (новые Shamir-шарды без рестарта)
- **API-5:** CLI-клиент —

  ```
  tuck status
  tuck login --addr=... --method=k8s|token
  tuck kv get secret/db/password
  tuck kv put secret/db/password value=s3cr3t
  tuck kv list secret/db/
  tuck token create --policy=db-rw --ttl=24h
  tuck policy write db-rw policy.hcl
  tuck operator unseal --key=<base64-shard>
  tuck operator seal
  ```

- **REL-3:** Release pipeline — goreleaser: linux/darwin/windows amd64/arm64, checksums, cosign sign, SBOM syft
- **REL-5:** Versioning — semver, `CHANGELOG.md` (keep-a-changelog), `--version` флаг, storage schema version
- **QA-1:** Race/fuzz/coverage — `-race` в CI, fuzz на парсеры, coverage gate ≥ 70% на crypto/auth пакеты

**Результат:** Production-grade. Есть CLI, ротация ключей, полный API, release pipeline.

---

### M8 — Pre-GA Hardening (v0.9)

*Цель: enterprise-ready, внешний audit.*

- **SEC-8:** Threat Model — `docs/THREAT_MODEL.md`: активы (root key, DEK, tokens), границы доверия, атаки (brute-force, side-channel, backup leak), in/out of scope
- **SECURITY.md** — политика раскрытия уязвимостей (coordinated disclosure, contact email)
- **QA-2:** Load/soak — k6 или vegeta: 5000 RPS / 24h soak без утечки памяти
- **QA-3:** Security audit — независимый review barrier/seal/shamir/auth
- **UI-1:** Embedded dashboard (Preact, go:embed)
- **REL-6:** Community — `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, issue templates, CODEOWNERS
- **QA-4:** Runbook — `docs/RUNBOOK.md`: unseal, backup/restore, token rotation, incident response

**Результат:** v1.0 GA. Внешний audit пройден, community-ready.

---

## Первый спринт (1 неделя)

**Строго в этом порядке:**

1. **День 1:** SEC-2 — TLS
   - Флаги `-tls-cert`, `-tls-key` в `cmd/tuck/main.go`
   - `-tls-auto` = auto-generate self-signed для dev
   - `http.Server{TLSConfig: ...}` в `internal/api/server.go`

2. **День 1–2:** OPS-1 + OPS-2 — Graceful shutdown + readiness split
   - `signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)` в main.go
   - `srv.Shutdown(shutdownCtx)` с 30s таймаутом
   - Добавить `GET /v1/sys/ready` (sealed → 503)
   - K8s manifest: `livenessProbe: /v1/health`, `readinessProbe: /v1/sys/ready`

3. **День 2:** SEC-4 — HTTP таймауты
   ```go
   &http.Server{
       ReadHeaderTimeout: 5 * time.Second,
       ReadTimeout:       30 * time.Second,
       WriteTimeout:      30 * time.Second,
       IdleTimeout:       5 * time.Minute,
       MaxHeaderBytes:    1 << 20,
   }
   ```

4. **День 2–3:** OPS-6 — Shamir и transit seal через флаги
   - `-seal-type=dev|shamir|transit`
   - `-seal-shamir-n=5 -seal-shamir-k=3`
   - `-seal-transit-wrap-url=... -seal-transit-unwrap-url=... -seal-transit-token=...`
   - `-seal-transit-key-file=...` (где хранить wrapped key)

5. **День 3–4:** REL-1 — CI pipeline
   ```yaml
   # .github/workflows/ci.yml
   jobs:
     test:
       steps:
         - go build ./...
         - go test -race -count=1 ./...
         - golangci-lint run
         - gosec ./...
         - govulncheck ./...
   ```

6. **День 4–5:** REL-2 + REL-4 — LICENSE + Dockerfile hardening
   - Выбрать `Apache-2.0`
   - Dockerfile: `FROM gcr.io/distroless/static:nonroot`, `USER 65532`

**Результат после спринта:** TLS работает, graceful shutdown, CI блокирует PR на красном, open-source чистый.

---

## Риски

| Риск | Влияние | Митигация |
|---|---|---|
| SEC-1 (HMAC-токены) — breaking change для существующих деплоев | Высокое | Документировать migration guide; версионировать API |
| HA-1 (Raft) требует значительной архитектурной работы | Высокое | Отложить до v1.x; документировать как known limitation |
| OP-1 (leader election) требует `coordination.k8s.io` ClusterRole | Среднее | Добавить в operator RBAC манифест |
| Audit-лог растёт без ограничений | Среднее | Ротация, retention policy, flush interval |
| Внешний security audit может найти критические проблемы | Высокое | Начать М5–М6 раньше; не анонсировать GA до audit |

---

## Временная оценка

| Milestone | Объём | Хардкорный дедлайн |
|---|---|---|
| **M5** Security & DevOps baseline | ~2 недели | v0.5 |
| **M6** Reliability & Observability | ~3 недели | v0.6 |
| **M7** API & Operator completeness | ~4 недели | v0.8 |
| **M8** Pre-GA hardening + audit | ~3 недели | v0.9–v1.0 |
| **Total** | **~12 недель** | **v1.0 GA** |

Минимум для "первого осторожного production деплоя": **M5** (TLS + graceful + CI).
Минимум для "уверенного production": **M5 + M6** (добавляет audit-лог + backup).
