# 12 — LDAP / Active Directory

[← Все гайды](README.md)

## Цель

Сотрудники логинятся логином/паролем домена LDAP или AD; Tuck выдаёт token с политиками по группам.

## Шаги

### 1. Настроить LDAP mount

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/ldap/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "url": "ldap://ldap.example.com",
    "binddn": "cn=tuck,ou=services,dc=example,dc=com",
    "bindpass": "bind-password",
    "userdn": "ou=users,dc=example,dc=com",
    "userattr": "uid",
    "groupdn": "ou=groups,dc=example,dc=com",
    "groupattr": "cn"
  }'
```

### 2. Маппинг групп на политики

```powershell
curl -X POST "$env:TUCK_ADDR/v1/auth/ldap/groups/devops" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["devops-full"]}'
```

### 3. Логин пользователя

```powershell
.\tuckcli.exe auth ldap login --username=alice --password=...
```

### 4. Проверка

```powershell
.\tuckcli.exe token lookup-self
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| ldap login alice | client_token |
| alice в группе devops | policies содержит devops-full |
| неверный пароль | 400 / invalid credentials |

## Дальше

- [11 — JWT](11-jwt-oidc.md)
- [02 — Политики](02-politiki-i-dostup.md)
