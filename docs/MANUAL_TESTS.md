# Tuck — Руководство по ручному тестированию

> Версия: v1.36.0+ · Обновлено: 2026-10-05  
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
| 12 | [SHAMIR — Ручное распечатывание](#12-shamir-seal) | SH-1…SH-3 |
| 13 | [BACKUP — Backup и Restore](#13-backup--restore) | BAK-1…BAK-2 |
| 14 | [METRICS — Prometheus-метрики](#14-metrics) | MET-1…MET-2 |
| 15 | [NAMESPACE — Изоляция пространств](#15-namespaces) | NS-1…NS-2 |
| 16 | [MULTI-CLUSTER — Несколько кластеров Kubernetes](#16-несколько-кластеров-kubernetes) | MC-1…MC-2 |
| 17 | [NS-BINDING — Привязка токена к namespace](#17-привязка-токена-к-namespace) | NSB-1…NSB-2 |
| 18 | [BARRIER — Шифр, ротация DEK, шифрование путей](#18-шифр-барьера-ротация-dek-шифрование-путей) | BR-1…BR-3 |
| 19 | [TLS — Горячая перезагрузка сертификата](#19-tls-горячая-перезагрузка-сертификата) | — |
| 20 | [RAFT mTLS — Защита транспорта Raft](#20-raft-mutual-tls-между-узлами) | — |
| 21 | [MLOCK — Защита памяти](#21-защита-памяти) | — |

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

## 12. Shamir Seal

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

## 13. Backup / Restore

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

## 14. Metrics

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

## 15. Namespaces

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

## 16. Несколько кластеров Kubernetes

Требует: два кластера (или один кластер, зарегистрированный под двумя именами) и ServiceAccount-токен рецензента для каждого.

### MC-1: Конфигурация кластеров

Сервер запускается с конфигом:

```yaml
kubernetes:
  api: https://k8s-a.example:6443        # кластер по умолчанию
  token_file: /etc/tuck/k8s-a.token
  ca_file: /etc/tuck/k8s-a.ca
  clusters:
    - {name: seg-b, api: https://k8s-b.example:6443, token_file: /etc/tuck/k8s-b.token, ca_file: /etc/tuck/k8s-b.ca}
```

```powershell
curl.exe -s "$env:TUCK_ADDR/v1/auth/kubernetes/clusters" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** в списке кластер по умолчанию и `seg-b`.

### MC-2: Роли и логин по кластеру

```powershell
# Роль в кластере seg-b; такой же SA в кластере по умолчанию доступа не получит
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/auth/kubernetes/clusters/seg-b/role/payments/api" `
    -H "X-Tuck-Token: $env:TUCK_TOKEN" -H "Content-Type: application/json" `
    -d '{"policies":["payments-read"],"ttl":"1h","tuck_namespace":"payments"}'

# Логин из пода кластера seg-b
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/auth/kubernetes/login" -H "Content-Type: application/json" `
    -d "{`"token`":`"$saToken`",`"cluster`":`"seg-b`"}"
```

**Ожидается:** токен с политикой `payments-read`. Тот же JWT с `"cluster":"default"` (или без поля) — 403: роль есть только в `seg-b`. JWT из другого кластера с `"cluster":"seg-b"` — 401/403 (TokenReview не проходит).

---

## 17. Привязка токена к namespace

### NSB-1: Токен не выходит за свой namespace

```powershell
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/sys/namespaces/team-a" -H "X-Tuck-Token: $env:TUCK_TOKEN"
curl.exe -s -XPUT "$env:TUCK_ADDR/v1/sys/namespaces/team-b" -H "X-Tuck-Token: $env:TUCK_TOKEN"
# Роль K8s/AppRole с "tuck_namespace":"team-a" (см. §11, §8), логин -> $nsToken
curl.exe -s -o NUL -w "%{http_code}" "$env:TUCK_ADDR/v1/secret/data" -H "X-Tuck-Token: $nsToken"                              # без заголовка
curl.exe -s -o NUL -w "%{http_code}" "$env:TUCK_ADDR/v1/secret/data" -H "X-Tuck-Token: $nsToken" -H "X-Tuck-Namespace: team-b"  # чужой
```

**Ожидается:** первый запрос работает в `team-a` (200/404 по наличию данных), второй — **403**.

### NSB-2: Удалённый namespace

Удалите `team-a` (`DELETE /v1/sys/namespaces/team-a`) и повторите логин по роли с `tuck_namespace":"team-a"`.

**Ожидается:** логин отклонён (403), токен не выдан.

---

## 18. Шифр барьера, ротация DEK, шифрование путей

### BR-1: Шифр в seal-status

```powershell
(Invoke-RestMethod "$env:TUCK_ADDR/v1/sys/seal-status").barrier_cipher
```

**Ожидается:** `aes-256-gcm` (или `gost-kuznyechik-mgm`, если хранилище инициализировано с `--barrier-cipher=gost-kuznyechik-mgm`). Для существующего хранилища флаг игнорируется.

### BR-2: Ротация DEK

```powershell
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/rotate-data-key" -H "X-Tuck-Token: $env:TUCK_TOKEN"
curl.exe -s -XPOST "$env:TUCK_ADDR/v1/sys/rotate-data-key?reencrypt=true" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

**Ожидается:** `{"ok":true,"key_version":N,"reencrypted":0}` для первого вызова (версия растёт на 1); для второго — `reencrypted` равно числу записей. Все ранее записанные секреты читаются. Токен без `write` на `sys/rotate-data-key` получает 403. Хранилище, созданное до версионирования ключей, — 409.

### BR-3: Шифрование путей

Запишите секрет `secret/very-secret-name`, остановите сервер и выполните:

```powershell
Select-String -Path .\tuck.db -Pattern "very-secret-name" -Encoding ascii
```

**Ожидается:** совпадений нет (имена записей в файле зашифрованы), при этом после запуска и распечатывания `LIST /v1/secret` показывает `very-secret-name`. Для хранилищ с `barrier/meta` версий 1–2 имена видны открытым текстом — это штатно.

---

## 19. TLS: горячая перезагрузка сертификата

```powershell
.\tuck.exe server -tls-cert .\srv.crt -tls-key .\srv.key ...
# в другой консоли: подмените srv.crt/srv.key новыми (другой CN/срок), подождите ~15 с
openssl s_client -connect localhost:8200 </dev/null 2>$null | openssl x509 -noout -serial -subject
```

**Ожидается:** без перезапуска сервер отдаёт новый сертификат (меняется serial). Если подложить битую пару, в логе сообщение об ошибке перезагрузки, а сервер продолжает работать со старым сертификатом.

---

## 20. Raft: mutual TLS между узлами

Требует: три узла и отдельный CA кластера (не публичный).

```powershell
.\tuck.exe server -cluster -cluster-node-id n1 -cluster-bootstrap -cluster-peers "n1=10.0.0.1:8201,n2=10.0.0.2:8201,n3=10.0.0.3:8201" `
    -cluster-tls-cert n1.crt -cluster-tls-key n1.key -cluster-tls-ca cluster-ca.crt
```

**Ожидается:** узлы с сертификатами от `cluster-ca.crt` формируют кластер, `GET /v1/sys/cluster` показывает лидера и 3 голосующих. Узел без сертификата или с сертификатом чужого CA в кластер не входит (в логе ошибка TLS-рукопожатия), порт 8201 не отвечает на обычный TCP-обмен. С `-cluster-tls-server-name` дополнительно проверяется имя в сертификате пира.

---

## 21. Защита памяти

```powershell
.\tuck.exe server -require-mlock ...
```

**Ожидается:** на Linux — запуск проходит, если память можно заблокировать (`ulimit -l`/`CAP_IPC_LOCK`), иначе сервер завершается с ошибкой. Без флага — только предупреждение. На Windows/macOS флаг не поддерживается.

---

## Полезные команды для отладки

```powershell
# Посмотреть все точки монтирования
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
| Shamir Seal | SH-1…3 | ✅ sys_test.go | — |
| Backup/Restore | BAK-1…2 | ✅ integration_e2e_test.go | — |
| Metrics | MET-1…2 | ✅ bench_test.go | — |
| Namespaces | NS-1…2 | ✅ integration | — |
| Multi-cluster K8s | MC-1…2 | ✅ k8s_test.go | 2 кластера K8s |
| Namespace binding | NSB-1…2 | ✅ api-тесты | — |
| Barrier: cipher / DEK / пути | BR-1…3 | ✅ barrier-тесты | — |
| TLS hot-reload | §19 | ✅ tlsutil/reload_test.go | — |
| Raft mTLS | §20 | ✅ raft/tls_test.go | — |

---

## Результаты сквозного CLI-прогона (2026-07-26)

> Исторический отчёт. Движки Transit/PKI/SSH/TOTP/Database и leases впоследствии удалены из Tuck; упоминания о них ниже сохранены как история находок.

Полный проход разделов (на момент прогона, включая ныне удалённые движки) живьём на пересобранном с нуля minikube-кластере (Helm chart, свежие
образы server/operator/csi), JWT/LDAP оставлены вне
объёма (нужен внешний OIDC/LDAP-сервер — см. матрицу выше). Найденные UX/документационные несоответствия ниже — но самая
серьёзная находка всей сессии (см. 0 ниже) обнаружилась не в этом прогоне, а позже, при постройке
реальных юз-кейсов применения (см. `docs/USE_CASES.md`), и уже исправлена.

0. **[КРИТИЧНО, исправлено] Политики (ACL) не проверялись вообще в большинстве движков.**
   Обнаружено при построении честного demo encryption-as-a-service (AppRole + Transit, юз-кейс 5):
   токен с политикой, ограниченной ТОЛЬКО `secret/data/* read/list`, смог расшифровать чужой Transit
   ciphertext, создать новый Transit-ключ, прочитать SQL-выражения из чужой Database-роли, выпустить
   PKI-сертификат по любой роли — и, самое серьёзное, **сгенерировать новый `secret_id` для ЛЮБОЙ
   существующей AppRole-роли по имени** (полная эскалация привилегий до политик этой роли) и
   **создать identity-entity с произвольными `policies` в теле запроса**. Причина: `requireToken`
   (`internal/api/server.go`) проверяет только валидность/срок токена, а не capability — почти
   каждый хендлер в `transit.go`, `pki.go`, `ssh.go`, `totp.go`, `database.go`, облачные движки (позже удалены),
   `namespaces.go`, `cluster.go`, значительная часть `approle.go`/`jwt.go`/`ldap.go`
   (управление ролями/конфигом) и весь `identity.go` (27 хендлеров) вызывали свои core-методы
   напрямую, без единого `s.core.EnforceAccess`. Реально enforced были только `kv.go`, `kvv2.go`,
   `tokens.go`, `policies.go`, `mounts.go`, `plugins.go`, `sysconfig.go`, `replication.go`,
   `github.go`, `k8s.go`. `cubbyhole.go`/`wrapping.go` намеренно НЕ тронуты — оба by design scoped
   на предъявителя (cubbyhole — на сам токен, wrapping — на знание wrap-токена), как и в Vault.
   **Исправлено**: добавлен `EnforceAccess` во все недостающие хендлеры с path/capability по тем же
   конвенциям, что уже использовались в `kv.go`/`kvv2.go`. Заодно исправлен сопутствующий баг:
   `parseCaps` молча превращал нераспознанную строку способности (например `"update"` — так эта же
   способность называется в Vault, откуда естественно взять привычку) в ноль capability без единой
   ошибки — политика тихо не давала прав вообще. Теперь `PUT /v1/policy/{name}` с такой строкой
   возвращает 400 с явным указанием проблемной строки. **Подтверждено вживую**: пересобран и
   передеплоен сервер на реальном кластере, все векторы эскалации выше дают 403 после фикса,
   легитимный least-privilege доступ (AppRole-сервисы, честно ограниченные политикой) продолжает
   работать. Тесты: `acl_enforcement_test.go` (24 сценария по всем движкам),
   `TestPolicyManagement_RejectsUnrecognizedCapability`.

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
