# Tuck — Система управления секретами

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Release](https://img.shields.io/badge/release-v1.35.0-green)](https://github.com/NAGenaev/tuck/releases)

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
| **LDAP / Active Directory** | Аутентификация пользователей каталога; членство в группах отображается на политики |
| **GitHub Actions OIDC** | Аутентификация конвейеров CI/CD без долгосрочных секретов |

### 2.3. Движки динамических секретов

| Движок | Описание |
|--------|----------|
| **База данных** | Временные учётные данные PostgreSQL / MySQL; автоматический отзыв по истечении аренды |
| **AWS IAM** | Временные ключи IAM-пользователя или сессии STS AssumeRole |
| **GCP** | Ключи сервисного аккаунта или токены OAuth2; автоматический отзыв |
| **Azure AD** | Клиентские секреты сервисных участников через Microsoft Graph API |
| **PKI** | Внутренний удостоверяющий центр X.509; выдача TLS-сертификатов по ролям |
| **Transit** | Шифрование как услуга: AES-256-GCM, ECDSA, Ed25519, RSA-PSS; ротация и перешифрование |
| **SSH CA** | Центр сертификации SSH; подпись открытых ключей пользователей и узлов |
| **TOTP** | Хранение ключей TOTP и проверка одноразовых паролей (RFC 6238) |

### 2.4. Подсистема интеграции с Kubernetes

- **Оператор** — CRD `TuckSecret` синхронизирует секреты Tuck с объектами Kubernetes Secret; выбор лидера на основе Lease; политика при удалении (`Retain` / `Delete`)
- **Webhook-инжектор** — MutatingWebhookConfiguration внедряет init-контейнер, записывающий секреты в том tmpfs; данные не попадают в etcd
- **CSI-драйвер** — монтирование секретов как файловой системы SecretProviderClass
- **Helm-чарт** — развёртывание сервера, оператора и инжектора одной командой

### 2.5. Подсистема обеспечения отказоустойчивости

- Встроенный Raft (3–5 узлов), не требующий внешних координаторов (Consul, etcd)
- Автоматическое снятие печати: AWS KMS, GCP Cloud KMS, Azure Key Vault, Transit
- Снятие печати по схеме Шамира (n-of-k): распределённое хранение долей
- Резервное копирование: `GET /v1/sys/snapshot` — потоковый снимок BoltDB

### 2.6. Средства аудита и мониторинга

- Журнал аудита с цепочкой хэшей SHA-256 (значения секретов не записываются)
- Метрики Prometheus на `/metrics`
- Трассировка OpenTelemetry (OTLP-экспортёр)
- Ограничение частоты запросов: token bucket на IP-адрес

---

## 3. Технические характеристики

| Параметр | Значение |
|----------|----------|
| Язык реализации | Go 1.25.11 |
| Размер исполняемого модуля | ~20 МБ |
| Физическое хранилище | BoltDB (одиночный узел) / встроенный Raft (кластер) |
| Шифрование | AES-256-GCM (конвертное) |
| Транспортный протокол | HTTPS (TLS 1.2+, ECDSA P-256 или пользовательский сертификат) |
| HTTP API | 194 эндпоинта, спецификация OpenAPI 3.0 |
| Лицензия | MIT |

---

## 4. Порядок развёртывания

### 4.1. Режим разработки (автоматическое снятие печати)

```sh
go run ./cmd/tuck --seal-type=dev
# tuck: unsealed (dev seal), serving on https://127.0.0.1:8200
# ROOT TOKEN: tuck_...
```

> Режим разработки не предназначен для использования в производственных средах.

### 4.2. Запись и чтение секрета

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_...

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
  --data-dir=/var/lib/tuck
```

При первом запуске система выводит корневой токен и 5 долей Шамира. Доли распределяются операторам раздельно. После перезапуска сервера для снятия печати необходимо предъявить любые 3 доли:

```sh
tuckcli unseal <доля-1>
tuckcli unseal <доля-2>
tuckcli unseal <доля-3>
```

### 4.4. Автоматическое снятие печати — AWS KMS

```sh
tuck \
  --seal-type=awskms \
  --seal-awskms-key-id=alias/tuck-seal \
  --seal-awskms-region=us-east-1 \
  --addr=0.0.0.0:8200 \
  --tls-cert=/etc/tuck/tls.crt \
  --tls-key=/etc/tuck/tls.key
```

Учётные данные получаются автоматически через IRSA или роль экземпляра EC2. Зашифрованный корневой ключ сохраняется в файле `tuck-awskms.enc` и не представляет ценности без доступа к ключу KMS.

### 4.5. Автоматическое снятие печати — GCP Cloud KMS

```sh
tuck \
  --seal-type=gcpkms \
  --seal-gcpkms-key-name=projects/my-project/locations/global/keyRings/tuck/cryptoKeys/seal \
  --addr=0.0.0.0:8200 \
  --tls-cert=/etc/tuck/tls.crt \
  --tls-key=/etc/tuck/tls.key
```

На GKE учётные данные предоставляются автоматически через Workload Identity. Вне GCP необходимо установить переменную `GOOGLE_APPLICATION_CREDENTIALS`.

### 4.6. Автоматическое снятие печати — Azure Key Vault

```sh
tuck \
  --seal-type=azurekv \
  --seal-azurekv-vault-url=https://my-vault.vault.azure.net \
  --seal-azurekv-key-name=tuck-seal \
  --addr=0.0.0.0:8200 \
  --tls-cert=/etc/tuck/tls.crt \
  --tls-key=/etc/tuck/tls.key
```

Учётные данные разрешаются через `DefaultAzureCredential`: Managed Identity (AKS/ВМ), переменные `AZURE_*` (CI), Azure CLI (локальная разработка). Управляемому удостоверению требуется роль `Key Vault Crypto User`.

### 4.7. Развёртывание в Kubernetes через Helm

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

### 5.6. Движок PKI

| Метод | Путь | Аутент. | Описание |
|-------|------|---------|----------|
| POST | `/v1/pki/generate/root` | токен | Создать корневой CA |
| POST | `/v1/pki/import/ca` | токен | Импортировать существующий CA |
| GET | `/v1/pki/ca/pem` | публичный | Сертификат CA |
| GET | `/v1/pki/crl/pem` | публичный | Список отзыва (CRL) |
| PUT/GET/DELETE/LIST | `/v1/pki/roles/{name}` | токен | Управление ролями PKI |
| POST | `/v1/pki/issue/{role}` | токен | Выдать сертификат |
| POST | `/v1/pki/revoke/{serial}` | токен | Отозвать сертификат |

### 5.7. Движок Transit

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/v1/transit/keys/{name}` | Создать ключ |
| GET/DELETE/LIST | `/v1/transit/keys/{name}` | Управление ключами |
| POST | `/v1/transit/keys/{name}/rotate` | Ротация ключа |
| POST | `/v1/transit/encrypt/{name}` | Зашифровать |
| POST | `/v1/transit/decrypt/{name}` | Дешифровать |
| POST | `/v1/transit/rewrap/{name}` | Перешифровать новым ключом |
| POST | `/v1/transit/sign/{name}` | Подписать |
| POST | `/v1/transit/verify/{name}` | Проверить подпись |
| POST | `/v1/transit/hmac/{name}` | Вычислить HMAC |

### 5.8. Движок SSH

| Метод | Путь | Аутент. | Описание |
|-------|------|---------|----------|
| POST | `/v1/ssh/generate/ca` | токен | Создать ключевую пару CA |
| GET | `/v1/ssh/ca/public-key` | публичный | Открытый ключ CA |
| PUT/GET/DELETE/LIST | `/v1/ssh/roles/{name}` | токен | Управление ролями SSH |
| POST | `/v1/ssh/sign/{role}` | токен | Подписать открытый ключ |

### 5.9. Движок TOTP

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/v1/totp/keys/{name}` | Создать ключ (возвращает URL `otpauth://`) |
| GET/DELETE/LIST | `/v1/totp/keys/{name}` | Управление ключами |
| GET | `/v1/totp/code/{name}` | Получить текущий код |
| POST | `/v1/totp/code/{name}` | Проверить код |

### 5.10. Служебные эндпоинты

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
tuckcli policy put <имя> <json-правила>
tuckcli policy get <имя>
tuckcli policy delete <имя>
tuckcli policy list
```

Переменные среды: `TUCK_ADDR` (по умолчанию `https://127.0.0.1:8200`), `TUCK_TOKEN`.

---

## 7. Go SDK

```go
import "github.com/NAGenaev/tuck/pkg/client"

c, err := client.New(client.Config{
    Addr:  "https://tuck:8200",
    Token: os.Getenv("TUCK_TOKEN"),
})

// KV v1
_ = c.KV().Put(ctx, "secret/db/password", "s3cr3t")
val, _ := c.KV().Get(ctx, "secret/db/password")

// KV v2
_ = c.KVv2().Put(ctx, "app/config", map[string]string{"key": "val"})
secret, _ := c.KVv2().Get(ctx, "app/config", 0) // 0 = последняя версия

// Transit
ct, _ := c.Transit().Encrypt(ctx, "payments", []byte("данные"))
pt, _ := c.Transit().Decrypt(ctx, "payments", ct)

// PKI
cert, _ := c.PKI().Issue(ctx, "pki", "web", client.PKIIssueRequest{
    CommonName: "api.example.com",
    TTL:        "720h",
})
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
│  Методы аутент.   │  Движки динамических секретов           │
│  · Kubernetes SA  │  · База данных (PostgreSQL / MySQL)      │
│  · JWT / OIDC     │  · PKI (удостоверяющий центр X.509)     │
│  · AppRole        │  · Transit (шифрование как услуга)      │
│  · LDAP / AD      │  · SSH (сертификаты в режиме CA)        │
│  · GitHub OIDC    │  · TOTP (одноразовые пароли)            │
├───────────────────┴─────────────────────────────────────────┤
│  Криптографический барьер  (AES-256-GCM)                    │
│  корневой ключ → ключ шифрования данных → шифротекст        │
├─────────────────────────────────────────────────────────────┤
│  Физический уровень хранения                                 │
│  BoltDB (одиночный узел) | Raft HA (3–5 узлов, встроенный) │
└─────────────────────────────────────────────────────────────┘
               ▲
         Снятие печати (dev | shamir | transit | awskms | gcpkms | azurekv)
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

MIT. Подробности — в файле [LICENSE](LICENSE).
