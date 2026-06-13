# 18 — TOTP (двухфакторная аутентификация)

[← Все гайды](README.md)

## Цель

Сгенерировать и проверять TOTP-коды (Google Authenticator и аналоги) через Tuck.

## Шаги

### 1. Создать ключ TOTP

```powershell
curl -X POST "$env:TUCK_ADDR/v1/totp/keys/alice" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "issuer": "MyApp",
    "account_name": "alice@example.com",
    "generate": true
  }'
```

В ответе — `barcode` / `url` для QR в приложении-аутентификаторе.

### 2. Получить текущий код (тест)

```powershell
.\tuckcli.exe totp code alice
```

### 3. Валидация кода (API)

```powershell
curl -X POST "$env:TUCK_ADDR/v1/totp/code/alice" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"code": "123456"}'
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| totp code | 6 цифр, меняется каждые 30с |
| validate правильный code | valid: true |
| validate старый code | valid: false |

## Дальше

- [11 — JWT](11-jwt-oidc.md)
- [12 — LDAP](12-ldap.md)
