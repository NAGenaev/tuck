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

`root` — только для локального стенда. В продакшне дайте оператору узкую политику с `read` на пути, которые команды вправе синхронизировать (оператор ходит в Tuck **одной** identity для всех namespace — см. «Изоляция по namespace» ниже).

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

### Несколько ключей в одном Secret

Реальные чарты ждут Secret с несколькими ключами. Используйте `data` (ключ Secret → путь в Tuck); его можно комбинировать с `tuckPath`/`secretKey`:

```yaml
spec:
  secretName: myapp-db
  data:
    username: team-a/db/user
    password: team-a/db/pass
    ca.crt:   team-a/db/ca
```

Правила:

- Все значения читаются из Tuck **до** записи. Если хоть один путь недоступен — Secret не меняется, в `status` попадает ошибка.
- Несколько TuckSecret могут наполнять один Secret, если владеют **разными ключами**: оператор делает merge (чужие ключи, labels, annotations и `type` сохраняются). Два TuckSecret на один ключ одного Secret — ошибка `conflict` у обоих, ничего не перезаписывается молча.
- Ключ, удалённый из `spec`, из Secret не удаляется — чистите его вручную или пересоздайте Secret.

## Изоляция по namespace (`--enforce-namespace-paths`)

Оператор логинится в Tuck один раз, своей SA, поэтому без ограничений любая команда, которой разрешено создавать TuckSecret в своём namespace, может указать **любой** путь, доступный оператору. С флагом `--enforce-namespace-paths` (Helm: `operator.enforceNamespacePaths=true`):

- TuckSecret в namespace `team-a` может читать только `team-a/...`;
- администратор кластера может выдать дополнительные пути аннотацией на объекте Namespace:

```powershell
kubectl annotate namespace team-a tuck.io/allowed-paths="shared/tls,platform/common" --overwrite
```

- сравнение идёт по границе сегмента (`team-a` не даёт `team-ab/...`), пути с `.`/`..`, пустыми сегментами, `? # % \` и пробелами отклоняются;
- если Namespace не удалось прочитать или аннотация некорректна — доступ запрещается (fail closed);
- оператору нужен RBAC `get namespaces` (Helm добавляет его сам при включённом флаге).

Аннотацию Namespace могут менять только администраторы кластера, поэтому команда не может расширить себе доступ из своего namespace. Политика оператора в Tuck при этом должна по-прежнему быть ограничена (например, `secret/*` на чтение, но не `root`): флаг — это второй рубеж, а не замена ACL.

## Минимальные права оператора в Kubernetes

По умолчанию ClusterRole оператора разрешает писать Secret **во всех** namespace — при компрометации оператора это чтение/подмена любого Secret кластера. Для многокомандных кластеров ограничьте запись списком командных namespace:

```yaml
operator:
  enforceNamespacePaths: true
  rbac:
    scope: namespaces
    namespaces: [team-a, team-b]
```

Тогда:

- Secret (и `patch` на Deployment/StatefulSet/DaemonSet при `enableRestartTargets`) выдаются только `RoleBinding`-ами в перечисленных namespace; кластерным остаётся лишь чтение TuckSecret (и `get namespaces` при `enforceNamespacePaths`);
- Lease лидер-выборов — `Role` в одном namespace, не cluster-wide;
- TuckSecret в namespace вне списка получает в `status` ошибку 403, а не синхронизируется. Новая команда = добавить её namespace в `rbac.namespaces` и выполнить `helm upgrade`.

Без Helm: `deploy/operator/namespaced-rbac.yaml`.

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
| `tuckPath` | Путь в Tuck KV (mount `secret/`); вместе с `secretKey` |
| `secretName` | Имя целевого K8s Secret (обязательно) |
| `secretKey` | Ключ в `data` Secret для `tuckPath` |
| `data` | Карта «ключ Secret → путь в Tuck» для нескольких ключей |
| `refreshInterval` | Интервал опроса (например `30s`, `5m`) |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| TuckSecret Pending | Оператор не запущен или нет K8s auth role |
| Secret пустой | Неверный `tuckPath` — проверьте `kv get` |
| 403 в логах оператора | Политика или role для SA оператора |
| `outside the paths allowed for namespace` | Путь вне `<namespace>/`; админ может добавить его аннотацией `tuck.io/allowed-paths` на Namespace |
| `conflict: TuckSecret ... also writes key` | Два TuckSecret пишут один ключ одного Secret — разведите ключи |

## Дальше

- Без etcd: [08 — Webhook injector](08-webhook-injector.md)
- SA auth для приложений: [09 — Kubernetes auth](09-kubernetes-auth.md)
