# 07 — TuckSecret: синхронизация в K8s Secret

[← Все гайды](README.md)

## Цель

Объявить CRD `TuckSecret` — оператор периодически читает секрет из Tuck и создаёт/обновляет обычный Kubernetes `Secret`.

## Кому подходит

Platform engineer, DevOps (сценарий «секрет в etcd K8s», знакомый по External Secrets).

## Предварительные условия

- Tuck server в кластере ([06 — Helm](06-kubernetes-helm.md))
- Root token (для первой настройки)
- Оператор задеплоен

## Шаги

### 1. Установить CRD и оператор

```powershell
kubectl apply -f deploy/crd/
kubectl apply -f deploy/operator/deployment.yaml
kubectl apply -f deploy/operator/local.yaml   # dev: образ tuck-operator:local
```

Полный E2E: [docs/E2E-DEMO.md](../../docs/E2E-DEMO.md).

### 2. Выдать оператору доступ к Tuck (K8s auth)

```powershell
curl -X PUT "$env:TUCK_ADDR/v1/auth/kubernetes/role/tuck/tuck-operator" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["root"],"ttl":"5m"}'
```

В продакшне замените `root` на узкую политику с `read` на нужные пути.

### 3. Записать секрет в Tuck

```powershell
.\tuckcli.exe kv put myapp/db-password "postgres-secret-xyz"
```

### 4. Создать TuckSecret

Файл `deploy/examples/test-tucksecret.yaml`:

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: default
spec:
  tuckPath: myapp/db-password      # путь в KV (без префикса secret/)
  secretName: myapp-db-credentials # имя K8s Secret
  secretKey: password              # ключ внутри Secret
  refreshInterval: "30s"
```

```powershell
kubectl apply -f deploy/examples/test-tucksecret.yaml
```

### 5. Дождаться синхронизации

```powershell
kubectl get tucksecret db-password
kubectl get secret myapp-db-credentials
```

Обычно 30–60 секунд (`refreshInterval`).

### 6. Прочитать значение из K8s Secret

```powershell
kubectl get secret myapp-db-credentials -o jsonpath='{.data.password}' | `
  ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($_)) }
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `kubectl get tucksecret` | STATUS Synced / Ready |
| `kubectl get secret myapp-db-credentials` | Secret существует |
| jsonpath password | `postgres-secret-xyz` |
| Обновить значение в Tuck | K8s Secret обновится после refresh |

## Поля spec

| Поле | Описание |
|------|----------|
| `tuckPath` | Путь в Tuck KV (mount `secret/`) |
| `secretName` | Имя целевого K8s Secret |
| `secretKey` | Ключ в `data` Secret |
| `refreshInterval` | Интервал опроса (например `30s`, `5m`) |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| TuckSecret Pending | Оператор не запущен или нет K8s auth role |
| Secret пустой | Неверный `tuckPath` — проверьте `kv get` |
| 403 в логах оператора | Политика или role для SA оператора |

## Дальше

- Без etcd: [08 — Webhook injector](08-webhook-injector.md)
- SA auth для приложений: [09 — Kubernetes auth](09-kubernetes-auth.md)
