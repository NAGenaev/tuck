# 22 — Cubbyhole (приватное хранилище токена)

[← Все гайды](README.md)

## Цель

Сохранить секрет, доступный **только** тому token, который его создал — исчезает при revoke token.

## Шаги

### 1. Создать ограниченный token

```powershell
.\tuckcli.exe token create --policy=default --ttl=10m --name=handoff
```

### 2. Записать в cubbyhole этим token

```powershell
$env:TUCK_TOKEN = "tuck_..."   # handoff token
curl -k -X POST "$env:TUCK_ADDR/v1/cubbyhole/response" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"secret":"one-time-value"}'
```

### 3. Прочитать тем же token

```powershell
curl -k "$env:TUCK_ADDR/v1/cubbyhole/response" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

### 4. Попытка другим token

```powershell
$env:TUCK_TOKEN = "tuck_..."   # root или другой
curl -k "$env:TUCK_ADDR/v1/cubbyhole/response" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

→ 403.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| read своим token | данные |
| read чужим token | 403 |
| revoke token | данные недоступны |

## Дальше

- [21 — Wrapping](21-response-wrapping.md)
- [02 — Политики](02-politiki-i-dostup.md)
