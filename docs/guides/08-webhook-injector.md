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

### 3a. Рекомендуемый путь: без статического токена (Kubernetes auth)

Добавьте аннотацию `tuck.io/auth: kubernetes`. Агент возьмёт JWT ServiceAccount Pod'а, обменяет его на короткоживущий токен Tuck (`POST /v1/auth/kubernetes/login`), и Secret с токеном не нужен. Роль для ServiceAccount создаётся как в [гайде 09](09-kubernetes-auth.md); у Pod'а должен быть включён automount ServiceAccount-токена. Пропустите шаг 3b.

### 3b. Альтернатива: статический токен

> Статический токен долгоживущий и лежит в K8s Secret, то есть в etcd — именно то, от чего Tuck призван уйти. Используйте только для dev.

```powershell
kubectl -n demo create secret generic tuck-token --from-literal=token=tuck_...
```

Токен должен иметь `read` на пути `db/*`, `api/*`.

### 4. Применить Pod с аннотациями

Пример: `deploy/webhook/example-pod.yaml`

Ключевые аннотации:

| Аннотация | Значение |
|-----------|----------|
| `tuck.io/inject` | `"true"` — нужна и как аннотация, и как метка (label) Pod'а: webhook срабатывает по метке (objectSelector), а решение об инъекции берётся из аннотации. В Deployment — в `spec.template.metadata` |
| `tuck.io/addr` | `https://tuck.tuck.svc:8200` |
| `tuck.io/secrets` | `db/password:db-password,db/user:db-user` |
| `tuck.io/auth` | `kubernetes` — логин по ServiceAccount вместо токена из Secret |
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

### Если injector недоступен

По умолчанию webhook работает с `failurePolicy: Ignore`: пока injector лежит, Pod создаётся **без** init-контейнера и приложение стартует без секретов. Для продакшна, где это недопустимо, задайте `injector.failurePolicy=Fail` (Helm): создание Pod'ов с меткой `tuck.io/inject=true` в помеченных namespace будет блокироваться, пока injector не поднимется. Запускайте не менее 2 реплик injector.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Pod Running | init `tuck-agent` Completed |
| `cat /tuck/secrets/db-password` | `s3cr3t` |
| `kubectl get secret` в demo | нет ваших app-секретов (только tuck-token) |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| Webhook не срабатывает | Namespace без `tuck.io/inject=enabled` или Pod без метки `tuck.io/inject: "true"` |
| tuck-agent CrashLoop | Неверный addr, TLS, или token |
| Файл не найден | Ошибка в `tuck.io/secrets` (path:filename) |

## Дальше

- [09 — K8s SA auth](09-kubernetes-auth.md) — вместо статического token Secret
- [07 — TuckSecret](07-tucksecret-operator.md) — альтернатива через K8s Secret
