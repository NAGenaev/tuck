# 02 — Политики и scoped-токены

[← Все гайды](README.md)

## Цель

Выдать приложению токен, который может **только читать** секреты по заданному префиксу — не root.

## Кому подходит

Разработчик, DevOps, security-инженер.

## Предварительные условия

- Работающий Tuck ([01 — Первый секрет](01-pervyy-sekret.md))
- Root или admin-токен в `TUCK_TOKEN`

## Шаги

### 1. Создать секреты для демо

```powershell
.\tuckcli.exe kv put app/api-key "key-111"
.\tuckcli.exe kv put app/db-password "pass-222"
.\tuckcli.exe kv put admin/root-key "super-secret"
```

### 2. Создать политику (ACL)

Формат — JSON-массив правил, у каждого `path` и `capabilities` (`read`, `write`, `delete`, `list`). Пути пишутся с mount: `secret/...`.

```powershell
'[{"path":"secret/app/*","capabilities":["read","list"]}]' | .\tuckcli.exe policy put app-readonly -
```

> Windows PowerShell 5.1 съедает двойные кавычки внутри аргумента нативной программы, поэтому JSON передаётся через pipe и `-` (чтение из stdin). Не передавайте его аргументом: `policy put name '[{"path":...}]'` в PS 5.1 сломается. В PowerShell 7 и в bash работают оба варианта.

Проверка:

```powershell
.\tuckcli.exe policy get app-readonly
```

### 3. Выпустить scoped-токен

```powershell
.\tuckcli.exe token create --name=my-app --policy=app-readonly --ttl=24h
```

Ответ в JSON; сохраните поле `id` (это и есть токен `tuck_...`).

### 4. Проверить доступ scoped-токеном

```powershell
$env:TUCK_TOKEN = "tuck_..."   # новый scoped token
.\tuckcli.exe kv get app/api-key          # OK
.\tuckcli.exe kv get admin/root-key       # 403 — нет доступа
```

### 5. (Опционально) Token role для повторной выдачи

```powershell
.\tuckcli.exe token role create --name=app-reader --policy=app-readonly --ttl=1h --renewable
.\tuckcli.exe token create-role app-reader
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `policy get app-readonly` | правило с `secret/app/*` и `read`, `list` |
| `kv get app/api-key` (scoped) | успех |
| `kv get admin/root-key` (scoped) | `403 permission denied` |
| `token lookup-self` | policies содержит `app-readonly` |

## Модель доступа в Tuck

```mermaid
flowchart LR
  User[Пользователь или SA] --> Login[Auth method]
  Login --> Token[Token с policies]
  Token --> ACL[Policy rules path plus caps]
  ACL --> Secret[KV / Cubbyhole]
```

- **Deny-first**: если путь не разрешён явно — доступ запрещён.
- Пути в политиках указываются с mount: `secret/...`, `cubbyhole/...`.

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| 403 на разрешённый путь | Проверьте префикс: `secret/app/*`, не `app/*` |
| Политика не применилась | Токен создан до `policy put` — перевыпустите токен |
| `policy put` падает с `invalid JSON` | JSON должен быть массивом `[{"path":...,"capabilities":[...]}]`, не объектом `{"paths":...}`; в PS 5.1 передавайте его через pipe и `-` |

## Дальше

- K8s SA auth: [09 — Kubernetes SA auth](09-kubernetes-auth.md)
- AppRole для CI: [10 — AppRole](10-approle.md)
