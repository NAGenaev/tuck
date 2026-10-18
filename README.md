# Tuck — Система управления секретами

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Release](https://img.shields.io/badge/release-v2.1.0-green)](https://github.com/NAGenaev/tuck/releases)

Tuck — специализированная система управления секретами (криптографическими ключами, учётными данными и конфиденциальными параметрами конфигурации), разработанная для функционирования в среде Kubernetes. Система реализована в виде единого исполняемого модуля без внешних зависимостей и обеспечивает централизованное хранение, разграничение доступа и автоматизированную ротацию секретов.

---

## 1. Назначение системы

Система Tuck предназначена для решения следующих задач:

- централизованное хранение конфиденциальных данных с применением конвертного шифрования AES-256-GCM;
- разграничение доступа к секретам на основе политик ACL с поддержкой glob-масок и запрещающих правил;
- интеграция с платформой Kubernetes посредством CRD-контроллера, webhook-инжектора и CSI-драйвера;
- поддержка отказоустойчивого кластера (Raft HA, 3–5 узлов) без внешних координаторов;
- динамическая генерация учётных данных для СУБД и облачных провайдеров с автоматическим отзывом по истечении срока аренды.

---

## 2. Функциональные характеристики

### 2.1. Подсистема хранения секретов

| Компонент | Описание |
|-----------|----------|
| **KV v1** | Хранилище пар «ключ — значение» с контролем доступа по политикам ACL |
| **KV v2** | Версионированное хранилище: проверка версии (CAS), мягкое удаление, восстановление, уничтожение версии, ограничение числа хранимых версий |
| **Cubbyhole** | Изолированное хранилище, привязанное к токену; уничтожается при отзыве токена |
| **Оборачивание ответов** | Одноразовая передача значения через обёрточный токен (response wrapping) |

### 2.2. Подсистема аутентификации

| Метод | Описание |
|-------|----------|
| **Token** | Корневой токен при инициализации; создание областных токенов с TTL, MaxUses, MaxTTL, возобновлением |
| **Kubernetes SA** | Рабочие нагрузки предъявляют JWT сервисного аккаунта через API TokenReview |
| **JWT / OIDC** | Любой провайдер — Keycloak, Auth0, GitHub Actions, Google Workspace |
| **AppRole** | Машинная аутентификация по паре RoleID + SecretID |
| **LDAP / Active Directory** | Аутентификация пользователей каталога; группа → роль → политики и namespace; перепроверка живых токенов (ушёл из группы — токен отозван), защита от перебора |
| **SSO (OIDC)** | Браузерный вход по коду с PKCE (Keycloak и др.), привязка групп к ролям, внутренний УЦ (`ca_cert`) |

### 2.2а. Подключение команд и изоляция

- **Мастер подключения команды** (страница Teams, `POST /v1/sys/teams`, `tuckcli team onboard`): namespace, политики `team-admin` / `team-developer` / `team-reader`, LDAP-роли «доменная группа → уровень доступа» и роли ServiceAccount по кластерам — целиком или не создаётся ничего; предпросмотр `dry_run`, откат при ошибке
- **Обновление шаблонов** у подключённых команд: `tuckcli team sync` (трогает только политики, записанные мастером)
- Токен привязан к namespace, в котором выдан; политики команды не позволяют ей создавать роли входа
- **Несколько зеркальных кластеров** с одним Tuck: `kubernetes.clusters`, роли по кластеру, горячее добавление кластера (SIGHUP или API)

### 2.3. Подсистема интеграции с Kubernetes

- **Оператор** — CRD `TuckSecret` синхронизирует секреты Tuck с объектами Kubernetes Secret; выбор лидера на основе Lease; политика при удалении (`Retain` / `Delete`)
- **Webhook-инжектор** — MutatingWebhookConfiguration внедряет init-контейнер, записывающий секреты в том tmpfs; данные не попадают в etcd
- **CSI-драйвер** — монтирование секретов как файловой системы SecretProviderClass
- **Helm-чарт** — развёртывание сервера, оператора и инжектора одной командой

### 2.4. Подсистема обеспечения отказоустойчивости

- Встроенный Raft (3–5 узлов), не требующий внешних координаторов (Consul, etcd)
- Автоматическое снятие печати: Transit (корневой ключ шифруется ключом другого Tuck/Vault-совместимого Transit)
- Снятие печати по схеме Шамира (n-of-k): распределённое хранение долей
- Резервное копирование: `GET /v1/sys/snapshot` — потоковый снимок BoltDB; для кластера Raft `tuckcli snapshot save|restore` с атомарным восстановлением
- Безопасное обновление версии по одному узлу с передачей лидерства (`step-down`)

### 2.5. Средства аудита и мониторинга

- Журнал аудита с цепочкой хэшей SHA-256 (значения секретов не записываются); в записях имя человека (`identity`), `entity_id` и реальный IP клиента за доверенным балансировщиком (`--trusted-proxies`)
- Метрики Prometheus на `/metrics`
- Трассировка OpenTelemetry (OTLP-экспортёр)
- Ограничение частоты запросов: token bucket на IP-адрес

---

## 3. Технические характеристики

| Параметр | Значение |
|----------|----------|
| Язык реализации | Go 1.26.9 |
| Размер исполняемого модуля | ~20 МБ |
| Физическое хранилище | BoltDB (одиночный узел) / встроенный Raft (кластер) |
| Шифрование | AES-256-GCM (конвертное) |
| Транспортный протокол | HTTPS (TLS 1.2+, ECDSA P-256 или пользовательский сертификат) |
| HTTP API | 194+ эндпоинтов, спецификация OpenAPI 3.0 |
| Производительность (стенд, 3 узла Raft, mTLS) | 889 запросов/с смешанной нагрузки без ошибок, чтение 1,2 мс, запись p95 197 мс — [отчёт](docs/PILOT_LAB_REPORT.md) |
| Лицензия | Apache-2.0 (за исключением `web/` — AGPL-3.0, см. ниже) |

---

## 4. Порядок развёртывания

### 4.1. Режим разработки (автоматическое снятие печати)

```sh
go run ./cmd/tuck --tls-auto
# tuck: TLS enabled (auto-generated self-signed — dev only)
# ROOT TOKEN (shown once — store it securely):
# tuck_...
```

`--seal-type=dev` задан по умолчанию; без `--tls-auto` (или `--tls-cert`/`--tls-key`) сервер слушает обычный HTTP. Данные пишутся в `tuck.db` в текущем каталоге.

> Режим разработки не предназначен для использования в производственных средах.

### 4.2. Запись и чтение секрета

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_...
export TUCK_INSECURE=1   # только для самоподписанного сертификата --tls-auto

tuckcli kv put db/password s3cr3t
tuckcli kv get db/password
tuckcli kv list db/
```

### 4.3. Производственное развёртывание (схема Шамира)

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

При первом запуске система выводит корневой токен и 5 долей Шамира. Доли распределяются операторам раздельно. После перезапуска сервера для снятия печати необходимо предъявить любые 3 доли:

```sh
tuckcli unseal <доля-1>
tuckcli unseal <доля-2>
tuckcli unseal <доля-3>
```

### 4.4. Автоматическое снятие печати — Transit

```sh
export TUCK_TRANSIT_TOKEN=<токен>
tuck   --seal-type=transit   --seal-transit-addr=https://transit.example.com   --seal-transit-key=tuck-root   --addr=0.0.0.0:8200   --tls-cert=/etc/tuck/tls.crt   --tls-key=/etc/tuck/tls.key
```

Корневой ключ шифруется ключом Transit внешнего сервиса и сохраняется в файле `tuck-transit.enc`; без доступа к ключу Transit файл не представляет ценности.

### 4.5. Развёртывание в Kubernetes через Helm

```sh
helm install tuck deploy/helm/tuck \
  --namespace tuck-system --create-namespace \
  --set server.sealType=shamir \
  --set server.shamirSeal.n=5,server.shamirSeal.k=3
```

**Объявление секрета через CRD:**

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

Several keys in one Secret: replace `tuckPath`/`secretKey` with `data: {username: my-app/db/user, password: my-app/db/pass}`. Run the operator with `--enforce-namespace-paths` on multi-team clusters so a TuckSecret can only read `<namespace>/...` (plus prefixes granted via the `tuck.io/allowed-paths` Namespace annotation) — see [guide 07](docs/guides/07-tucksecret-operator.md).

**Внедрение секретов через webhook (без записи в etcd):**

```yaml
metadata:
  annotations:
    tuck.io/inject: "true"
    tuck.io/addr: "https://tuck.tuck-system:8200"
    tuck.io/secrets: "db/password:password.txt,api/key:api-key.txt"
```

Секреты записываются в `/tuck/secrets/` на тот tmpfs до запуска контейнеров приложения.

---

## 5. Справочник API

Все эндпоинты требуют заголовок `X-Tuck-Token`, если не указано иное.

### 5.1. Системные операции

| Метод | Путь | Аутент. | Описание |
|-------|------|---------|----------|
| GET | `/v1/sys/seal-status` | публичный | Состояние печати |
| GET | `/v1/sys/ready` | публичный | Готовность (503 при запечатанном состоянии) |
| GET | `/v1/health` | публичный | Работоспособность |
| POST | `/v1/sys/unseal` | публичный | Предъявить долю Шамира |
| POST | `/v1/sys/seal` | токен | Запечатать сервер |
| POST | `/v1/sys/rotate` | токен | Ротация корневого ключа |
| GET | `/v1/sys/snapshot` | токен | Загрузить резервную копию BoltDB |
| GET | `/v1/sys/cluster` | токен | Статус кластера Raft |
| POST | `/v1/sys/cluster/join` | токен | Добавить узел Raft |
| DELETE | `/v1/sys/cluster/node/{id}` | токен | Исключить узел Raft |

### 5.2. Хранилище KV v1

| Метод | Путь | Описание |
|-------|------|----------|
| GET/PUT/DELETE | `/v1/secret/{path}` | Чтение / запись / удаление |
| LIST | `/v1/secret/{prefix}` | Перечисление ключей |

### 5.3. Хранилище KV v2 (версионированное)

| Метод | Путь | Описание |
|-------|------|----------|
| GET/PUT/DELETE | `/v2/secret/{path}` | Чтение / запись / мягкое удаление |
| LIST | `/v2/secret/{prefix}` | Перечисление ключей |
| POST | `/v2/secret/undelete/{path}` | Восстановление удалённой версии |
| POST | `/v2/secret/destroy/{path}` | Необратимое уничтожение версии |
| GET/PUT/DELETE | `/v2/secret/metadata/{path}` | Метаданные версий |

### 5.4. Аутентификация

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/v1/auth/token` | Создать токен |
| GET/DELETE | `/v1/auth/token/{id}` | Получить / отозвать |
| POST | `/v1/auth/token/{id}/renew` | Возобновить |
| LIST | `/v1/auth/token/` | Перечислить |
| POST | `/v1/auth/kubernetes/login` | Вход по Kubernetes SA (публичный) |
| PUT/GET/DELETE | `/v1/auth/kubernetes/role/{ns}/{sa}` | Привязка роли K8s |
| POST | `/v1/auth/jwt/login` | Вход JWT/OIDC (публичный) |
| GET/PUT | `/v1/auth/jwt/config` | Конфигурация JWKS |
| PUT/GET/DELETE/LIST | `/v1/auth/jwt/role/{name}` | Управление ролями JWT |
| POST | `/v1/auth/approle/login` | Вход AppRole (публичный) |
| PUT/GET/DELETE/LIST | `/v1/auth/approle/role/{name}` | Управление ролями AppRole |
| POST | `/v1/auth/approle/role/{name}/secret-id` | Генерация SecretID |
| POST | `/v1/auth/ldap/login` | Вход LDAP/AD (публичный) |
| GET/PUT | `/v1/auth/ldap/config` | Конфигурация LDAP |
| PUT/GET/DELETE/LIST | `/v1/auth/ldap/role/{name}` | Управление ролями LDAP |

### 5.5. Политики

| Метод | Путь | Описание |
|-------|------|----------|
| PUT/GET/DELETE | `/v1/policy/{name}` | Управление политикой |
| LIST | `/v1/policy/` | Перечислить политики |

### 5.6. Служебные эндпоинты

| Путь | Описание |
|------|----------|
| `GET /metrics` | Метрики Prometheus |
| `GET /ui/` | Веб-интерфейс управления |
| `GET /openapi.json` | Спецификация OpenAPI 3.0 |

---

## 6. Справочник CLI

```sh
# Системные операции
tuckcli status                              # состояние печати
tuckcli unseal <доля>                       # предъявить долю Шамира
tuckcli seal                                # запечатать сервер
tuckcli rotate                              # ротация корневого ключа

# Хранилище секретов
tuckcli kv get <путь>
tuckcli kv put <путь> <значение>
tuckcli kv delete <путь>
tuckcli kv list <префикс/>

# Управление токенами
tuckcli token create --name=x --policy=y --ttl=24h
tuckcli token get <id>
tuckcli token renew <id> 48h
tuckcli token revoke <id>
tuckcli token list

# Управление политиками
tuckcli policy put <имя> <json-массив-правил|->     # [{"path":"secret/app/*","capabilities":["read","list"]}]
tuckcli policy get <имя>
tuckcli policy delete <имя>
tuckcli policy list
```

Переменные среды: `TUCK_ADDR` (по умолчанию `http://127.0.0.1:8200`), `TUCK_TOKEN`, `TUCK_INSECURE=1` (не проверять самоподписанный сертификат; то же, что `--insecure`), `TUCK_NAMESPACE`. Полный список команд: `tuckcli` без аргументов.

---

## 7. Go SDK

```go
import "github.com/NAGenaev/tuck/pkg/client"

c := client.New("https://tuck:8200", os.Getenv("TUCK_TOKEN")) // + client.WithCAFile(path) / client.WithInsecure()

// KV v1 (путь относительно mount secret/)
_ = c.PutSecret(ctx, "db/password", []byte("s3cr3t"))
val, _ := c.GetSecret(ctx, "db/password")

// KV v2
_, _ = c.KVv2().Write(ctx, "app/config", []byte("val"), 0) // 0 = без CAS
res, _ := c.KVv2().Read(ctx, "app/config", 0)              // 0 = последняя версия; res.Value
```

---

## 8. Архитектура системы

```
┌─────────────────────────────────────────────────────────────┐
│  Уровень HTTP API  (net/http)                                │
│  TLS · Аутентификация · Ограничение частоты · Аудит         │
│  Prometheus · OpenTelemetry · OpenAPI 3.0                    │
├─────────────────────────────────────────────────────────────┤
│  Уровень ядра  (оркестрация и логические операции)           │
│  Хранилище токенов · Хранилище политик · KV v1/v2           │
├───────────────────┬─────────────────────────────────────────┤
│  Методы аутент.   │  Интеграция с Kubernetes                │
│  · Kubernetes SA  │  · Оператор (TuckSecret)                │
│  · JWT / OIDC     │  · Webhook-инжектор                     │
│  · AppRole        │  · CSI-драйвер                          │
│  · LDAP / AD      │                                         │
├───────────────────┴─────────────────────────────────────────┤
│  Криптографический барьер  (AES-256-GCM)                    │
│  корневой ключ → ключ шифрования данных → шифротекст        │
├─────────────────────────────────────────────────────────────┤
│  Физический уровень хранения                                 │
│  BoltDB (одиночный узел) | Raft HA (3–5 узлов, встроенный) │
└─────────────────────────────────────────────────────────────┘
               ▲
         Снятие печати (dev | shamir | transit)
```

---

## 9. Разработка и сборка

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck

go test ./...              # все тесты
go test -race ./...        # с детектором гонок
go build ./cmd/tuck        # бинарный файл сервера
go build ./cmd/tuckcli     # бинарный файл CLI
go build ./cmd/tuck-operator
go build ./cmd/tuck-injector
```

Дополнительная документация: [CONTRIBUTING.md](CONTRIBUTING.md), [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/RUNBOOK.md](docs/RUNBOOK.md).

---

## 10. Безопасность

Политика раскрытия уязвимостей изложена в [SECURITY.md](SECURITY.md). Модель угроз — в [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).

Сообщения об уязвимостях направлять по адресу: **genaevlive@gmail.com** (координированное раскрытие, срок — 90 дней).

---

## 11. Лицензия

Apache-2.0. Подробности — в файле [LICENSE](LICENSE).

Исключение: директория [`web/`](web/) (веб-интерфейс, форк UI-каркаса из [Remnawave](https://github.com/remnawave/frontend)) распространяется отдельно под **AGPL-3.0-only** — см. [`web/LICENSE-AGPL-3.0`](web/LICENSE-AGPL-3.0) и [`web/NOTICE`](web/NOTICE).
