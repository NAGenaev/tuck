# 16 — Transit (шифрование as a service)

[← Все гайды](README.md)

## Цель

Шифровать и расшифровывать данные в приложении, не экспортируя мастер-ключ из Tuck.

## Шаги

### 1. Создать ключ

```powershell
curl -X POST "$env:TUCK_ADDR/v1/transit/keys/app-data" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"type":"aes256-gcm96"}'
```

### 2. Зашифровать

```powershell
.\tuckcli.exe transit encrypt app-data "hello world"
```

Ответ содержит `ciphertext` (например `tuck:v1:...`).

### 3. Расшифровать

```powershell
.\tuckcli.exe transit decrypt app-data "tuck:v1:..."
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| encrypt | ciphertext, не plaintext |
| decrypt | исходный текст |
| rotate key | старые ciphertext всё ещё decrypt |

## Политики

```json
[
  {"path": "transit/encrypt/app-data", "capabilities": ["update"]},
  {"path": "transit/decrypt/app-data", "capabilities": ["update"]}
]
```

## Дальше

- [05 — Auto-unseal Transit](05-auto-unseal-kms.md)
- [21 — Wrapping](21-response-wrapping.md)
