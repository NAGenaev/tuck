# 09 — Kubernetes ServiceAccount auth

[← Все гайды](README.md)

## Цель

Приложение в Pod логинится в Tuck по JWT своего ServiceAccount и получает короткоживущий token с нужными политиками.

## Предварительные условия

- Tuck в кластере, настроен auth mount `kubernetes`
- Root token для настройки ролей

## Шаги

### 1. Создать ServiceAccount

```powershell
kubectl create serviceaccount my-app -n default
```

### 2. Политика для приложения

```powershell
$rules = '[{"path":"secret/app/*","capabilities":["read"]}]'
.\tuckcli.exe policy put my-app-policy $rules
```

### 3. Роль K8s auth

Формат пути: `auth/kubernetes/role/{namespace}/{sa-name}`

```powershell
curl -X PUT "$env:TUCK_ADDR/v1/auth/kubernetes/role/default/my-app" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["my-app-policy"],"ttl":"1h"}'
```

### 4. Логин с хоста (тест)

```powershell
$SA_TOKEN = kubectl create token my-app -n default --duration=1h

curl -X POST "$env:TUCK_ADDR/v1/auth/kubernetes/login" `
  -H "Content-Type: application/json" `
  -d "{`"token`":`"$SA_TOKEN`"}"
```

В ответе — `auth.client_token` (scoped token).

### 5. Использовать client token

```powershell
$APP_TOKEN = "tuck_..."   # из login
curl "$env:TUCK_ADDR/v1/secret/app/config" -H "X-Tuck-Token: $APP_TOKEN"
```

### 6. В Pod (типичный паттерн)

Sidecar или init-скрипт:

1. Читает SA token из `/var/run/secrets/kubernetes.io/serviceaccount/token`
2. `POST /v1/auth/kubernetes/login`
3. Использует `client_token` для API

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| login с SA JWT | `client_token` + `policies: ["my-app-policy"]` |
| read `secret/app/*` | 200 |
| read `secret/admin/*` | 403 |

## Дальше

- [08 — Injector](08-webhook-injector.md) + scoped token
- [02 — Политики](02-politiki-i-dostup.md)
