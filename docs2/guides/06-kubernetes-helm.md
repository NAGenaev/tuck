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
kubectl -n tuck logs deployment/tuck-server | Select-String "ROOT TOKEN"
```

Для `sealType: shamir` — выполните init/unseal по [04 — Shamir](04-prod-shamir.md).

### 4. Port-forward для доступа с хоста

```powershell
kubectl -n tuck port-forward svc/tuck 8200:8200
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
| `curl .../v1/health` | `{"initialized":true,"sealed":false}` |
| `helm list -n tuck` | release `tuck` deployed |

## Minikube на Windows

Пошагово с локальными образами: [docs/MINIKUBE.md](../../docs/MINIKUBE.md).

## Дальше

- [07 — TuckSecret](07-tucksecret-operator.md)
- [08 — Webhook injector](08-webhook-injector.md)
