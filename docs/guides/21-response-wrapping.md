# 21 — Response Wrapping (одноразовая передача)

[← Все гайды](README.md)

## Цель

Безопасно передать секрет или token другой стороне через **одноразовый wrapping token**.

> В PowerShell `curl` это алиас `Invoke-WebRequest`, поэтому везде `curl.exe`. JSON-тело кладём в файл: PS 5.1 съедает двойные кавычки в аргументах native-программ, а pipe может дописать BOM. `-k` нужен для самоподписанного dev-сертификата (`-tls-auto`).

## Шаги

### 1. Обернуть payload

```powershell
[IO.File]::WriteAllText("$PWD\wrap.json", '{"ttl":"5m","data":{"password":"super-secret","user":"deploy"}}')
curl.exe -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/wrap" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@wrap.json"
```

Ответ: `token` (например `tuck_wrap_...`) и `expires_at`.

### 2. Передать wrapping token получателю

По защищённому каналу (не вместе с root token).

### 3. Unwrap (получатель)

Нужен любой валидный Tuck token **и** wrapping token в теле:

```powershell
[IO.File]::WriteAllText("$PWD\unwrap.json", '{"token":"tuck_wrap_..."}')
curl.exe -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/unwrap" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@unwrap.json"
```

Ответ: `{"data":{"password":"super-secret","user":"deploy"}}`.

### 4. Повторный unwrap

Второй вызов с тем же wrapping token — `404 wrapping token not found or already used`.

### 5. (Опционально) Проверить TTL до unwrap

```powershell
curl.exe -k -X POST "$env:TUCK_ADDR/v1/sys/wrapping/lookup" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@unwrap.json"
```

Ответ: `creation_time`, `expires_at`, `creation_ttl` (секунды).

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| wrap | `token` + `expires_at` |
| unwrap | `data` с исходным payload |
| повторный unwrap | 404 |

### 6. Список живых капсул (без токенов)

`GET /v1/sys/wrapping/list` возвращает метаданные живых wrapped-ответов: `accessor`
(НЕ unwrap-токен — только отзыв), время создания/истечения, TTL, размер payload,
namespace, личность создателя (accessor/отпечаток/имя токена) и `creation_path`,
если он был указан при `wrap` (`creation_path` в теле запроса). Токен и payload
никогда не возвращаются. Обычный caller видит только свои капсулы (`scope: "own"`);
caller с правом `list` на `sys/wrapping/list` (root) видит все (`scope: "all"`),
в пределах текущего namespace.

```powershell
curl.exe -k "$env:TUCK_ADDR/v1/sys/wrapping/list" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

### 7. Отзыв по accessor

Отзывает капсулу без необходимости знать wrapping token (список её не содержит):

```powershell
curl.exe -k -X DELETE "$env:TUCK_ADDR/v1/sys/wrapping/revoke-accessor/$accessor" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

Чужая капсула или несуществующий accessor — `404` (как и для несуществующего
accessor), чтобы не раскрывать существование чужих капсул.

### 8. Счётчики

`GET /v1/sys/wrapping/stats` (нужен `read` на `sys/wrapping/stats`) — счётчики за
всё время работы сервера (переживают перезапуск): `wrapped`, `unwrapped`,
`expired`, `revoked`, текущее число живых капсул `active`, `since`.

## Типичные сценарии

- Передача credentials при onboarding нового admin
- CI получает secret без доступа к полному хранилищу
- Zero-trust handoff между командами

## Дальше

- [22 — Cubbyhole](22-cubbyhole.md)
- [10 — AppRole](10-approle.md)
