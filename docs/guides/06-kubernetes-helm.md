# 06 — Установка Tuck в Kubernetes (Helm)

[← Все гайды](README.md)

## Цель

Развернуть Tuck server, operator и (опционально) webhook injector в кластере одной командой Helm.

## Предварительные условия

- Kubernetes 1.24+
- `helm` 3.x
- `kubectl` с доступом к кластеру

## Шаги

### 1. Добавить chart (локально из репо)

```powershell
cd D:\Projects\tuck
helm install tuck deploy/helm/tuck -n tuck --create-namespace
```

### 2. Дождаться готовности

```powershell
kubectl -n tuck rollout status deployment/tuck-server
kubectl -n tuck get pods
```

### 3. Получить root token (dev seal)

```powershell
kubectl -n tuck logs deployment/tuck-server | Select-String "ROOT TOKEN" -Context 0,1
```

Токен печатается один раз, при первом старте, на строке после заголовка. Для `sealType: shamir` в том же логе будут доли (`SHAMIR SHARES`); после каждого рестарта пода нужны `k` долей через `tuckcli unseal`, см. [04 — Shamir](04-prod-shamir.md).

### 4. Port-forward для доступа с хоста

```powershell
kubectl -n tuck port-forward svc/tuck-server 8200:8200
```

```powershell
$env:TUCK_ADDR = "http://127.0.0.1:8200"
$env:TUCK_TOKEN = "tuck_..."
.\tuckcli.exe status
```

### 5. (Опционально) Включить operator и injector

В `values.yaml` или через `--set`:

```yaml
operator:
  enabled: true
injector:
  enabled: true
```

```powershell
helm upgrade tuck deploy/helm/tuck -n tuck --set operator.enabled=true --set injector.enabled=true
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `kubectl get pods -n tuck` | tuck-server Running |
| `curl.exe -s http://127.0.0.1:8200/v1/health` | `"sealed": false` |
| `helm list -n tuck` | release `tuck` deployed |

## Minikube на Windows

Пошагово с локальными образами: раздел 6 в [00 — Установка и запуск](00-setup-guide-ru.md).

## Дальше

- [07 — TuckSecret](07-tucksecret-operator.md)
- [08 — Webhook injector](08-webhook-injector.md)
