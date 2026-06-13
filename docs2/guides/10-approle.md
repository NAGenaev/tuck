# 10 — AppRole (CI/CD, machine-to-machine)

[← Все гайды](README.md)

## Цель

Выдать CI/CD pipeline пару `role_id` + `secret_id` для получения короткоживущего token без человеческого логина.

## Шаги

### 1. Политика

```powershell
$rules = '[{"path":"secret/ci/*","capabilities":["read"]}]'
.\tuckcli.exe policy put ci-deploy $rules
```

### 2. Создать AppRole

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/approle/role/deploy" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["ci-deploy"],"token_ttl":"15m","secret_id_ttl":"24h"}'
```

### 3. Получить role_id

```powershell
curl "$env:TUCK_ADDR/v1/auth/approle/role/deploy/role-id" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

### 4. Выпустить secret_id

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/approle/role/deploy/secret-id" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

Сохраните `secret_id` в секретах CI (GitHub Actions, GitLab CI).

### 5. Логин из pipeline

```powershell
.\tuckcli.exe auth approle login --role-id=ROLE_ID --secret-id=SECRET_ID
```

Или curl:

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/approle/login" `
  -H "Content-Type: application/json" `
  -d '{"role_id":"...","secret_id":"..."}'
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| approle login | `client_token` с policy `ci-deploy` |
| read `secret/ci/...` | успех |
| повторный secret_id | старый можно отозвать |

## Безопасность

- `secret_id` — как пароль: ротация, минимальный TTL.
- `role_id` можно считать публичным идентификатором.
- В CI храните только secret_id в protected variables.

## Дальше

- [02 — Политики](02-politiki-i-dostup.md)
- [11 — JWT](11-jwt-oidc.md) для human SSO
