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
.\tuck.exe server -dev
```

В выводе найдите строку **ROOT TOKEN** — скопируйте значение `tuck_...`.

> В dev-режиме сервер уже распечатан (unsealed), данные в памяти, TLS самоподписанный.

### 3. Настроить CLI

```powershell
$env:TUCK_ADDR  = "https://127.0.0.1:8200"
$env:TUCK_TOKEN = "tuck_..."   # ваш root token
$env:TUCK_INSECURE = "1"       # доверять самоподписанному TLS
```

### 4. Проверить здоровье

```powershell
.\tuckcli.exe status
```

### 5. Записать секрет

```powershell
.\tuckcli.exe kv put myapp/db-password "postgres-secret-xyz"
```

Путь `myapp/db-password` соответствует API `PUT /v1/secret/myapp/db-password`.

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
| `tuckcli status` | `sealed: false`, `initialized: true` |
| `kv get myapp/db-password` | JSON с полем `value` = `postgres-secret-xyz` |
| `kv list myapp` | массив с ключом `db-password` |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| `connection refused` | Сервер не запущен или неверный `TUCK_ADDR` |
| `403 permission denied` | Неверный или просроченный `TUCK_TOKEN` |
| TLS / certificate error | Установите `TUCK_INSECURE=1` или добавьте `-k` в curl |

## Дальше

- Ограничить доступ: [02 — Политики и токены](02-politiki-i-dostup.md)
- Kubernetes: [06 — Helm в K8s](06-kubernetes-helm.md)
