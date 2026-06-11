# Tuck — архитектура и требования

## Что такое Tuck

Минималистичный менеджер секретов, написанный на Go.
Anti-Vault: один бинарь, нет внешней БД, нет Consul, нет etcd.
Ориентирован на деплой в Kubernetes рядом с рабочей нагрузкой.

**Принципы:**
- Единственная внешняя зависимость — bbolt (embedded BoltDB)
- Zero external deps в runtime (нет gRPC-фреймворков, нет client-go)
- Всё хранимое — зашифровано AES-256-GCM

---

## Компоненты

```
cmd/tuck/            — HTTP-сервер (точка входа)
cmd/tuck-operator/   — Kubernetes-оператор (TuckSecret CRD)

deploy/
  crd/               — TuckSecret CRD
  server/            — Tuck-сервер в k8s (dev seal) + RBAC
  operator/          — оператор (+ local.yaml для minikube)
  console/           — OpenShift Console UI на minikube
  examples/          — примеры TuckSecret
build/               — Dockerfile.server, Dockerfile.operator

internal/
  physical/   — физический слой: bbolt + in-memory (для тестов)
  barrier/    — криптографический барьер: AES-256-GCM, sealed/unsealed
  seal/       — способы хранения/получения root key (dev / shamir / transit)
  shamir/     — математика Shamir's Secret Sharing: GF(256), Split/Combine
  core/       — бизнес-логика: tokens, policies, k8s auth, KV secrets
  token/      — модель токена: генерация, TTL, хранение в barrier
  policy/     — ACL: политики, glob-матчинг путей, проверка capability
  api/        — HTTP-слой: роутинг, middleware, сериализация
  k8s/        — Kubernetes TokenReview клиент + RoleStore
  operator/   — контроллер TuckSecret CRD
```

---

## Слои и поток данных

```
Клиент (curl / tuck CLI / оператор)
        │  HTTP
        ▼
  api.Server   ←── X-Tuck-Token header
        │
        ▼
  core.Core    ←── Authenticate → EnforceAccess (policy check)
        │
        ▼
  barrier.Barrier  ←── AES-256-GCM encrypt/decrypt
        │
        ▼
  physical.Backend ←── bbolt (только зашифрованные байты)
```

**barrier** отвечает за то, что в bbolt никогда не попадают открытые данные.
Пока barrier sealed (root key в памяти = nil) — любой запрос возвращает 503.

---

## Криптография

```
root key (32 байта)   ←── хранится только в памяти, получен через seal
     │
     └──▶ barrier key (DEK, AES-256)
               │
               └──▶ AES-256-GCM(nonce, plaintext) ──▶ bbolt
```

**Envelope encryption:** root key шифрует DEK, DEK шифрует данные.
При рестарте: seal.Unseal() → root key → barrier.Unseal() → DEK расшифрован → работа.

---

## Типы seal

### dev (разработка)
- Root key хранится **открытым текстом** в файле
- Auto-unseal при старте
- **Только для dev/тестов**

### shamir (on-prem, multi-operator)
- Root key разбивается на N шардов через Shamir's Secret Sharing в GF(256)
- Каждый шард — base64url(x || f(x))
- Ни один шард сам по себе не восстанавливает ключ
- При старте сервер остаётся **sealed**
- Операторы по одному отправляют свои шарды через `POST /v1/sys/unseal`
- После K шардов barrier unseals автоматически
- При рестарте — процесс повторяется (шарды в памяти не сохраняются)

### transit (cloud, auto-unseal)
- Root key **оборачивается** внешним KMS-сервисом при инициализации
- Зашифрованный blob хранится в локальном файле
- При старте: читает blob → POST на unwrap endpoint → получает root key → auto-unseal
- Совместим с Vault Transit API (`/v1/transit/encrypt/<key>`, `/v1/transit/decrypt/<key>`)

---

## Хранилище (bbolt)

Все ключи имеют логический путь. barrier шифрует значение перед записью.

| Логический путь                       | Что хранится              |
|---------------------------------------|---------------------------|
| `auth/token/<id>`                     | JSON токена               |
| `auth/policy/<name>`                  | JSON политики             |
| `auth/k8s/role/<namespace>/<sa>`      | JSON k8s role binding     |
| `secret/<path>`                       | Байты секрета (plaintext) |
| `barrier/keyring`                     | DEK, зашифрованный root key |

---

## Токены

Формат: `tuck_` + base64url(32 случайных байта)

Поля: `id`, `display_name`, `policies []string`, `created_at`, `expires_at`

**Root-токен**: единственный токен с политикой `root`. Создаётся при первом старте,
выводится в лог один раз. Root-политика хардкодирована — её нельзя удалить через API.

---

## Политики (ACL)

```json
{
  "name": "db-readwrite",
  "rules": [
    {"path": "secret/db/*",    "capabilities": ["read", "write", "delete"]},
    {"path": "secret/shared/*","capabilities": ["read"]}
  ]
}
```

Capabilities: `read`, `write`, `delete`, `list`

Пути — glob-матчинг: `secret/db/*` матчит `secret/db/password` но не `secret/db/sub/key`.
`secret/**` матчит любую глубину.

---

## HTTP API

Базовый URL: `http://localhost:8200`
Аутентификация: заголовок `X-Tuck-Token: tuck_...`

### Sys

| Метод | Путь                  | Auth | Описание                              |
|-------|-----------------------|------|---------------------------------------|
| GET   | /v1/sys/seal-status   | нет  | Текущий статус: sealed, тип, шарды    |
| POST  | /v1/sys/unseal        | нет  | Передать один Shamir-шард             |
| POST  | /v1/sys/seal          | root | Принудительно запечатать              |

### Health

| Метод | Путь       | Auth | Описание           |
|-------|------------|------|--------------------|
| GET   | /v1/health | нет  | `{"sealed": false}`|

### Секреты (KV)

| Метод  | Путь                     | Auth | Описание         |
|--------|--------------------------|------|------------------|
| GET    | /v1/secret/{path...}     | да   | Получить секрет  |
| PUT    | /v1/secret/{path...}     | да   | Записать секрет  |
| DELETE | /v1/secret/{path...}     | да   | Удалить секрет   |

### Токены

| Метод  | Путь                  | Auth | Описание         |
|--------|-----------------------|------|------------------|
| POST   | /v1/auth/token        | да   | Создать токен    |
| GET    | /v1/auth/token/{id}   | да   | Посмотреть токен |
| DELETE | /v1/auth/token/{id}   | да   | Отозвать токен   |

### Политики

| Метод  | Путь               | Auth | Описание           |
|--------|--------------------|------|--------------------|
|PUT     | /v1/policy/{name}  | да   | Создать/обновить   |
| GET    | /v1/policy/{name}  | да   | Получить           |
| DELETE | /v1/policy/{name}  | да   | Удалить            |

### Kubernetes Auth

| Метод  | Путь                                    | Auth | Описание               |
|--------|-----------------------------------------|------|------------------------|
| POST   | /v1/auth/kubernetes/login               | нет  | Войти с SA токеном     |
| PUT    | /v1/auth/kubernetes/role/{ns}/{sa}      | да   | Создать role binding   |
| GET    | /v1/auth/kubernetes/role/{ns}/{sa}      | да   | Посмотреть role binding|
| DELETE | /v1/auth/kubernetes/role/{ns}/{sa}      | да   | Удалить role binding   |

---

## Оператор (TuckSecret CRD)

Оператор синхронизирует секреты из Tuck в Kubernetes Secrets.

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: production
spec:
  tuckPath: db/password          # путь в Tuck (без /v1/secret/)
  secretName: db-credentials     # имя K8s Secret для создания/обновления
  secretKey: password            # ключ внутри K8s Secret .data
  refreshInterval: "5m"          # как часто обновлять (default: 5m)
```

**Жизненный цикл:**
1. Оператор логинится в Tuck через SA токен (`/v1/auth/kubernetes/login`)
2. Токен Tuck кэшируется (TTL 4 мин, обновляется за 30 сек до истечения)
3. Watch на CRD + периодический reconcile
4. При ADDED/MODIFIED: GET secret из Tuck → ApplySecret в K8s
5. При DELETED: role binding удаляется, K8s Secret **не трогается** (безопасное поведение)
