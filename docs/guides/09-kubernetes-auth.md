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

То же через CLI (с привязкой токенов роли к namespace Tuck через `--tuck-namespace`, пустое значение — root):

```bash
tuckcli auth kubernetes role put default my-app --policies=my-app-policy --ttl=1h --tuck-namespace=team-a
tuckcli auth kubernetes role get default my-app
```

### 4. Логин с хоста (тест)

```powershell
$SA_TOKEN = kubectl create token my-app -n default --duration=1h

curl -X POST "$env:TUCK_ADDR/v1/auth/kubernetes/login" `
  -H "Content-Type: application/json" `
  -d "{`"token`":`"$SA_TOKEN`"}"
```

В ответе — поле `token` (scoped token).

### 5. Использовать client token

```powershell
$APP_TOKEN = "tuck_..."   # из login
curl "$env:TUCK_ADDR/v1/secret/app/config" -H "X-Tuck-Token: $APP_TOKEN"
```

### 6. В Pod (типичный паттерн)

Sidecar или init-скрипт:

1. Читает SA token из `/var/run/secrets/kubernetes.io/serviceaccount/token`
2. `POST /v1/auth/kubernetes/login`
3. Использует `token` для API

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| login с SA JWT | `token` + `policies: ["my-app-policy"]` |
| read `secret/app/*` | 200 |
| read `secret/admin/*` | 403 |

## Много кластеров: топология

Один Tuck доверяет **одному** кластеру по умолчанию (`--k8s-api`): роль определяется парой `namespace/serviceaccount`, и в двух кластерах `prod/api` – это одна и та же роль. Для банка с несколькими кластерами есть три схемы:

| Схема | Когда | Что делать |
|---|---|---|
| **Tuck на сегмент** (рекомендуется для пилота) | сегменты изолированы сетью и ИБ | на каждый сегмент свой Tuck со своим `--k8s-api`; ничего не пересекается |
| **Один Tuck, несколько кластеров** | общий Tuck-кластер (Raft), кластеры одного уровня доверия | перечислить кластеры в конфиге (ниже), роли создавать под `clusters/<имя>` |
| **AppRole / JWT-OIDC** | кластеры без доступа Tuck к их TokenReview API | см. [10](10-approle.md), [11](11-jwt-oidc.md) |

Несколько кластеров в одном Tuck:

```yaml
kubernetes:
  api: https://k8s-dc1.bank.local:6443          # кластер по умолчанию
  token_file: /etc/tuck/k8s/dc1.token
  ca_file: /etc/tuck/k8s/dc1.ca
  clusters:
    - name: dc2
      api: https://k8s-dc2.bank.local:6443
      token_file: /etc/tuck/k8s/dc2.token      # SA токен с правом create tokenreviews
      ca_file: /etc/tuck/k8s/dc2.ca
```

Роль в конкретном кластере (в кластере по умолчанию путь прежний):

```bash
curl -X PUT -H "X-Tuck-Token: $ROOT" $TUCK/v1/auth/kubernetes/clusters/dc2/role/prod/api \n  -d '{"policies":["prod-read"],"tuck_namespace":"prod"}'
```

Логин: в теле добавляется `"cluster":"dc2"` (пусто = кластер по умолчанию). Подам этого не нужно знать вручную: `tuck.io/auth: kubernetes` + `tuck.io/cluster: dc2` в Injector, `TUCK_K8S_CLUSTER=dc2` в агенте. Токен, выданный кластером dc2, не пройдёт проверку в другом кластере, поэтому Pod не может выдать себя за кластер, которого нет в его аннотации. Политики можно ограничивать по кластеру путём `auth/k8s/cluster/dc2/role/*`.

Ограничения: список кластеров читается при старте (изменение – рестарт); Operator выбирает кластер флагом `--tuck-cluster`.

### Ротация токена reviewer

`token_file` перечитывается **при каждом TokenReview**, поэтому токен можно менять без рестарта Tuck (покрыто тестом `TestReviewer_RereadsTokenFileEveryCall`). Не держите бессрочный токен из Secret — выпускайте короткий и обновляйте файл:

- **Свой кластер** (Tuck внутри кластера): projected-том `serviceAccountToken` с `expirationSeconds` — kubelet сам обновляет файл, делать ничего не нужно.
- **Удалённый кластер** (`dc2`): ограниченный токен выпускается снаружи и кладётся в файл на узле Tuck, например по расписанию (cron / systemd-timer, период меньше срока токена):

```bash
kubectl --context dc2 -n tuck-reviewer create token tuck-reviewer --duration=24h \
  > /etc/tuck/k8s/dc2.token.new && mv /etc/tuck/k8s/dc2.token.new /etc/tuck/k8s/dc2.token
```

`mv` атомарен — Tuck не увидит наполовину записанный файл. SA `tuck-reviewer` должен иметь только `create` на `tokenreviews.authentication.k8s.io` (ClusterRole `system:auth-delegator`). Если файл недоступен, логин отклоняется (ошибка, а не пропуск).

## Дальше

- [08 — Injector](08-webhook-injector.md) + scoped token
- [02 — Политики](02-politiki-i-dostup.md)
