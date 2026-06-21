# 13 — Dynamic database credentials

[← Все гайды](README.md)

## Цель

Приложение получает **временные** логин/пароль PostgreSQL (или MySQL и др.) — ротация без смены секрета вручную.

## Шаги

### 1. Включить database engine

```powershell
curl -X POST "$env:TUCK_ADDR/v1/database/config/postgres" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "plugin_name": "postgresql",
    "connection_url": "postgresql://{{username}}:{{password}}@db.example.com:5432/mydb?sslmode=require",
    "allowed_roles": ["readonly","readwrite"],
    "username": "tuck_admin",
    "password": "admin-pass"
  }'
```

### 2. Создать роль (SQL шаблон)

```powershell
curl -X POST "$env:TUCK_ADDR/v1/database/roles/readonly" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "db_name": "postgres",
    "creation_statements": [
      "CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '\''{{password}}'\'' VALID UNTIL '\''{{expiration}}'\'';",
      "GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";"
    ],
    "default_ttl": "1h",
    "max_ttl": "24h"
  }'
```

### 3. Получить креды

```powershell
.\tuckcli.exe db creds readonly
```

Или API:

```powershell
curl "$env:TUCK_ADDR/v1/database/creds/readonly" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

### 4. Подключиться к БД

Используйте `username` и `password` из ответа — они перестанут работать после TTL.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `db creds readonly` | username, password, lease_duration |
| SELECT в БД | успех в рамках grants |
| после TTL | login failed |

## Политика для приложения

```json
[{"path": "database/creds/readonly", "capabilities": ["read"]}]
```

## Дальше

- [14 — Cloud dynamic](14-dynamic-cloud.md)
- [09 — K8s auth](09-kubernetes-auth.md)
