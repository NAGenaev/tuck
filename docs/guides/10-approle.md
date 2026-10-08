# 10 — AppRole (CI/CD, machine-to-machine)

[← Все гайды](README.md)

## Цель

Выдать CI/CD pipeline пару `role_id` + `secret_id` для получения короткоживущего token без человеческого логина.

> У AppRole нет команд в `tuckcli`, роль создаётся через HTTP API. В PowerShell `curl` это алиас `Invoke-WebRequest`, поэтому ниже `curl.exe`. JSON-тело кладём в файл: PS 5.1 съедает двойные кавычки в аргументах native-программ, а pipe может дописать BOM. Для самоподписанного TLS `-k` (dev-режим `-tls-auto`).

## Шаги

### 1. Политика

```powershell
'[{"path":"secret/ci/*","capabilities":["read"]}]' | .\tuckcli.exe policy put ci-deploy -
```

### 2. Создать AppRole

Создание роли это `PUT`:

```powershell
[IO.File]::WriteAllText("$PWD\body.json", '{"policies":["ci-deploy"],"token_ttl":"15m","secret_id_ttl":"24h"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/auth/approle/role/deploy" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@body.json"
```

В ответе `role_id` (его же вернёт `GET /v1/auth/approle/role/deploy`). Отдельного endpoint'а `role-id` нет.

Поля роли: `policies`, `token_ttl`, `secret_id_ttl`, `secret_id_num_uses`, `bound_cidrs`, `metadata`, `tuck_namespace`.

### 3. Выпустить secret_id

```powershell
curl.exe -k -X POST "$env:TUCK_ADDR/v1/auth/approle/role/deploy/secret-id" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

Ответ: `{"id":"...","role_name":"deploy","created_at":...,"expires_at":...}`. Значение `id` это и есть `secret_id`; сохраните его в секретах CI (GitHub Actions, GitLab CI). Показывается один раз.

### 4. Логин из pipeline

```powershell
$env:TUCK_INSECURE = "1"   # только для самоподписанного dev-сертификата
.\tuckcli.exe auth approle login --role-id=ROLE_ID --secret-id=SECRET_ID
```

Или curl:

```powershell
[IO.File]::WriteAllText("$PWD\login.json", '{"role_id":"ROLE_ID","secret_id":"SECRET_ID"}')
curl.exe -k -X POST "$env:TUCK_ADDR/v1/auth/approle/login" `
  -H "Content-Type: application/json" `
  --data-binary "@login.json"
```

Ответ: `{"token":"tuck_...","policies":["ci-deploy"],"expires_at":"..."}`.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| approle login | `token` с `policies: ["ci-deploy"]` |
| read `secret/ci/...` | успех |
| повторный secret_id | старый можно отозвать |

## Безопасность

- `secret_id` — как пароль: ротация, минимальный TTL.
- `role_id` можно считать публичным идентификатором.
- В CI храните только secret_id в protected variables.

## Дальше

- [02 — Политики](02-politiki-i-dostup.md)
- [11 — JWT](11-jwt-oidc.md) для human SSO
