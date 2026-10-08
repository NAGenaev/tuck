# 22 — Cubbyhole (приватное хранилище токена)

[← Все гайды](README.md)

## Цель

Сохранить секрет, доступный **только** тому token, который его создал — исчезает при revoke token.

> В PowerShell `curl` это алиас `Invoke-WebRequest`, поэтому везде `curl.exe`. JSON-тело кладём в файл: PS 5.1 съедает двойные кавычки в аргументах native-программ, а pipe может дописать BOM. `-k` нужен для самоподписанного dev-сертификата (`-tls-auto`).

## Шаги

### 1. Создать ограниченный token

Политики не нужны: к своему cubbyhole у любого токена есть доступ.

```powershell
.\tuckcli.exe token create --ttl=10m --name=handoff
```

Сохраните поле `id` из ответа (`tuck_...`).

### 2. Записать в cubbyhole этим token

Запись это `PUT`, тело любой JSON-объект; успех `204`.

```powershell
$env:TUCK_TOKEN = "tuck_..."   # id токена handoff
[IO.File]::WriteAllText("$PWD\cubby.json", '{"secret":"one-time-value"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/cubbyhole/response" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@cubby.json"
```

### 3. Прочитать тем же token

```powershell
curl.exe -k "$env:TUCK_ADDR/v1/cubbyhole/response" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

Ответ: `{"data":{"secret":"one-time-value"}}`.

### 4. Попытка другим token

```powershell
$env:TUCK_TOKEN = "tuck_..."   # root или другой
curl.exe -k "$env:TUCK_ADDR/v1/cubbyhole/response" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

→ `404 not found`: у каждого токена свой cubbyhole, чужого пути для него просто нет (даже у root).

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| read своим token | данные |
| read чужим token | 404 |
| revoke token | данные недоступны |

## Дальше

- [21 — Wrapping](21-response-wrapping.md)
- [02 — Политики](02-politiki-i-dostup.md)
