# Tuck — Руководство по ручному тестированию

> Версия: v1.35.0 · Обновлено: 2026-06-21  
> Документ описывает все модули Tuck и позволяет проверить каждый из них вручную.

---

## Содержание

| # | Модуль | Тест-кейсы |
|---|--------|-----------|
| 1 | [CORE — Запуск, состояние, seal/unseal](#1-core) | CORE-1…CORE-5 |
| 2 | [KV-v1 — Key-Value v1](#2-kv-v1) | KV1-1…KV1-5 |
| 3 | [KV-v2 — Версионирование секретов](#3-kv-v2) | KV2-1…KV2-6 |
| 4 | [CUBBYHOLE — Личное хранилище токена](#4-cubbyhole) | CUBB-1…CUBB-3 |
| 5 | [WRAP — Response Wrapping](#5-response-wrapping) | WRAP-1…WRAP-3 |
| 6 | [TOKEN — Жизненный цикл токенов](#6-tokens) | TOK-1…TOK-6 |
| 7 | [POLICY — Политики ACL](#7-policies) | POL-1…POL-4 |
| 8 | [APPROLE — AppRole аутентификация](#8-approle) | AR-1…AR-5 |
| 9 | [JWT — JWT/OIDC аутентификация](#9-jwt-oidc) | JWT-1…JWT-3 |
| 10 | [LDAP — LDAP/AD аутентификация](#10-ldap) | LDAP-1…LDAP-3 |
| 11 | [K8S — Kubernetes аутентификация](#11-kubernetes-auth) | K8S-1…K8S-2 |
| 12 | [TRANSIT — Шифрование как сервис](#12-transit) | TR-1…TR-8 |
| 13 | [PKI — Центр сертификации](#13-pki) | PKI-1…PKI-5 |
| 14 | [SSH — Подпись SSH-ключей](#14-ssh) | SSH-1…SSH-4 |
| 15 | [TOTP — Двухфакторная аутентификация](#15-totp) | TOTP-1…TOTP-4 |
| 16 | [SHAMIR — Ручное распечатывание](#16-shamir-seal) | SH-1…SH-3 |
| 17 | [BACKUP — Backup и Restore](#17-backup--restore) | BAK-1…BAK-2 |
| 18 | [METRICS — Prometheus-метрики](#18-metrics) | MET-1…MET-2 |
| 19 | [NAMESPACE — Изоляция пространств](#19-namespaces) | NS-1…NS-2 |
| 20 | [DYNAMIC DB — Динамические секреты БД](#20-dynamic-secrets--database) | DB-1…DB-2 |

---

## Подготовка (один раз)

### Сборка и запуск

```powershell
# Из корня репозитория:
go build -o tuck.exe ./cmd/tuck

# Запуск в dev-режиме (авто-распечатывание, in-memory)
.\tuck.exe server -dev
```

При запуске в dev-режиме сервер выводит root-токен в консоль:
```
Root Token: tuck_s.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Переменные окружения

```powershell
$env:TUCK_ADDR  = "http://localhost:8200"
$env:TUCK_TOKEN = "tuck_s.xxxxxxxxxxxxxxxx"  # вставьте ваш root token
```

### Вспомогательная функция

```powershell
function tuck {
    param([string]$Method, [string]$Path, $Body=$null)
    $uri = "$env:TUCK_ADDR$Path"
    $headers = @{ "X-Tuck-Token" = $env:TUCK_TOKEN; "Content-Type" = "application/json" }
    if ($Body) {
        Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers -Body ($Body | ConvertTo-Json -Depth 10)
    } else {
        Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers
    }
}
```

Или используйте `curl.exe` напрямую (примеры ниже дублируются для curl).

---

## 1. CORE

Базовые эндпоинты, доступные без токена.

### CORE-1: Проверка состояния (seal status)

```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/seal-status
```

**Ожидается:**
```json
{ "sealed": false, "type": "dev" }
```

### CORE-2: Readiness probe

```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/ready
```

**Ожидается:** `{ "ready": true }` с HTTP 200.  
**При запечатанном сервере:** HTTP 503 `{ "sealed": true }`.

### CORE-3: Ручное запечатывание

```powershell
Invoke-RestMethod -Method POST http://localhost:8200/v1/sys/seal `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** HTTP 200. После этого `seal-status.sealed == true`.

### CORE-4: Health endpoint

```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/health
```

**Ожидается:** `{ "initialized": true, "sealed": false, "version": "1.35.0" }`.

### CORE-5: Версия

```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/version
```

**Ожидается:** `{ "version": "1.35.0", ... }`.

---

## 2. KV-v1

Простое хранилище ключ-значение (без версионирования).

### KV1-1: Запись секрета

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/myapp/db" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"host":"postgres:5432","password":"hunter2"}'
```

**Ожидается:** HTTP 204 (нет тела).

### KV1-2: Чтение секрета

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/secret/myapp/db" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:**
```json
{ "host": "postgres:5432", "password": "hunter2" }
```

### KV1-3: Листинг

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/secret/myapp/?list=true" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** `{ "keys": ["db"] }`.

### KV1-4: Обновление (перезапись)

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/myapp/db" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"host":"postgres:5432","password":"new-pass"}'
```

Затем прочитайте — `password` должен стать `new-pass`.

### KV1-5: Удаление

```powershell
curl.exe -s -XDELETE "$env:TUCK_ADDR/v1/secret/myapp/db" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** HTTP 204. Последующий GET → HTTP 404.

---

## 3. KV-v2

Версионированное хранилище. Все операции через `/v1/secret/data/` (данные) и `/v1/secret/metadata/` (метаданные).

### KV2-1: Запись (версия 1)

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/data/config/api" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"data":{"url":"https://api.example.com","key":"abc"}}'
```

**Ожидается:** `{ "version": 1, "created_time": "..." }`.

### KV2-2: Обновление (версия 2)

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/data/config/api" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"data":{"url":"https://api-v2.example.com","key":"xyz"}}'
```

**Ожидается:** `{ "version": 2 }`.

### KV2-3: Чтение текущей версии

```powershell
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/data/config/api" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** данные версии 2.

### KV2-4: Чтение конкретной версии

```powershell
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/data/config/api?version=1" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** данные версии 1 (`url=https://api.example.com`).

### KV2-5: Метаданные (список версий)

```powershell
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/metadata/config/api" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** `{ "versions": { "1": {...}, "2": {...} }, "current_version": 2 }`.

### KV2-6: CAS (Check-And-Set)

```powershell
# Запись с CAS=2 (текущая версия) — должна пройти
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/data/config/api" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"options":{"cas":2},"data":{"url":"https://cas-ok.example.com"}}'

# Запись с CAS=1 (устаревшая версия) — должна вернуть 409
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/data/config/api" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"options":{"cas":1},"data":{"url":"https://cas-fail.example.com"}}'
```

**Ожидается:** первый → HTTP 200; второй → HTTP 409 Conflict.

---

## 4. Cubbyhole

Персональное изолированное хранилище: каждый токен видит только своё.

### CUBB-1: Запись в cubbyhole

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/cubbyhole/my-data" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"secret":"only-i-can-see"}'
```

### CUBB-2: Чтение из cubbyhole

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/cubbyhole/my-data" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** `{ "secret": "only-i-can-see" }`.

### CUBB-3: Изоляция (другой токен не видит)

```powershell
# Создать второй токен
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["root"]}' -ContentType application/json
$tok2 = $r.token

# Попытаться прочитать cubbyhole первого токена вторым
curl.exe -s "$env:TUCK_ADDR/v1/cubbyhole/my-data" `
    -H "X-Tuck-Token: $tok2"
```

**Ожидается:** HTTP 404 — cubbyhole изолирован.

---

## 5. Response Wrapping

Произвольный JSON-документ оборачивается в одноразовый wrap-токен через отдельные `sys/wrapping/*`
эндпоинты — это НЕ заголовок на обычном `GET /v1/secret/...` (см. `internal/api/wrapping.go`).

### WRAP-1: Оборачивание секрета

```powershell
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/wrapping/wrap" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -d '{"data":{"host":"db.internal","password":"s3cr3t"},"ttl":"60s"}'
```

**Ожидается:** `{ "token": "tuck_wrap_...", "expires_at": "..." }` вместо самого секрета.

### WRAP-2: Разворачивание

```powershell
$wrapTok = "tuck_wrap_..."  # из предыдущего шага
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/wrapping/unwrap" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -d "{`"token`":`"$wrapTok`"}"
```

**Ожидается:** оригинальные данные: `{ "data": { "host": "...", "password": "..." } }`.

### WRAP-3: Одноразовость

```powershell
# Повторный unwrap тем же токеном — должен вернуть 404
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/wrapping/unwrap" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -d "{`"token`":`"$wrapTok`"}"
```

**Ожидается:** HTTP 404 — wrap-токен уже использован (просроченный/несуществующий даёт тот же код;
`wrapping.ErrExpired` отдельно маппится на 410).

### WRAP-4: Lookup и revoke без разворачивания

```powershell
# Метаданные токена без его "траты"
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/wrapping/lookup" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -d "{`"token`":`"$wrapTok`"}"

# Досрочный отзыв (тоже не требует прав на исходные данные)
curl.exe -s -XDELETE "$env:TUCK_ADDR/v1/sys/wrapping/revoke" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -d "{`"token`":`"$wrapTok`"}"
```

**Ожидается:** lookup — `{"creation_time":"...","expires_at":"...","creation_ttl":...}`; revoke — HTTP 204.

---

## 6. Tokens

### TOK-1: Создание дочернего токена

```powershell
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/token/create" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"policies":["default"],"ttl":"1h","display_name":"test-token"}'
```

**Ожидается:** `{ "token": "tuck_s.xxx", "policies": ["default"], "ttl": 3600 }`.

### TOK-2: Lookup-self (информация о текущем токене)

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/auth/token/lookup-self" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** `{ "id": "...", "policies": ["root"], "renewable": false, "ttl": 0 }`.

### TOK-3: Токен с max_uses

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["root"],"num_uses":2}' -ContentType application/json
$useTok = $r.token

# Использование 1 — OK
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/test" -Headers @{"X-Tuck-Token"=$useTok}

# Использование 2 — OK (последнее)
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/test" -Headers @{"X-Tuck-Token"=$useTok}

# Использование 3 — 403 (токен исчерпан)
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/test" -Headers @{"X-Tuck-Token"=$useTok}
```

### TOK-4: Renew-self

```powershell
# Создать токен с коротким TTL
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["root"],"ttl":"30s","renewable":true}' -ContentType application/json
$shortTok = $r.token

# Продлить
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/token/renew-self" `
    -H "X-Tuck-Token: $shortTok" `
    -H "Content-Type: application/json" `
    -d '{"ttl":"2m"}'
```

**Ожидается:** HTTP 200 с обновлённым `ttl`.

### TOK-5: Revoke-self

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["root"]}' -ContentType application/json
$rTok = $r.token

curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/token/revoke-self" `
    -H "X-Tuck-Token: $rTok"
```

**Ожидается:** HTTP 204. Последующий запрос с `$rTok` → 403.

### TOK-6: Accessor lookup

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["root"]}' -ContentType application/json
$accessor = $r.accessor

# Lookup по accessor (не нужен сам токен)
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/token/lookup-accessor" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d "{`"accessor`":`"$accessor`"}"
```

**Ожидается:** метаданные токена без раскрытия самого ID.

---

## 7. Policies

### POL-1: Создание политики

```powershell
$rules = @'
[
  {"path":"secret/myapp/*","capabilities":["read","write","delete"]},
  {"path":"secret/other/*","capabilities":["deny"]}
]
'@

curl.exe -s -XPUT "$env:TUCK_ADDR/v1/policy/myapp-policy" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d "{`"rules`":$rules}"
```

**Ожидается:** HTTP 204.

### POL-2: Чтение политики

```powershell
Invoke-RestMethod "$env:TUCK_ADDR/v1/policy/myapp-policy" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

### POL-3: Проверка deny-правила

```powershell
# Создать токен с политикой myapp-policy
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/token/create" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"policies":["myapp-policy"]}' -ContentType application/json
$restricted = $r.token

# Разрешённый путь — OK
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/myapp/key" `
    -H "X-Tuck-Token: $restricted" `
    -H "Content-Type: application/json" `
    -d '"value"'

# Запрещённый путь (deny) — 403
curl.exe -s "$env:TUCK_ADDR/v1/secret/other/key" `
    -H "X-Tuck-Token: $restricted"
```

### POL-4: Листинг политик

```powershell
Invoke-RestMethod "$env:TUCK_ADDR/v1/policy/?list=true" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** список включает `myapp-policy`, `root`, `default`.

---

## 8. AppRole

AppRole — метод аутентификации сервисов: `role_id` (публичный идентификатор) + `secret_id` (одноразовый секрет).

### AR-1: Создание роли

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/approle/role/myapp" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"policies":["myapp-policy"],"token_ttl":"1h","secret_id_ttl":"24h","secret_id_num_uses":10}'
```

### AR-2: Получение role_id

```powershell
$role = Invoke-RestMethod "$env:TUCK_ADDR/v1/auth/approle/role/myapp/role-id" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
$roleId = $role.role_id
Write-Host "role_id: $roleId"
```

### AR-3: Генерация secret_id

```powershell
$sid = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/approle/role/myapp/secret-id" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
$secretId = $sid.secret_id
Write-Host "secret_id: $secretId"
```

### AR-4: Логин (получение токена)

```powershell
$login = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/approle/login" `
    -Body "{`"role_id`":`"$roleId`",`"secret_id`":`"$secretId`"}" `
    -ContentType application/json
$appTok = $login.token
Write-Host "token: $appTok"
```

**Ожидается:** токен с политиками из роли `myapp`.

### AR-5: Использование токена

```powershell
# Работает в рамках политики
curl.exe -s "$env:TUCK_ADDR/v1/secret/myapp/key" `
    -H "X-Tuck-Token: $appTok"

# Отклоняется за пределами политики
curl.exe -s "$env:TUCK_ADDR/v1/secret/admin/key" `
    -H "X-Tuck-Token: $appTok"
```

---

## 9. JWT/OIDC

Требует: настроенный JWKS endpoint (можно использовать любой OIDC-провайдер: Keycloak, Auth0, Dex) или сгенерировать вручную через OpenSSL + локальный HTTP-сервер.

> **Быстрый вариант для тестирования:** запустите Dex в Docker или используйте реальный Keycloak.

### JWT-1: Настройка JWT auth

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/jwt/config" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d "{`"jwks_uri`":`"https://your-oidc-provider/.well-known/jwks.json`",`"issuer`":`"https://your-oidc-provider`",`"default_ttl`":`"1h`"}"
```

### JWT-2: Создание роли

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/jwt/role/webapp" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"bound_subject":"service-account-1","policies":["myapp-policy"],"ttl":"1h"}'
```

### JWT-3: Логин с JWT-токеном

```powershell
$jwt = "eyJ..."  # JWT-токен от вашего OIDC-провайдера

$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/jwt/login" `
    -Body "{`"jwt`":`"$jwt`"}" `
    -ContentType application/json
Write-Host "token: $($r.token)"
```

> Полный пример с автоматически запускаемым JWKS-сервером см. в `internal/api/jwt_test.go`.

---

## 10. LDAP

Требует: LDAP-сервер (OpenLDAP, Active Directory, или Docker `osixia/openldap`).

### Запуск тестового LDAP (Docker)

```powershell
docker run -d --name openldap -p 389:389 `
    -e LDAP_ORGANISATION="Test" -e LDAP_DOMAIN="test.local" `
    -e LDAP_ADMIN_PASSWORD="admin" `
    osixia/openldap:latest
```

### LDAP-1: Настройка LDAP auth

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/ldap/config" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{
      "urls": ["ldap://localhost:389"],
      "bind_dn": "cn=admin,dc=test,dc=local",
      "bind_password": "admin",
      "user_dn": "ou=users,dc=test,dc=local"
    }'
```

### LDAP-2: Создание роли по группе

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/ldap/role/devs" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"groups":["developers"],"policies":["myapp-policy"],"ttl":"8h"}'
```

### LDAP-3: Логин

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/auth/ldap/login" `
    -Body '{"username":"alice","password":"alicepass"}' `
    -ContentType application/json
Write-Host "token: $($r.token)"
```

---

## 11. Kubernetes Auth

Требует: кластер Kubernetes (minikube, kind, или реальный).

### K8S-1: Настройка K8s auth

```powershell
# Получите SA token и CA cert из кластера
$saToken = kubectl get secret ... -o jsonpath='{.data.token}' | base64 -d
$caCert  = kubectl get secret ... -o jsonpath='{.data.ca\.crt}'

curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/kubernetes/config" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d "{`"kubernetes_host`":`"https://kubernetes.default.svc`",`"kubernetes_ca_cert`":`"$caCert`"}"
```

### K8S-2: Привязка роли и логин

```powershell
# Привязать роль к namespace/serviceaccount
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/kubernetes/role/default/myapp" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"policies":["myapp-policy"],"ttl":"1h"}'

# Логин от имени SA (из пода)
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/kubernetes/login" `
    -H "Content-Type: application/json" `
    -d "{`"token`":`"$saToken`"}"
```

> Подробный гайд: [guides/09-kubernetes-auth.md](guides/09-kubernetes-auth.md).

---

## 12. Transit

Encryption-as-a-service. Tuck управляет ключами — данные шифруются/расшифровываются на сервере, в хранилище попадает только шифртекст.

### TR-1: Создание ключа (AES-256)

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/transit/keys/mykey" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"type":"aes256-gcm96"}'
```

**Ожидается:** HTTP 204.

### TR-2: Шифрование данных

```powershell
# plaintext должен быть в base64url (без padding)
$plain = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("Hello, World!")).TrimEnd('=').Replace('+','-').Replace('/','_')

$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/encrypt/mykey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"plaintext`":`"$plain`"}" `
    -ContentType application/json
$cipher = $r.ciphertext
Write-Host "ciphertext: $cipher"
```

**Ожидается:** `vault:v1:...` (формат ciphertext).

### TR-3: Расшифровка

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/decrypt/mykey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"ciphertext`":`"$cipher`"}" `
    -ContentType application/json

# Декодировать plaintext из base64url
$decoded = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($r.plaintext.Replace('-','+').Replace('_','/')))
Write-Host "plaintext: $decoded"
```

**Ожидается:** `Hello, World!`.

### TR-4: Ротация ключа

```powershell
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/transit/keys/mykey/rotate" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** HTTP 204. Новые шифрования используют v2, старый ciphertext (v1) всё ещё расшифровывается.

### TR-5: Rewrap (перешифрование к новой версии ключа)

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/rewrap/mykey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"ciphertext`":`"$cipher`"}" `
    -ContentType application/json
Write-Host "new ciphertext: $($r.ciphertext)"
```

**Ожидается:** `vault:v2:...` — тот же plaintext, новая версия ключа.

### TR-6: Подпись данных (ECDSA-P256)

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/transit/keys/sigkey" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"type":"ecdsa-p256"}'

$input = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("data-to-sign")).TrimEnd('=').Replace('+','-').Replace('/','_')

$sig = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/sign/sigkey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"input`":`"$input`"}" `
    -ContentType application/json
Write-Host "signature: $($sig.signature)"
```

### TR-7: Верификация подписи

```powershell
$v = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/verify/sigkey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"input`":`"$input`",`"signature`":`"$($sig.signature)`"}" `
    -ContentType application/json
Write-Host "valid: $($v.valid)"
```

**Ожидается:** `"valid": true`.

### TR-8: HMAC

```powershell
$r = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/transit/hmac/mykey" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"input`":`"$input`"}" `
    -ContentType application/json
Write-Host "hmac: $($r.hmac)"
```

**Ожидается:** `vault:v1:hmac-sha256:...`.

---

## 13. PKI

Встроенный центр сертификации для выпуска TLS-сертификатов.

### PKI-1: Генерация Root CA

```powershell
$ca = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/pki/root/generate" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"common_name":"Tuck Root CA","ttl":"87600h"}' `
    -ContentType application/json
Write-Host "CA cert:"
Write-Host $ca.certificate
```

**Ожидается:** PEM-сертификат Root CA.

### PKI-2: Создание роли для выпуска сертификатов

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/pki/roles/web-server" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"allowed_domains":["example.com","*.example.com"],"allow_subdomains":true,"ttl":"720h"}'
```

### PKI-3: Выпуск сертификата

```powershell
$cert = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/pki/issue/web-server" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"common_name":"api.example.com","ttl":"24h"}' `
    -ContentType application/json

Write-Host "Certificate:"
Write-Host $cert.certificate
Write-Host "Private key:"
Write-Host $cert.private_key
```

**Ожидается:** PEM certificate + private key. Проверьте цепочку через:
```powershell
$cert.certificate | openssl verify -CAfile <(Write-Output $ca.certificate)
```

### PKI-4: Отзыв сертификата

```powershell
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/pki/revoke" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d "{`"serial_number`":`"$($cert.serial_number)`"}"
```

**Ожидается:** `{ "revocation_time": ... }`.

### PKI-5: CRL (список отозванных сертификатов)

```powershell
# CRL доступен без авторизации (как у настоящих CA)
Invoke-RestMethod "$env:TUCK_ADDR/v1/pki/crl" -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** список серийных номеров отозванных сертификатов.

---

## 14. SSH

Tuck действует как SSH Certificate Authority. Подписывает публичные SSH-ключи — не нужно распространять authorized_keys.

### SSH-1: Генерация CA

```powershell
$caKey = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/ssh/ca/generate" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"key_type":"ed25519"}' `
    -ContentType application/json
Write-Host "SSH CA public key:"
Write-Host $caKey.public_key
```

**Ожидается:** публичный ключ в формате `ssh-ed25519 AAAA...`.

### SSH-2: Просмотр CA публичного ключа

```powershell
# Этот endpoint открыт (для настройки TrustedUserCAKeys на серверах)
Invoke-RestMethod "$env:TUCK_ADDR/v1/ssh/ca/public-key"
```

### SSH-3: Создание роли

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/ssh/roles/developers" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{
      "key_type": "ca",
      "allowed_users": "ubuntu,ec2-user",
      "default_extensions": {"permit-pty":"","permit-port-forwarding":""},
      "ttl": "12h"
    }'
```

### SSH-4: Подпись пользовательского ключа

```powershell
# Сгенерируйте ключ (если нет)
ssh-keygen -t ed25519 -f $env:USERPROFILE\.ssh\test_key -N ""
$pubKey = Get-Content "$env:USERPROFILE\.ssh\test_key.pub"

# Подпишите
$signed = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/ssh/sign/developers" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"public_key`":`"$pubKey`",`"valid_principals`":`"ubuntu`"}" `
    -ContentType application/json

Write-Host "Signed cert:"
Write-Host $signed.signed_key

# Проверьте сертификат
$signed.signed_key | Out-File -Encoding ascii "$env:USERPROFILE\.ssh\test_key-cert.pub"
ssh-keygen -Lf "$env:USERPROFILE\.ssh\test_key-cert.pub"
```

**Ожидается:** `ssh-ed25519-cert-v01@openssh.com ...` с указанными principals и TTL.

---

## 15. TOTP

Time-based One-Time Password (RFC 6238 / Google Authenticator). Tuck может выступать как сервер TOTP.

### TOTP-1: Создание TOTP-ключа

```powershell
$key = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/totp/keys/myuser" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body '{"generate":true,"issuer":"Tuck","account_name":"alice@example.com"}' `
    -ContentType application/json

Write-Host "Secret: $($key.secret)"
Write-Host "QR URL: $($key.url)"
```

**Ожидается:** base32 secret + `otpauth://totp/...` URL (откройте в Google Authenticator или Authy).

### TOTP-2: Генерация кода

```powershell
$code = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/totp/code/myuser" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
Write-Host "TOTP code: $($code.code)"
```

**Ожидается:** 6-значный код, совпадающий с тем, что показывает аутентификатор.

### TOTP-3: Валидация кода

```powershell
$valid = Invoke-RestMethod -Method POST "$env:TUCK_ADDR/v1/totp/validate/myuser" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN} `
    -Body "{`"code`":`"$($code.code)`"}" `
    -ContentType application/json
Write-Host "valid: $($valid.valid)"
```

**Ожидается:** `"valid": true`. Один код действителен 30 секунд и только один раз.

### TOTP-4: Импорт существующего секрета

```powershell
# Если у вас уже есть TOTP-секрет (например, из существующего сервиса)
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/totp/keys/existing" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"key":"JBSWY3DPEHPK3PXP","issuer":"MyApp","account_name":"user@example.com"}'
```

---

## 16. Shamir Seal

Распределённое ручное распечатывание: ключ разделён на `n` частей, для открытия нужны любые `k`.

### Запуск в режиме Shamir

```powershell
# Конфиг shamir.json:
# { "type": "shamir", "shares": 5, "threshold": 3 }
.\tuck.exe server -config shamir.json
```

При первом запуске сервер выводит:
```
Share 1: abc123...
Share 2: def456...
...
Root Token: tuck_s.xxx
```

**СОХРАНИТЕ ЭТИ ДОЛИ В НАДЁЖНОМ МЕСТЕ.** После рестарта они потребуются.

### SH-1: Статус (запечатан)

После рестарта без unseal-операций:
```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/seal-status
```
**Ожидается:** `{ "sealed": true, "shares": 5, "threshold": 3, "progress": 0 }`.

### SH-2: Поочерёдная подача долей

```powershell
$shares = @("abc123...", "def456...", "ghi789...")

foreach ($share in $shares) {
    $r = Invoke-RestMethod -Method POST http://localhost:8200/v1/sys/unseal `
        -Body "{`"key`":`"$share`"}" `
        -ContentType application/json
    Write-Host "sealed=$($r.sealed) progress=$($r.progress)"
}
```

**Ожидается:** после `k` долей `sealed=false, message="unseal complete"`.

### SH-3: Проверка после unseal

```powershell
Invoke-RestMethod http://localhost:8200/v1/sys/seal-status
# → sealed=false

# Старый root token всё ещё работает
curl.exe -s "$env:TUCK_ADDR/v1/secret/test" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

---

## 17. Backup / Restore

### BAK-1: Создание snapshot

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/sys/snapshot" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    --output backup-$(Get-Date -Format yyyyMMdd).snap

Write-Host "Backup created: backup-$(Get-Date -Format yyyyMMdd).snap"
```

**Ожидается:** бинарный файл snapshot (~несколько КБ для in-memory backend).

### BAK-2: Восстановление из snapshot

```powershell
# Запустите новый чистый инстанс Tuck
# Затем:
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/snapshot/restore" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    --data-binary @backup-20260621.snap

# После восстановления проверьте данные
Invoke-RestMethod "$env:TUCK_ADDR/v1/secret/myapp/db" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

**Ожидается:** данные, записанные до backup, доступны на новом инстансе.

---

## 18. Metrics

### MET-1: Prometheus метрики

```powershell
Invoke-RestMethod http://localhost:8200/metrics
```

**Ожидается:** текстовый вывод в формате Prometheus. Ищите ключевые метрики:
- `tuck_kv_requests_total` — число запросов к KV
- `tuck_barrier_encrypt_count` — число операций шифрования
- `tuck_token_count` — активные токены
- `go_goroutines` — число горутин
- `process_resident_memory_bytes` — потребление памяти

### MET-2: Health с метриками

```powershell
# Выполните несколько запросов
1..10 | ForEach-Object {
    curl.exe -s "$env:TUCK_ADDR/v1/secret/test/key$_" -H "X-Tuck-Token: $env:TUCK_TOKEN" | Out-Null
}

# Проверьте что счётчики выросли
(Invoke-RestMethod http://localhost:8200/metrics) -split "`n" | Where-Object { $_ -match "tuck_kv" }
```

---

## 19. Namespaces

Изоляция данных и политик внутри одного инстанса Tuck.

### NS-1: Создание namespace

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/sys/namespaces/team-a" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** HTTP 204.

### NS-2: Работа в namespace

```powershell
# Запись секрета в namespace team-a
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/secret/data" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "X-Tuck-Namespace: team-a" `
    -H "Content-Type: application/json" `
    -d '{"value":"team-a-secret"}'

# Запрос без namespace (root) не видит данные namespace
curl.exe -s "$env:TUCK_ADDR/v1/secret/data" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** данные, записанные в namespace `team-a`, не видны из root namespace.

---

## 20. Dynamic Secrets — Database

Требует: работающая БД (PostgreSQL, MySQL, MongoDB и т.д.).

### Запуск тестовой PostgreSQL (Docker)

```powershell
docker run -d --name pgtest -p 5432:5432 `
    -e POSTGRES_PASSWORD=tuck123 postgres:15
```

### DB-1: Настройка подключения

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/database/config/mydb" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{
      "plugin_name": "postgresql-database-plugin",
      "connection_url": "postgresql://postgres:tuck123@localhost:5432/postgres?sslmode=disable",
      "allowed_roles": ["readonly"]
    }'
```

### DB-2: Создание роли и получение учётных данных

```powershell
# Создать роль
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/database/roles/readonly" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{
      "db_name": "mydb",
      "creation_statements": ["CREATE USER \"{{name}}\" WITH PASSWORD '\''{{password}}'\'' VALID UNTIL '\''{{expiration}}'\'';"],
      "default_ttl": "1h",
      "max_ttl": "24h"
    }'

# Получить динамические учётные данные
$creds = Invoke-RestMethod "$env:TUCK_ADDR/v1/database/creds/readonly" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
Write-Host "username: $($creds.username)"
Write-Host "password: $($creds.password)"

# Проверьте что пользователь создан в PostgreSQL
psql -h localhost -U "$($creds.username)" -W -c "\du" postgres
```

**Ожидается:** временный пользователь с уникальным именем и паролем. После истечения TTL — автоматически удаляется.

---

## Полезные команды для отладки

```powershell
# Посмотреть все смонтированные движки
Invoke-RestMethod "$env:TUCK_ADDR/v1/sys/mounts" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}

# Посмотреть активные методы аутентификации
Invoke-RestMethod "$env:TUCK_ADDR/v1/sys/auth" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}

# Проверить возможности токена на пути
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/capabilities-self" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"paths":["secret/myapp/db"]}'

# Листинг всех политик
Invoke-RestMethod "$env:TUCK_ADDR/v1/policy/?list=true" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}

# Аудит лог (если настроен)
Invoke-RestMethod "$env:TUCK_ADDR/v1/sys/audit" `
    -Headers @{"X-Tuck-Token"=$env:TUCK_TOKEN}
```

---

## Матрица покрытия тест-кейсов

| Модуль | Ручные тест-кейсы | Авто-тесты | Требует внешний сервис |
|--------|:-----------------:|:----------:|:----------------------:|
| CORE | CORE-1…5 | ✅ sys_test.go | — |
| KV v1 | KV1-1…5 | ✅ server_test.go, integration | — |
| KV v2 | KV2-1…6 | ✅ integration_e2e_test.go | — |
| Cubbyhole | CUBB-1…3 | ✅ integration_e2e_test.go | — |
| Wrapping | WRAP-1…4 | ✅ integration_e2e_test.go | — |
| Tokens | TOK-1…6 | ✅ integration_e2e_test.go | — |
| Policies | POL-1…4 | ✅ server_test.go | — |
| AppRole | AR-1…5 | ✅ integration_e2e_test.go | — |
| JWT/OIDC | JWT-1…3 | ✅ jwt_test.go (10 тестов) | OIDC-провайдер |
| LDAP | LDAP-1…3 | ✅ ldap_test.go (9 тестов) | LDAP-сервер |
| Kubernetes | K8S-1…2 | ✅ k8s_test.go (8 тестов) | K8s-кластер |
| Transit | TR-1…8 | ✅ transit_test.go (9 тестов) | — |
| PKI | PKI-1…5 | ✅ integration_e2e_test.go | — |
| SSH | SSH-1…4 | ✅ ssh_test.go (11 тестов) | ssh-keygen |
| TOTP | TOTP-1…4 | ✅ totp_test.go (9 тестов) | — |
| Shamir Seal | SH-1…3 | ✅ sys_test.go | — |
| Backup/Restore | BAK-1…2 | ✅ integration_e2e_test.go | — |
| Metrics | MET-1…2 | ✅ bench_test.go | — |
| Namespaces | NS-1…2 | ✅ integration | — |
| Dynamic DB | DB-1…2 | ⚠️ unit (нет e2e) | PostgreSQL/MySQL |
| GitHub auth | — | ⚠️ только негативные | GitHub Actions |
| Dynamic AWS | — | ⚠️ unit | AWS |
| Dynamic GCP | — | ⚠️ unit | GCP |
| Dynamic Azure | — | ⚠️ unit | Azure |

---

## Результаты сквозного CLI-прогона (2026-07-26)

Полный проход разделов 1–20 живьём на пересобранном с нуля minikube-кластере (Helm chart, свежие
образы server/operator/csi), поверх реального Postgres-контейнера для §20. JWT/LDAP оставлены вне
объёма (нужен внешний OIDC/LDAP-сервер — см. матрицу выше), AWS/GCP/Azure dynamic secrets тоже
(нет реальных облачных аккаунтов). Все разделы пройдены без критичных находок — сессия в основном
подтверждала уже исправленные ранее баги (см. `docs/UI_MANUAL_TESTS.md`), а не искала новые.
Найденные UX/документационные несоответствия:

1. **[Средне, UX] Policies**: `rules`-объект вместо массива (`{"paths":[...]}` вместо `[{"path":...}]`)
   даёт `{"error":"invalid JSON"}` (400) — синтаксически валидный JSON, просто неверная схема.
   Сообщение вводит в заблуждение (звучит как ошибка парсинга). См. находку 2 ниже — общая причина.
2. **[Средне, системный паттерн, частично исправлено] Вводящие в заблуждение сообщения об ошибках
   при type mismatch.** Найдено дважды в разных движках: (а) Policies PUT см. выше (не исправлено —
   ниже по приоритету, т.к. полная схема правил сложнее одного поля); (б) `POST /v1/ssh/sign/{role}`
   — если `valid_principals` прислан строкой вместо массива, `json.Unmarshal` падал, но
   `internal/api/ssh.go:176` трактовал любую ошибку Unmarshal как «`public_key` required», уводя
   отладку не в ту сторону. Причина — паттерн
   `if err := json.Unmarshal(body, &req); err != nil || req.Field == "" { return "Field required" }`,
   объединяющий синтаксическую/типовую ошибку JSON и пустое поле в одно сообщение. Грепом по
   `internal/api/*.go` найдено **20 вхождений** этого паттерна в 12 файлах (approle.go, cluster.go,
   jwt.go, k8s.go, ldap.go, mounts.go, pki.go, replication.go, ssh.go, tokens.go, totp.go,
   transit.go) — кандидат на отдельный проход по error-handling. **(б) исправлено в этой же сессии**:
   `sshSign` теперь проверяет `json.Unmarshal` отдельно от `PublicKey == ""` и возвращает
   `"invalid JSON: <err>"` с реальной причиной; тест `TestSSHSignInvalidValidPrincipalsType`. Полный
   грепнутый список (12 файлов) остаётся открытым — это отдельный проход по error-handling, не
   разовый фикс.
3. **[UX] Несогласованность формата тела между похожими эндпоинтами**: KV v1/v2 PUT принимают
   **сырую строку** (`--data-raw "text"`), Cubbyhole PUT требует **JSON-объект** и падает с тем же
   `{"error":"invalid JSON"}` на сырой строке. Пользователь, привыкший к KV, естественно попробует
   то же в cubbyhole. Предложение: либо унифицировать, либо явно назвать ожидаемый формат в ошибке.
4. **[Документация, уже исправлено]** Раздел 5 (Response Wrapping) в этом файле полностью не
   соответствовал реальному API (описывал несуществующий `X-Tuck-Wrap-TTL` заголовок на обычном
   `GET /v1/secret/...`). Переписан на реальный `POST /v1/sys/wrapping/{wrap,unwrap,lookup,revoke}`
   (см. `internal/api/wrapping.go`). Сама фича работает корректно, включая одноразовость.
5. **[Заметка, не баг]** KV v2 PUT не использует Vault-style конверт `{"data":{...}}` — тело целиком
   становится значением секрета, как в KV v1. Для пользователей, знакомых с HashiCorp Vault, это
   неожиданно: `{"data":{"user":"a"}}` сохранится буквально как строка, а не распакуется в поля.
6. **[Заметка]** TTL/длительности в «сырых» JSON-ответах API возвращаются как наносекунды int64
   (например `"ttl":3600000000000` для 1 часа), не человекочитаемой строкой — неудобно для прямого
   curl-использования без CLI/Terraform (которые это форматируют).
7. **[Мелочь]** AppRole: `secret-id`-эндпоинт возвращает поле `"id"`, а `login` ожидает `"secret_id"`
   в запросе — разная терминология для одного и того же значения между response/request.
8. **[Мелочь/безопасность, исправлено]** `PUT /v1/database/config/{name}` возвращал `connection_url`
   без редактирования (в открытом виде, с паролем), хотя `GET` того же ресурса уже маскирует его.
   Клиент и так знает значение (сам его отправил), поэтому это не была утечка новой информации, но
   могло засветиться в логах CI/shell history/аудите стороннего инструмента. Исправлено в этой же
   сессии: `putDBConfig` маскирует `connection_url` в ответе так же, как `getDBConfig`. Тест:
   `TestPutDBConfig_RedactsConnectionURL`.
9. **[Подтверждено живьём]** Ротация Postgres dynamic secrets (фикс REVOKE CONNECT + DROP OWNED BY)
   и отзыв lease через специфичный для движка эндпоинт работают корректно на реальном контейнере —
   пользователь создаётся, креды генерируются, пользователь реально исчезает из `pg_user` после revoke.
10. **[Подтверждено живьём, CSI]** Live-refresh секретов через `tuck.io/refresh-interval` (фича,
    добавленная в эту же сессию) работает корректно: значение в смонтированном томе реально
    обновляется на следующем тике (~30с) без пересоздания пода. Первая попытка теста ложно показала
    отсутствие обновления — причина оказалась в устаревшем Docker-образе DaemonSet'а (тот же класс
    проблемы, что и с leader election, см. ниже), не в самой фиче.
11. **[Инфраструктура/DevEx]** При локальной пересборке образов для minikube (operator, csi, вероятно
    server) недостаточно `docker build` + `minikube image load`: (а) `COPY <binary> /app` в
    Dockerfile кеширует по контенту — если сам бинарник не пересобран, слой остаётся старым;
    (б) `minikube image load` не форсит замену тега, если старый образ ещё используется контейнером
    внутри minikube (`docker rmi` без `-f` тихо фейлится, видно только с `-v=3`). Полный рабочий
    цикл: пересобрать бинарник → пересобрать образ → `minikube ssh -- sudo docker rmi -f <image>` →
    `minikube image load` → пересоздать под/под-DaemonSet. Стоит задокументировать в
    `docs/DEVELOPMENT.md` — на эту причину ушло ~20 минут отладки дважды за сессию (operator и csi).
