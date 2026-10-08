# 13 — Подключить команду: namespace, политика, роли

[← Все гайды](README.md)

## Цель

За один проход выдать команде `team-a` свой изолированный раздел Tuck: сотрудники входят через LDAP, приложения в Kubernetes читают секреты через ServiceAccount или оператор. Это сквозной сценарий, который собирает вместе гайды [09](09-kubernetes-auth.md), [12](12-ldap.md) и [07](07-tucksecret-operator.md).

## Кто что делает

| Шаг | Кто |
|-----|-----|
| 1–3. namespace, политика, роли | администратор Tuck (root или токен с правом на `sys/namespaces`, `auth/*`) |
| 4. аннотация Kubernetes-namespace | администратор кластера |
| 5. вход сотрудников | сами сотрудники, ничего не настраивая |

Namespace в Tuck создаёт только администратор. Из LDAP они не создаются: группа каталога лишь определяет, в какой **уже существующий** namespace попадёт пользователь.

## Шаги

### 1. Namespace

```bash
tuckcli namespace create team-a
```

Всё, что находится внутри (секреты, политики), изолировано от других namespace. Удаление namespace удаляет и его данные.

### 2. Политика внутри namespace

Политики namespace хранятся в самом namespace, поэтому флаг `--namespace` обязателен:

```bash
tuckcli --namespace=team-a policy put team-a-rw \
  '[{"path":"secret/**","capabilities":["read","write","delete","list"]}]'

tuckcli --namespace=team-a policy put team-a-read \
  '[{"path":"secret/**","capabilities":["read","list"]}]'
```

### 3. Роли: кому и что дать

Сотрудники (группа из LDAP получает запись и чтение, привязка к `team-a`):

```bash
tuckcli auth ldap role put team-a-devs \
  --groups=team-a-devs --policies=team-a-rw --ttl=8h --tuck-namespace=team-a
```

Приложения (ServiceAccount `my-app` из Kubernetes-namespace `app-a` читает секреты `team-a`):

```bash
tuckcli auth kubernetes role put app-a my-app \
  --policies=team-a-read --ttl=1h --tuck-namespace=team-a
```

Проверка ролей: `tuckcli auth ldap role list`, `tuckcli auth kubernetes role get app-a my-app`.

### 4. Оператор: привязать Kubernetes-namespace к Tuck-namespace

Если секреты в Kubernetes синхронизирует оператор (TuckSecret), администратор кластера один раз размечает namespace приложения:

```bash
kubectl annotate namespace app-a tuck.io/tuck-namespace=team-a --overwrite
```

Дальше команда пишет обычные манифесты:

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db
  namespace: app-a
spec:
  secretName: db-creds
  tuckPath: db/password
  secretKey: password
  refreshInterval: 1m
```

Оператор прочитает `secret/db/password` из Tuck-namespace `team-a` и создаст Kubernetes Secret `db-creds`. Для работы аннотации оператор должен быть запущен с `--enforce-namespace-paths` (Helm: `operator.enforceNamespacePaths=true`). Подробности: [07](07-tucksecret-operator.md).

### 5. Положить первый секрет и войти сотрудником

```bash
tuckcli --namespace=team-a kv put db/password 's3cret'
tuckcli auth ldap login --username=alice --password=...
```

В UI сотрудник открывает страницу входа, выбирает вкладку LDAP и вводит логин и пароль. Если его группы дают доступ в нескольких namespace, UI предложит выбрать один из них.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `tuckcli --namespace=team-a kv list ""` под токеном администратора | видны ключи команды |
| тот же список в root (без `--namespace`) | ключей команды нет |
| токен `team-a` с заголовком `X-Tuck-Namespace: team-b` | `403` |
| `TuckSecret` в `app-a` | Secret `db-creds` создан, значение совпадает |
| TuckSecret в namespace без аннотации | не читает данные `team-a` |

## Частые ошибки

- **`policy put` без `--namespace` не помогает команде** — политика создаётся в root, а токен команды ищет её в `team-a`. Всегда указывайте `--namespace`.
- **403 при входе по LDAP** — нет роли для групп пользователя (проверьте CN/DN группы).
- **409 при входе** — у пользователя роли в нескольких namespace; выберите один (`--namespace=...`).
- **Оператор не видит данные `team-a`** — не включён `--enforce-namespace-paths` или на Namespace нет аннотации `tuck.io/tuck-namespace`.

## Дальше

- [12 — LDAP](12-ldap.md)
- [09 — Kubernetes auth](09-kubernetes-auth.md)
- [07 — TuckSecret](07-tucksecret-operator.md)
