# Tuck

> Самый простой менеджер секретов для Kubernetes. Спрячь секрет — без лишних ритуалов.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.9_pre--GA-orange)]()

Tuck — это open-source менеджер секретов, созданный для Kubernetes. Главная идея: **анти-Vault** — один статический бинарь, никакой внешней базы данных, автоматическое распечатывание по умолчанию. `kubectl apply` — и работает.

[English documentation →](README.md)

---

## Зачем ещё один менеджер секретов?

[HashiCorp Vault](https://www.vaultproject.io) мощный, но операционно тяжёлый: кворум Consul/Raft, ручное распечатывание после каждого рестарта, сложная модель ACL. [OpenBao](https://openbao.org) наследует ту же сложность. [Infisical](https://infisical.com) требует базу данных.

Ставка Tuck — **операционная простота**:

| | Vault | Tuck |
|---|---|---|
| Зависимости | Consul / Raft + DB | нет — один бинарь |
| Хранилище | Внешнее | Встроенный bbolt |
| Распечатывание после рестарта | Ручное (кворум Shamir) | Автоматическое (dev/transit) |
| Kubernetes-оператор | Внешний (ESO) | Встроенный |
| Размер бинаря | ~300 МБ | ~20 МБ |

Tuck не пытается заменить Vault для крупных энтерпрайзов. Целевая аудитория — **небольшие и средние Kubernetes-платформы**, где операционная нагрузка важнее, чем федерация и динамические секреты.

---

## Возможности

- **Envelope-шифрование AES-256-GCM** — root key → DEK → данные; ротация ключа перезаворачивает только DEK, данные не перешифровываются
- **Три типа seal:** dev (автораспечатывание, только локально), Shamir (кворум n-of-k), Transit (KMS через Vault-совместимый API)
- **Полный REST API** — KV-секреты, токены, ACL-политики, LIST-эндпоинты, бинарно-безопасные значения
- **Kubernetes-оператор** — CRD `TuckSecret` синхронизирует секреты Tuck в нативные K8s Secrets с conditions в статусе
- **Kubernetes SA-аутентификация** — воркloadы аутентифицируются через `TokenReview` API, никаких sidecar
- **CLI-клиент** (`tuckcli`) — get/put/delete/list секретов, управление токенами и политиками
- **Встроенный веб-дашборд** по адресу `/ui/` — без шага сборки
- **Prometheus-метрики** по адресу `/metrics`
- **Tamper-evident аудит-лог** — SHA-256 hash chain, значения секретов никогда не логируются
- **Ограничение запросов по IP** — token bucket, экспоненциальный backoff при ошибках авторизации
- **TLS** — самоподписанный ECDSA P-256 для разработки или принесите свой сертификат
- **Graceful shutdown** — 30-секундный дренаж + seal при выходе
- **HA-оператор** — выбор лидера через `Lease` Kubernetes

---

## Быстрый старт

### Запуск локально (dev seal)

```sh
go run ./cmd/tuck --seal-type=dev
# tuck: unsealed (dev seal), serving on https://127.0.0.1:8200
# ROOT TOKEN (shown once): tuck_...
```

### Сохранить и прочитать секрет

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

При первом запуске Tuck выводит root-токен и 5 шардов Shamir. Раздайте шарды разным операторам — ни один из них в одиночку не может распечатать сервер.

После рестарта подайте любые 3 шарда:

```sh
tuckcli unseal <шард-1>
tuckcli unseal <шард-2>
tuckcli unseal <шард-3>   # "unsealed successfully"
```

---

## Kubernetes

### Установка оператора

```sh
kubectl apply -f deploy/crd/
kubectl apply -f deploy/operator/
```

### Объявление секрета

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

Оператор следит за CRD и создаёт/обновляет нативный `Secret`. В статусе CRD отображается условие `Synced: True` или сообщение об ошибке.

### Аутентификация воркloada

```sh
tuckcli token create \
  --name=my-app \
  --policy=app-policy \
  --k8s-sa=my-app/default
```

Воркload обменивает свой ServiceAccount-токен на токен Tuck через `POST /v1/auth/k8s`.

---

## Справка по CLI

```
tuckcli status                              # статус seal
tuckcli unseal <шард>                       # подать шард Shamir
tuckcli seal                                # запечатать сервер

tuckcli kv get <путь>                       # прочитать секрет
tuckcli kv put <путь> <значение>            # записать секрет
tuckcli kv delete <путь>                    # удалить секрет
tuckcli kv list <префикс/>                  # список ключей

tuckcli token create --name=x --policy=y --ttl=24h
tuckcli token get <id>
tuckcli token renew <id> 48h
tuckcli token revoke <id>
tuckcli token list

tuckcli policy put <имя> <json-правила>
tuckcli policy get <имя>
tuckcli policy delete <имя>
tuckcli policy list

tuckcli rotate                              # ротация root key (нужен root-токен)
```

Переменные окружения: `TUCK_ADDR` (по умолчанию `https://127.0.0.1:8200`), `TUCK_TOKEN`.

---

## Архитектура

```
┌──────────────────────────────────────────────┐
│  HTTP API  (net/http, без фреймворков)        │
│  TLS · Auth middleware · Rate limiter         │
│  Audit log (hash chain) · Metrics             │
├──────────────────────────────────────────────┤
│  Core  (оркестрация, логический KV)           │
│  Token store · Policy store                   │
├──────────────────────────────────────────────┤
│  Barrier  (envelope-шифрование AES-256-GCM)   │
│  root key → DEK → ciphertext                  │
├──────────────────────────────────────────────┤
│  Physical backend                             │
│  bbolt (один файл) | in-memory (тесты)        │
└──────────────────────────────────────────────┘
     ▲
  Seal (dev | shamir | transit)
```

Ротация ключей: `POST /v1/sys/rotate` генерирует новый root key, перезаворачивает DEK и возвращает новые шарды Shamir — данные не перешифровываются.

---

## Разработка

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck

go test ./...              # все тесты
go test -race ./...        # с детектором гонок
go build ./cmd/tuck        # бинарь сервера
go build ./cmd/tuckcli     # бинарь CLI
go build ./cmd/tuck-operator  # бинарь оператора
```

Подробности: [CONTRIBUTING.md](CONTRIBUTING.md), архитектура — [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), операционные процедуры — [docs/RUNBOOK.md](docs/RUNBOOK.md).

### Нагрузочное тестирование

```sh
k6 run \
  --env TUCK_ADDR=https://127.0.0.1:8200 \
  --env TUCK_TOKEN=$TUCK_TOKEN \
  --duration 1m --vus 200 \
  test/load/k6_soak.js
```

---

## API

Все эндпоинты требуют заголовок `X-Tuck-Token`, кроме `/v1/sys/seal-status` и `/v1/sys/unseal`.

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/v1/sys/seal-status` | Статус seal |
| `POST` | `/v1/sys/unseal` | Подать шард Shamir |
| `POST` | `/v1/sys/seal` | Запечатать сервер |
| `POST` | `/v1/sys/rotate` | Ротация root key |
| `GET` | `/v1/sys/snapshot` | Скачать снепшот bbolt |
| `GET` | `/v1/health` | Liveness probe |
| `GET` | `/v1/sys/ready` | Readiness probe (503 если sealed) |
| `GET/PUT/DELETE` | `/v1/secret/{path}` | KV-операции |
| `LIST` | `/v1/secret/{prefix}` | Список ключей |
| `POST` | `/v1/auth/token` | Создать токен |
| `GET/DELETE` | `/v1/auth/token/{id}` | Получить / отозвать токен |
| `POST` | `/v1/auth/token/{id}/renew` | Продлить токен |
| `LIST` | `/v1/auth/token/` | Список токенов |
| `GET/PUT/DELETE` | `/v1/policy/{name}` | Операции с политиками |
| `LIST` | `/v1/policy/` | Список политик |
| `POST` | `/v1/auth/k8s` | Аутентификация Kubernetes SA |
| `GET` | `/metrics` | Prometheus-метрики |
| `GET` | `/ui/` | Веб-дашборд |

---

## Безопасность

Политика раскрытия уязвимостей: [SECURITY.md](SECURITY.md). Модель угроз: [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).

Сообщить об уязвимости: **genaevlive@gmail.com** (координированное раскрытие, 90-дневное окно).

---

## Статус и дорожная карта

| Milestone | Статус |
|---|---|
| M0 — Crypto core (barrier, bbolt, dev seal, KV API) | ✅ |
| M1 — Токен-аутентификация, ACL-политики | ✅ |
| M2 — Kubernetes ServiceAccount auth | ✅ |
| M3 — TuckSecret CRD + оператор | ✅ |
| M4 — Production seals (Shamir, Transit) | ✅ |
| M5 — Security & DevOps baseline (TLS, graceful shutdown, CI) | ✅ |
| M6 — Надёжность и наблюдаемость (аудит-лог, метрики, бэкап) | ✅ |
| M7 — Полнота API (LIST, renew, ротация ключей, CLI) | ✅ |
| M8 — Pre-GA hardening (HA-оператор, веб-UI, threat model, сообщество) | ✅ |
| v1.0 GA — Внешний security audit | 🔜 |

Post-GA (v1.x): Raft HA-backend, KV v2 (версионирование), Webhook Agent Injector, OpenTelemetry.

---

## Лицензия

Apache-2.0. Подробнее: [LICENSE](LICENSE).
