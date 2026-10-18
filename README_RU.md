# Tuck

> Самый простой менеджер секретов для Kubernetes. Спрячь секрет — без лишних ритуалов.

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Release](https://img.shields.io/badge/release-v2.1.0-green)](https://github.com/NAGenaev/tuck/releases)

Tuck — open-source менеджер секретов для Kubernetes. Главная идея: **анти-Vault** — один статический бинарь, никакой внешней базы данных, автоматическое распечатывание по умолчанию. `kubectl apply` — и работает.

[English documentation →](README.md)

---

## Зачем ещё один менеджер секретов?

[HashiCorp Vault](https://www.vaultproject.io) мощный, но операционно тяжёлый: кворум Consul/Raft, ручное распечатывание после каждого рестарта, сложная модель ACL. [OpenBao](https://openbao.org) наследует ту же сложность. [Infisical](https://infisical.com) требует базу данных.

Ставка Tuck — **операционная простота**:

| | Vault | Tuck |
|---|---|---|
| Зависимости | Consul / Raft + DB | нет — один бинарь |
| Хранилище | Внешнее | Встроенный bbolt или встроенный Raft |
| Распечатывание после рестарта | Ручное (кворум Shamir) | Автоматическое (dev / transit) |
| Kubernetes-оператор | Внешний (ESO) | Встроенный |
| Хранилища секретов | KV, PKI, Transit, SSH, Database, TOTP | KV v1/v2, Cubbyhole (фокус на Kubernetes; PKI, Transit, SSH, TOTP и динамические секреты удалены) |
| Методы аутентификации | Token, K8s, JWT, AppRole, LDAP | Те же, плюс SSO (OIDC) |
| HA | Vault Enterprise / OSS Raft | Встроенный Raft (3–5 нод) |
| Размер бинаря | ~300 МБ | ~20 МБ |

---

## Возможности

### Ядро

- **Envelope-шифрование AES-256-GCM** — root key → DEK → шифртекст; ротация ключа перезаворачивает только DEK, данные не перешифровываются
- **Три типа seal:** dev (автораспечатывание, локально), Shamir (кворум n-of-k), Transit (Vault-совместимый API)
- **KV v1** — простые пары ключ-значение с ACL
- **KV v2** — версионированные секреты: CAS (check-and-set), мягкое удаление, восстановление, уничтожение, настраиваемый `max_versions`
- **Tamper-evident аудит-лог** — SHA-256 hash chain, значения секретов никогда не логируются
- **Rate limiting по IP** — token bucket, экспоненциальный backoff при ошибках авторизации
- **TLS** — самоподписанный ECDSA P-256 для разработки или принесите свой сертификат
- **Graceful shutdown** — 30-секундный дренаж + запечатывание при выходе
- **Backup/restore** — `GET /v1/sys/snapshot` отдаёт живой снапшот bbolt
- **Ротация ключа** — `POST /v1/sys/rotate` генерирует новый root key, перезаворачивает DEK

### Методы аутентификации

| Метод | Описание |
|-------|----------|
| **Token** | Root-токен при инициализации; создание скопированных токенов с TTL и политиками |
| **Kubernetes SA** | Воркloadы обмениваются ServiceAccount JWT через `TokenReview` API |
| **JWT / OIDC** | Любой OIDC-провайдер — Keycloak, Auth0, Google |
| **AppRole** | Машина-машина: `role_id` + `secret_id` |
| **LDAP / AD** | Пользователи из OpenLDAP, Active Directory, FreeIPA; группа → роль → политики и namespace; перепроверка токенов каждые 5 минут |
| **SSO (OIDC)** | Вход по коду с PKCE (Keycloak и др.), `bound_groups`, собственный УЦ |

### Команды и несколько кластеров

- **Мастер подключения команды** (страница Teams, `tuckcli team onboard`): namespace, политики, LDAP-роли и роли ServiceAccount одной операцией, с предпросмотром и откатом
- **Один Tuck — несколько зеркальных кластеров** (`kubernetes.clusters`), горячее подключение кластера без рестарта
- Снимки и атомарное восстановление Raft-кластера (`tuckcli snapshot`)

### Kubernetes

- **Оператор** — CRD `TuckSecret` синхронизирует секреты в нативные K8s Secrets; conditions (`Synced`, `Ready`); Lease-выбор лидера; политика удаления (`Retain` / `Delete`)
- **Webhook Agent Injector** — MutatingWebhook инжектирует init-контейнер, который записывает секреты на tmpfs; etcd не затрагивается
- **Helm-chart** — один `helm install` разворачивает сервер + оператор + опциональный инжектор

### Операции

- **Raft HA** — встроенный кластер 3–5 нод; встроенный консенсус; никакого внешнего координатора
- **Prometheus-метрики** по адресу `/metrics`
- **OpenTelemetry-трейсинг** (OTLP-экспортёр)
- **Встроенный веб-дашборд** по адресу `/ui/`
- **CLI-клиент** (`tuckcli`) — полное управление KV, токенами, политиками
- **Go SDK** (`pkg/client`) — типизированный клиент для всего API
- **OpenAPI 3.0 spec** по адресу `/openapi.json`

---

## Быстрый старт

### Запуск локально (dev seal)

```sh
go run ./cmd/tuck --tls-auto
# tuck: TLS enabled (auto-generated self-signed — dev only)
# ROOT TOKEN (shown once — store it securely):
# tuck_...
```

`--seal-type=dev` включён по умолчанию; без `--tls-auto` (или `--tls-cert`/`--tls-key`) сервер слушает обычный HTTP. Данные пишутся в `tuck.db` в текущем каталоге.

### Сохранить и получить секрет

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_...
export TUCK_INSECURE=1   # только для самоподписанного сертификата --tls-auto

tuckcli kv put db/password s3cr3t
tuckcli kv get db/password
# JSON: {"created_at": "...", "path": "db/password", "value": "s3cr3t"}

tuckcli kv list db/
# JSON: {"keys": ["password"]}
```

### Через curl

```sh
curl -k -X PUT https://127.0.0.1:8200/v1/secret/db/password \
  -H "X-Tuck-Token: $TUCK_TOKEN" -d 's3cr3t'

curl -k https://127.0.0.1:8200/v1/secret/db/password \
  -H "X-Tuck-Token: $TUCK_TOKEN"
```

---

## Продакшн (Shamir seal)

```sh
tuck \
  --seal-type=shamir \
  --seal-shamir-n=5 \
  --seal-shamir-k=3 \
  --addr=0.0.0.0:8200 \
  --tls-cert=/etc/tuck/tls.crt \
  --tls-key=/etc/tuck/tls.key \
  --data=/var/lib/tuck/tuck.db
```

При первом старте Tuck выводит root-токен и 5 шардов Shamir. Раздайте шарды разным операторам — ни один из них в одиночку не может распечатать сервер.

После рестарта введите любые 3 шарда:

```sh
tuckcli unseal <shard-1>
tuckcli unseal <shard-2>
tuckcli unseal <shard-3>   # "unsealed successfully"
```

---

## Kubernetes

### Helm-установка

```sh
helm install tuck deploy/helm/tuck \
  --namespace tuck-system --create-namespace \
  --set server.sealType=shamir \
  --set server.shamirSeal.n=5,server.shamirSeal.k=3
```

### Объявление секрета через CRD

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

Несколько ключей в одном Secret: вместо `tuckPath`/`secretKey` используйте `data: {username: my-app/db/user, password: my-app/db/pass}`. В кластерах с несколькими командами запускайте оператор с `--enforce-namespace-paths`: TuckSecret сможет читать только `<namespace>/...` (плюс префиксы из аннотации Namespace `tuck.io/allowed-paths`) — см. [гайд 07](docs/guides/07-tucksecret-operator.md).

### Инъекция через Webhook (минует etcd)

```yaml
metadata:
  annotations:
    tuck.io/inject: "true"
    tuck.io/addr: "https://tuck.tuck-system:8200"
    tuck.io/secrets: "db/password:password.txt,api/key:api-key.txt"
```

Секреты записываются в `/tuck/secrets/` на tmpfs-volume до старта app-контейнеров.

---

## Архитектура

```
┌─────────────────────────────────────────────────────────┐
│  HTTP API  (net/http, без фреймворков)                   │
│  TLS · Auth-middleware · Rate limiter · Audit log        │
│  Prometheus · OpenTelemetry · OpenAPI                    │
├─────────────────────────────────────────────────────────┤
│  Core  (оркестрация + логические операции)               │
│  Token store · Policy store · KV v1/v2                   │
├────────────────┬────────────────────────────────────────┤
│  Auth-методы   │  Kubernetes-интеграция                  │
│  · K8s SA      │  · Оператор TuckSecret (CRD)            │
│  · JWT / OIDC  │  · Webhook-инжектор (tmpfs)             │
│  · AppRole     │  · CSI-провайдер                        │
│  · LDAP / AD   │                                         │
├────────────────┴────────────────────────────────────────┤
│  Barrier  (AES-256-GCM envelope-шифрование)             │
│  root key → DEK → шифртекст                             │
├─────────────────────────────────────────────────────────┤
│  Physical backend                                        │
│  bbolt (один файл) | Raft HA (3–5 нод, встроенный)      │
└─────────────────────────────────────────────────────────┘
               ▲
         Seal (dev | shamir | transit)
```

---

## Разработка

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck

go test ./...              # все тесты
go test -race ./...        # с race detector
go build ./cmd/tuck        # сервер
go build ./cmd/tuckcli     # CLI
go build ./cmd/tuck-operator
go build ./cmd/tuck-injector
go build ./cmd/tuck-agent
```

---

## Статус

| Milestone | Версия | Статус |
|-----------|--------|--------|
| M0 — Криптоядро, bbolt, KV API | v0.1 | ✅ |
| M1 — Token-аутентификация, ACL-политики | v0.2 | ✅ |
| M2 — Kubernetes SA auth | v0.3 | ✅ |
| M3 — TuckSecret CRD + оператор | v0.4 | ✅ |
| M4 — Shamir + Transit seal | v0.5 | ✅ |
| M5 — TLS, graceful shutdown, CI | v0.5 | ✅ |
| M6 — Аудит-лог, метрики, backup, rate limiting | v0.6 | ✅ |
| M7 — LIST, token renew, ротация ключей, CLI | v0.7 | ✅ |
| M8 — HA-оператор, веб-UI, threat model | v0.9 | ✅ |
| M9 — KV v2, OpenTelemetry, OpenAPI, dashboard | v0.9 | ✅ |
| M10 — Go SDK, goreleaser, release pipeline | v0.10 | ✅ |
| M11 — Raft HA backend (кластер 3–5 нод) | v0.11 | ✅ |
| M12 — Webhook Agent Injector (tmpfs, минует etcd) | v0.12 | ✅ |
| M13 — JWT/OIDC auth, Helm-chart | v0.13 | ✅ |
| M14 — AppRole auth, Database dynamic secrets (Database удалён) | v0.14 | ✅ |
| M15 — PKI-движок (внутренний X.509 CA) | v0.15 | 🗑 удалено |
| M16 — Transit-движок (шифрование как сервис) | v0.16 | 🗑 удалено |
| M17 — SSH-движок (CA-режим, сертификаты) | v0.17 | 🗑 удалено |
| M18 — TOTP-движок (2FA / OTP-валидация) | v0.18 | 🗑 удалено |
| M19 — AWS KMS + GCP Cloud KMS seal backends | v0.19 | 🗑 удалено |
| M20 — LDAP/AD auth (Azure Key Vault seal — удалён) | v0.20 | ✅ |
| M21 — AWS динамические секреты | v0.21 | 🗑 удалено |
| M22 — GCP динамические секреты | v0.22 | 🗑 удалено |
| M23 — Azure динамические секреты | v0.23 | 🗑 удалено |
| M24 — Response wrapping (одноразовые токены, безопасная передача секретов) | v0.24 | ✅ |
| M25 — Cubbyhole (приватное хранилище токена, автоочистка при отзыве) | v0.25 | ✅ |
| M26 — Token accessor (псевдоним tuck_acc_, lookup/revoke без знания токена) | v0.26 | ✅ |
| M27 — Deny-правила в политиках (CapDeny; запрет имеет приоритет над разрешением) | v0.27 | ✅ |
| M28 — Возобновляемые токены с MaxTTL (флаг renewable, ограничение max_ttl, lookup-self, renew-self) | v0.28 | ✅ |
| M29 — Token MaxUses (авто-отзыв после N вызовов API; одноразовые bootstrap-токены) | v0.29 | ✅ |
| M30 — UI: Auth Methods (AppRole/JWT/LDAP/K8s) | v0.30 | ✅ |
| M31 — UI: Crypto Engines (PKI / Transit / SSH / TOTP в браузере) | v0.31 | 🗑 снято |
| M32 — CLI completeness (db creds, pki/transit/ssh/totp операции) | v0.32 | 🗑 снято |
| v1.0 GA (2026-06-12); внешний security audit | v1.0 | 🔜 (аудит) |
| Сужение до Kubernetes-сценария (удаление движков) | v2.0 | ✅ |
| Пилот банка: мастер команд, LDAP/SSO, зеркальные кластеры, восстановление, нагрузочные тесты | v2.1 | ✅ |

---

## Безопасность

Смотрите [SECURITY.md](SECURITY.md) для политики раскрытия уязвимостей и [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) для модели угроз.

Сообщить об уязвимости: **genaevlive@gmail.com** (coordinated disclosure, 90-дневное окно).

---

## Лицензия

Apache-2.0. Смотрите [LICENSE](LICENSE).

Исключение: директория [`web/`](web/) (веб-интерфейс, форк UI-каркаса из [Remnawave](https://github.com/remnawave/frontend)) распространяется отдельно под **AGPL-3.0-only** — см. [`web/LICENSE-AGPL-3.0`](web/LICENSE-AGPL-3.0) и [`web/NOTICE`](web/NOTICE).
