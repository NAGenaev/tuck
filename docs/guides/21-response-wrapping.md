# 21 — Response Wrapping (одноразовая передача)

[← Все гайды](README.md)

## Цель

Безопасно передать секрет или token другой стороне через **одноразовый wrapping token**.

## Шаги

### 1. Обернуть payload

```powershell
curl -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/wrap" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "ttl": "5m",
    "data": {"password": "super-secret", "user": "deploy"}
  }'
```

Ответ: `token` (например `tuck_wrap_...`) и `expires_at`.

### 2. Передать wrapping token получателю

По защищённому каналу (не вместе с root token).

### 3. Unwrap (получатель)

Нужен любой валидный Tuck token **и** wrapping token в теле:

```powershell
curl -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/unwrap" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"token": "tuck_wrap_..."}'
```

### 4. Повторный unwrap

Второй вызов с тем же wrapping token — `404 not found or already used`.

### 5. (Опционально) Проверить TTL до unwrap

```powershell
curl -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/lookup" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"token": "tuck_wrap_..."}'
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| wrap | `token` + `expires_at` |
| unwrap | `data` с исходным payload |
| повторный unwrap | 404 |

## Типичные сценарии

- Передача credentials при onboarding нового admin
- CI получает secret без доступа к полному хранилищу
- Zero-trust handoff между командами

## Дальше

- [22 — Cubbyhole](22-cubbyhole.md)
- [10 — AppRole](10-approle.md)
