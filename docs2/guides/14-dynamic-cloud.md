# 14 — Dynamic cloud credentials (AWS / GCP / Azure)

[← Все гайды](README.md)

## Цель

Выдать приложению временные облачные ключи (STS, GCP SA key, Azure SP) вместо долгоживущих IAM credentials.

---

## AWS

### Настройка

```powershell
curl -X POST "$env:TUCK_ADDR/v1/aws/config/root" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "access_key": "AKIA...",
    "secret_key": "...",
    "region": "eu-central-1"
  }'
```

```powershell
curl -X POST "$env:TUCK_ADDR/v1/aws/roles/s3-reader" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "credential_type": "iam_user",
    "policy_arns": ["arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess"],
    "default_sts_ttl": "1h"
  }'
```

### Получить креды

```powershell
.\tuckcli.exe aws creds s3-reader
```

---

## GCP

```powershell
curl -X POST "$env:TUCK_ADDR/v1/gcp/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"credentials": "..."}'
```

```powershell
.\tuckcli.exe gcp creds my-role
```

---

## Azure

```powershell
curl -X POST "$env:TUCK_ADDR/v1/azure/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"tenant_id":"...","client_id":"...","client_secret":"..."}'
```

```powershell
.\tuckcli.exe azure creds my-role
```

---

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `aws creds ROLE` | access_key, secret_key, security_token |
| Доступ к ресурсу | в рамках policy role |
| Истёк TTL | AccessDenied |

## Дальше

- [13 — Database](13-dynamic-database.md)
- [02 — Политики](02-politiki-i-dostup.md)
