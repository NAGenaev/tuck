# 08 — Webhook injector (секреты в tmpfs Pod)

[← Все гайды](README.md)

## Цель

Подложить секреты в Pod **без** записи в Kubernetes etcd: init-контейнер `tuck-agent` кладёт файлы в tmpfs volume `/tuck/secrets/`.

## Кому подходит

Разработчик / platform engineer (самый безопасный путь доставки секретов в Pod).

## Предварительные условия

- Tuck server доступен из кластера
- Mutating webhook injector задеплоен
- Bearer token в K8s Secret (например `tuck-token`)

## Шаги

### 1. Записать секреты в Tuck

```powershell
.\tuckcli.exe kv put db/password "s3cr3t"
.\tuckcli.exe kv put db/user "appuser"
.\tuckcli.exe kv put api/key "key-abc"
```

### 2. Создать namespace с меткой webhook

```powershell
kubectl label namespace demo tuck.io/inject=enabled --overwrite
```

### 3. Создать Secret с Tuck token

```powershell
kubectl -n demo create secret generic tuck-token --from-literal=token=tuck_...
```

Токен должен иметь `read` на пути `db/*`, `api/*`.

### 4. Применить Pod с аннотациями

Пример: `deploy/webhook/example-pod.yaml`

Ключевые аннотации:

| Аннотация | Значение |
|-----------|----------|
| `tuck.io/inject` | `"true"` |
| `tuck.io/addr` | `https://tuck.tuck.svc:8200` |
| `tuck.io/secrets` | `db/password:db-password,db/user:db-user` |
| `tuck.io/token-secret` | `tuck-token` (по умолчанию) |
| `tuck.io/output-dir` | `/tuck/secrets` (по умолчанию) |
| `tuck.io/insecure` | `"true"` только для dev TLS |

Формат `tuck.io/secrets`: `tuckPath:filename` через запятую.

```powershell
kubectl apply -f deploy/webhook/example-pod.yaml
```

### 5. Проверить init-контейнер и файлы

```powershell
kubectl -n demo logs myapp -c tuck-agent
kubectl -n demo exec myapp -- cat /tuck/secrets/db-password
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Pod Running | init `tuck-agent` Completed |
| `cat /tuck/secrets/db-password` | `s3cr3t` |
| `kubectl get secret` в demo | нет ваших app-секретов (только tuck-token) |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| Webhook не срабатывает | Namespace без `tuck.io/inject=enabled` |
| tuck-agent CrashLoop | Неверный addr, TLS, или token |
| Файл не найден | Ошибка в `tuck.io/secrets` (path:filename) |

## Дальше

- [09 — K8s SA auth](09-kubernetes-auth.md) — вместо статического token Secret
- [07 — TuckSecret](07-tucksecret-operator.md) — альтернатива через K8s Secret
