# 01 — Первый секрет (локальный dev)

[← Все гайды](README.md)

## Цель

За 5 минут запустить Tuck в dev-режиме, записать секрет и прочитать его обратно.

## Кому подходит

Разработчик, который впервые знакомится с Tuck.

## Предварительные условия

- Go 1.22+ или готовый бинарь `tuck` / `tuckcli`
- Windows / Linux / macOS

## Шаги

### 1. Собрать или скачать бинарии

```powershell
cd D:\Projects\tuck
go build -o tuck.exe ./cmd/tuck
go build -o tuckcli.exe ./cmd/tuckcli
```

### 2. Запустить сервер в dev-режиме

```powershell
.\tuck.exe -tls-auto
```

У `tuck` нет подкоманд и флага `-dev`: dev-режим это значения по умолчанию (`-seal-type dev`) плюс `-tls-auto` для самоподписанного HTTPS. Адрес по умолчанию `127.0.0.1:8200`.

При **первом** старте в логе печатается блок `ROOT TOKEN (shown once ...)`, на следующей строке значение `tuck_...`. Скопируйте его: повторно токен не показывается.

> Dev-режим: сервер распечатывается сам, ключ лежит открытым текстом в файле `tuck-dev-rootkey`, данные хранятся в файле `tuck.db` в текущей папке (переживают перезапуск; для чистого старта удалите оба файла). TLS самоподписанный. Для продакшна см. [04](04-prod-shamir.md).

### 3. Настроить CLI

```powershell
$env:TUCK_ADDR  = "https://127.0.0.1:8200"
$env:TUCK_TOKEN = "tuck_..."   # ваш root token
$env:TUCK_INSECURE = "1"       # доверять самоподписанному TLS (то же, что флаг --insecure)
```

### 4. Проверить здоровье

```powershell
.\tuckcli.exe status
```

### 5. Записать секрет

```powershell
.\tuckcli.exe kv put myapp/db-password "postgres-secret-xyz"
```

Путь в CLI указывается относительно монтирования `secret/`: `myapp/db-password` это API `PUT /v1/secret/myapp/db-password`. Префикс `secret/` в командах `kv` писать не нужно (в политиках, наоборот, нужен).

### 6. Прочитать секрет

```powershell
.\tuckcli.exe kv get myapp/db-password
```

### 7. Список секретов

```powershell
.\tuckcli.exe kv list myapp
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `tuckcli status` | `"sealed": false`, `"type": "dev"` |
| `kv get myapp/db-password` | JSON с полем `value` = `postgres-secret-xyz` |
| `kv list myapp` | массив с ключом `db-password` |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| `connection refused` | Сервер не запущен или неверный `TUCK_ADDR` (схема `https://`, раз запущен с `-tls-auto`) |
| `address already in use` | Порт 8200 занят: запустите с `-addr 127.0.0.1:8201` и поправьте `TUCK_ADDR` |
| `403 permission denied` | Неверный или просроченный `TUCK_TOKEN` |
| TLS / certificate error | Установите `TUCK_INSECURE=1` (или `--insecure`), в curl добавьте `-k` |

## Дальше

- Ограничить доступ: [02 — Политики и токены](02-politiki-i-dostup.md)
- Kubernetes: [06 — Helm в K8s](06-kubernetes-helm.md)
