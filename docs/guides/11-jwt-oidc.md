# 11 — JWT / OIDC login

[← Все гайды](README.md)

## Цель

Пользователь логинится в Tuck по JWT от IdP (Keycloak, Azure AD, Google) и получает token с политиками.

## Шаги

### 1. Настроить JWT auth mount

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/jwt/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "oidc_discovery_url": "https://keycloak.example.com/realms/myrealm",
    "bound_issuer": "https://keycloak.example.com/realms/myrealm"
  }'
```

### 2. Создать роль

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/jwt/role/developer" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "policies": ["default"],
    "bound_audiences": ["tuck"],
    "user_claim": "sub",
    "ttl": "1h"
  }'
```

### 3. Логин

```powershell
.\tuckcli.exe auth jwt login --jwt=eyJhbG... --role=developer
```

### 4. Проверить token

```powershell
$env:TUCK_TOKEN = "tuck_..."   # client_token из login
.\tuckcli.exe token lookup-self
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| login с валидным JWT | `client_token` |
| login с чужим issuer | 403 |
| lookup-self | policies из роли |

## Дальше

- [12 — LDAP](12-ldap.md)
- [02 — Политики](02-politiki-i-dostup.md)
