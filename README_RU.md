# Tuck

> Самый простой менеджер секретов для Kubernetes. Спрячь секрет — без лишних ритуалов.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Release](https://img.shields.io/badge/release-v1.31.0-green)](https://github.com/NAGenaev/tuck/releases)

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
| Распечатывание после рестарта | Ручное (кворум Shamir) | Автоматическое (dev / transit / AWS KMS / GCP KMS / Azure KV) |
| Kubernetes-оператор | Внешний (ESO) | Встроенный |
| Движки секретов | PKI, Transit, SSH, Database, TOTP | Те же |
| Методы аутентификации | Token, K8s, JWT, AppRole | Те же |
| HA | Vault Enterprise / OSS Raft | Встроенный Raft (3–5 нод) |
| Размер бинаря | ~300 МБ | ~20 МБ |

---

## Возможности

### Ядро

- **Envelope-шифрование AES-256-GCM** — root key → DEK → шифртекст; ротация ключа перезаворачивает только DEK, данные не перешифровываются
- **Шесть типов seal:** dev (автораспечатывание, локально), Shamir (кворум n-of-k), Transit (Vault-совместимый API), AWS KMS (IRSA / instance role), GCP Cloud KMS (Workload Identity / ADC), Azure Key Vault (Workload Identity / Managed Identity / DefaultAzureCredential)
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
| **JWT / OIDC** | Любой OIDC-провайдер — Keycloak, Auth0, GitHub Actions, Google |
| **AppRole** | Машина-машина: `role_id` + `secret_id` |
| **LDAP / AD** | Аутентификация пользователей из OpenLDAP, Active Directory, FreeIPA; членство в группах → политики через Roles |

### Движки динамических секретов

| Движок | Описание |
|--------|----------|
| **AWS** | On-demand учётные данные IAM или STS AssumeRole-сессии; автоотзыв по истечении lease |
| **GCP** | On-demand JSON-ключи сервисного аккаунта или OAuth2 access-токены; автоотзыв по истечении lease |
| **Azure** | On-demand клиентские секреты Azure AD (Graph API); автоотзыв по истечении lease |
| **Database** | On-demand учётные данные PostgreSQL / MySQL; автоотзыв по истечении lease |
| **PKI** | Внутренний X.509 CA; выпуск короткоживущих TLS-сертификатов по роли |
| **Transit** | Шифрование как сервис; версионированные ключи (AES-256-GCM, ECDSA, Ed25519, RSA-PSS); sign/verify/HMAC; rewrap после ротации |
| **SSH** | CA-режим: подпись публичных ключей пользователей и хостов; workflow `TrustedUserCAKeys` |
| **TOTP** | Хранение TOTP-секретов, валидация и генерация OTP-кодов; URL `otpauth://` для QR-кода |

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
go run ./cmd/tuck --seal-type=dev
# tuck: unsealed (dev seal), serving on https://127.0.0.1:8200
# ROOT TOKEN (shown once): tuck_...
```

### Сохранить и получить секрет

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_...

tuckcli kv put db/password s3cr3t
tuckcli kv get db/password
# {"path":"db/password","value":"s3cr3t"}

tuckcli kv list db/
# {"keys":["password"]}
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
  --data-dir=/var/lib/tuck
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

## Примеры динамических секретов

### PKI — выпуск TLS-сертификата

```sh
# Создать корневой CA
curl -XPOST https://tuck:8200/v1/pki/generate/root \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"common_name":"Tuck Internal CA","ttl":"87600h"}'

# Создать роль
curl -XPUT https://tuck:8200/v1/pki/roles/web \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"allowed_domains":["svc.cluster.local"],"allow_subdomains":true,"default_ttl":"72h"}'

# Выпустить сертификат
curl -XPOST https://tuck:8200/v1/pki/issue/web \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"common_name":"api.svc.cluster.local"}'
```

### SSH — подпись SSH-ключа

```sh
# Настройка хоста (один раз): добавить CA-ключ в TrustedUserCAKeys
curl https://tuck:8200/v1/ssh/ca/public-key | jq -r .public_key \
  | sudo tee /etc/ssh/trusted_user_ca_keys

# Подписать публичный ключ пользователя
curl -XPOST https://tuck:8200/v1/ssh/sign/ops \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"public_key":"ssh-ed25519 AAAA...","valid_principals":["ubuntu"],"ttl":"24h"}'
```

### Transit — шифрование без передачи ключей

```sh
# Создать AES-ключ
curl -XPOST https://tuck:8200/v1/transit/keys/payments \
  -H "X-Tuck-Token: $TOKEN" -d '{"type":"aes256-gcm96"}'

# Зашифровать
CIPHER=$(curl -s -XPOST https://tuck:8200/v1/transit/encrypt/payments \
  -H "X-Tuck-Token: $TOKEN" \
  -d "{\"plaintext\":\"$(echo -n 'card:4242' | base64)\"}" | jq -r .ciphertext)

# Ротировать и перезавернуть
curl -XPOST https://tuck:8200/v1/transit/keys/payments/rotate -H "X-Tuck-Token: $TOKEN"
curl -XPOST https://tuck:8200/v1/transit/rewrap/payments \
  -H "X-Tuck-Token: $TOKEN" -d "{\"ciphertext\":\"$CIPHER\"}"
```

### TOTP — валидация 2FA

```sh
# Создать ключ (вернёт секрет + otpauth:// URL для импорта в приложение)
curl -XPOST https://tuck:8200/v1/totp/keys/myapp \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"issuer":"ACME Corp","account":"user@example.com"}'

# Проверить код от пользователя
curl -XPOST https://tuck:8200/v1/totp/code/myapp \
  -H "X-Tuck-Token: $TOKEN" -d '{"code":"123456"}'
# → {"valid":true}
```

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
│  Auth-движки   │  Движки динамических секретов           │
│  · K8s SA      │  · Database (PostgreSQL / MySQL)        │
│  · JWT / OIDC  │  · PKI (X.509 CA)                       │
│  · AppRole     │  · Transit (шифрование как сервис)      │
│                │  · SSH (CA-режим, сертификаты)          │
│                │  · TOTP (time-based OTP)                │
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
| M14 — AppRole auth, Database dynamic secrets | v0.14 | ✅ |
| M15 — PKI-движок (внутренний X.509 CA) | v0.15 | ✅ |
| M16 — Transit-движок (шифрование как сервис) | v0.16 | ✅ |
| M17 — SSH-движок (CA-режим, сертификаты) | v0.17 | ✅ |
| M18 — TOTP-движок (2FA / OTP-валидация) | v0.18 | ✅ |
| M19 — AWS KMS + GCP Cloud KMS seal backends | v0.19 | ✅ |
| M20 — LDAP/AD auth, Azure Key Vault seal | v0.20 | ✅ |
| M21 — AWS динамические секреты (IAM user + STS AssumeRole) | v0.21 | ✅ |
| M22 — GCP динамические секреты (SA key + access token) | v0.22 | ✅ |
| M23 — Azure динамические секреты (клиентские секреты AD, Graph API) | v0.23 | ✅ |
| M24 — Response wrapping (одноразовые токены, безопасная передача секретов) | v0.24 | ✅ |
| M25 — Cubbyhole (приватное хранилище токена, автоочистка при отзыве) | v0.25 | ✅ |
| M26 — Token accessor (псевдоним tuck_acc_, lookup/revoke без знания токена) | v0.26 | ✅ |
| M27 — Deny-правила в политиках (CapDeny; запрет имеет приоритет над разрешением) | v0.27 | ✅ |
| M28 — Возобновляемые токены с MaxTTL (флаг renewable, ограничение max_ttl, lookup-self, renew-self) | v0.28 | ✅ |
| M29 — Token MaxUses (авто-отзыв после N вызовов API; одноразовые bootstrap-токены) | v0.29 | ✅ |
| M30 — UI: Auth Methods (AppRole/JWT/LDAP/K8s) + Dynamic Secrets (DB/AWS/GCP/Azure) + Leases | v0.30 | ✅ |
| M31 — UI: Crypto Engines (PKI / Transit / SSH / TOTP в браузере) | v0.31 | 🔜 |
| M32 — CLI completeness (db/aws/gcp/azure creds, pki/transit/ssh/totp операции) | v0.32 | 🔜 |
| v1.0 GA — Внешний security audit | — | 🔜 |

---

## Безопасность

Смотрите [SECURITY.md](SECURITY.md) для политики раскрытия уязвимостей и [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) для модели угроз.

Сообщить об уязвимости: **genaevlive@gmail.com** (coordinated disclosure, 90-дневное окно).

---

## Лицензия

Apache-2.0. Смотрите [LICENSE](LICENSE).
