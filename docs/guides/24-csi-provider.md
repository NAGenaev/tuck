# 24 — CSI Provider: монтирование секретов как файлов в Pod

> **Что это?** CSI Provider позволяет монтировать секреты Tuck прямо в файловую систему Pod как обычные файлы — без изменений в коде приложения и без попадания данных в etcd.

## Зачем нужен CSI Provider

| Способ | Минус |
|--------|-------|
| Kubernetes Secret | Хранится в etcd открытым текстом (base64) |
| Webhook Injector | Требует дополнительного sidecar-контейнера |
| Environment Variables | Видны в `kubectl describe pod`, логах, /proc |
| **CSI Provider** | Монтируется в tmpfs (ОЗУ), не попадает в etcd, не требует изменений в приложении |

Файлы появляются в Pod в tmpfs (только в ОЗУ), удаляются при остановке Pod, недоступны другим Pod на той же ноде.

---

## Предварительные требования

- Kubernetes 1.22+ для входа через ServiceAccount (статический токен работает и на 1.20+)
- Tuck запущен и доступен изнутри кластера (через Service)
- Для входа через ServiceAccount: в Tuck настроен доверенный кластер (см. [гайд 09](09-kubernetes-auth.md))
- Helm 3.x

---

## Установка через Helm

### 1. Выбери, как драйвер входит в Tuck

**Способ A (рекомендуется): ServiceAccount Pod'а.** Токена в Secret'ах нет. Kubelet выдаёт каждому Pod'у короткоживущий токен его ServiceAccount, драйвер меняет его в Tuck на токен с правами роли и сам перелогинивается, когда срок токена Tuck истёк. В аудите виден конкретный `namespace/serviceaccount`, а не общий токен.

```bash
# Политика на чтение
tuckcli policy put csi-readonly '[{"path":"secret/**","capabilities":["read"]}]'

# Роль для ServiceAccount, под которым работает приложение
tuckcli auth kubernetes role put myapp-ns myapp-sa \
  --policies=csi-readonly --ttl=1h
```

В спеке Pod'а указывается `tuck.io/auth: "kubernetes"`, `nodePublishSecretRef` не нужен (пример в разделе «Использование в Pod»).

**Способ B: статический токен.** Подходит, если Kubernetes старше 1.22 или Kubernetes-авторизация в Tuck не включена.

```bash
# Токен только на чтение, без срока жизни (см. ограничение ниже)
TOKEN=$(tuckcli token create --name=csi --policy=csi-readonly | jq -r .id)

# Сохрани в K8s Secret
kubectl create secret generic tuck-csi-token \
  --from-literal=token="$TOKEN" \
  -n tuck
```

Политику `csi-readonly` нужно создать заранее (команда выше).

> **Статический токен драйвер сам не продлевает.** Он использует токен из `nodePublishSecretRef` как есть. Поэтому не задавайте `--ttl` (без него токен бессрочный) или берите срок с запасом больше жизни Pod'ов и ротируйте токен вместе с пересозданием Pod'ов. Бессрочный токен с широкими правами нежелателен, поэтому по возможности используйте способ A.

### 2. Включи CSI в Helm-чарте

```bash
helm upgrade --install tuck oci://ghcr.io/nagenaev/tuck \
  --namespace tuck \
  --create-namespace \
  --set csi.enabled=true
```

Или в `values.yaml`:

```yaml
csi:
  enabled: true
  kubeletRootDir: /var/lib/kubelet  # измени для RKE2: /var/lib/rancher/rke2/agent/kubelet
  kubernetesAuth:                   # вход через ServiceAccount Pod'а (способ A), включено по умолчанию
    enabled: true
    audience: ""                    # пусто = audience самого API-сервера, его и проверяет Tuck
    expirationSeconds: 3600         # срок токена ServiceAccount; kubelet обновляет его заранее
```

`kubernetesAuth.enabled` добавляет в `CSIDriver` поля `tokenRequests` и `requiresRepublish`. Если используете только статические токены, поставьте `enabled: false`.

> Имя K8s Secret с токеном для способа B (`tuck-csi-token` в примерах ниже) — не настройка чарта, а обычный `nodePublishSecretRef.name` в спеке конкретного Pod'а: чарт разворачивает только сам драйвер (DaemonSet), а не индивидуальные mount'ы, поэтому у него нет и не может быть единого "чартового" токена на все Pod'ы.

### 3. Проверь, что DaemonSet запустился

```bash
kubectl get daemonset -n tuck
# NAME         DESIRED   CURRENT   READY
# tuck-csi     1         1         1

kubectl get pods -n tuck -l app.kubernetes.io/component=csi
# NAME               READY   STATUS    RESTARTS
# tuck-csi-xxxxx     2/2     Running   0
```

---

## Использование в Pod

### Пример Pod с монтированием KV v1

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
spec:
  volumes:
    - name: tuck-secrets
      csi:
        driver: secrets.tuck.io
        volumeAttributes:
          tuck.io/addr: "http://tuck-server.tuck.svc:8200"
          tuck.io/paths: "myapp"       # путь в KV v1 (через запятую — несколько путей)
          tuck.io/expand-keys: "true"          # каждый ключ = отдельный файл
        nodePublishSecretRef:
          name: tuck-csi-token       # K8s Secret с токеном

  containers:
    - name: app
      image: myapp:latest
      volumeMounts:
        - name: tuck-secrets
          mountPath: /run/secrets
          readOnly: true
```

Имя файла берётся из последнего сегмента пути. Если у двух путей оно совпадает (`db/core/password` и `db/ledger/password`), монтирование **завершается ошибкой**, а не отдаёт приложению один из секретов под чужим именем. Задайте имена явно через `путь=имя_файла`:

```yaml
tuck.io/paths: "db/core/password=core-password,db/ledger/password=ledger-password"
```

После запуска в контейнере:

```
/run/secrets/
  username    ← содержимое поля "username" из myapp
  password    ← содержимое поля "password" из myapp
```

### Вход через ServiceAccount (без токена в Secret)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
  namespace: myapp-ns
spec:
  serviceAccountName: myapp-sa      # для него создана роль в Tuck
  volumes:
    - name: tuck-secrets
      csi:
        driver: secrets.tuck.io
        readOnly: true
        volumeAttributes:
          tuck.io/addr: "http://tuck-server.tuck.svc:8200"
          tuck.io/paths: "myapp"
          tuck.io/auth: "kubernetes"          # вместо nodePublishSecretRef
          tuck.io/refresh-interval: "5m"      # по желанию: живое обновление
  containers:
    - name: app
      image: myapp:latest
      volumeMounts:
        - name: tuck-secrets
          mountPath: /run/secrets
          readOnly: true
```

Токен Tuck живёт столько, сколько задано в `--ttl` роли. С `refresh-interval` драйвер при отказе Tuck заново входит по актуальному токену ServiceAccount, так что ничего ротировать вручную не нужно.

### KV v2

```yaml
volumeAttributes:
  tuck.io/addr: "http://tuck-server.tuck.svc:8200"
  tuck.io/paths: "myapp"
  tuck.io/kv-version: "2"          # включает KV v2
  tuck.io/expand-keys: "true"
```

Монтирование всегда отдаёт актуальную (latest) версию секрета — точечного пиннинга на конкретную версию KV v2 через CSI нет (в отличие, например, от `tuckcli kv get --version`).

### Один файл (без expand-keys)

```yaml
volumeAttributes:
  tuck.io/paths: "db-password"
  tuck.io/expand-keys: "false"   # весь JSON пишется в один файл с именем "db-password"
```

### Настройка прав на файлы

```yaml
volumeAttributes:
  tuck.io/paths: "myapp"
  tuck.io/mode: "0400"   # только чтение для владельца (по умолчанию: "0400")
```

### Live-refresh без пересоздания Pod'а

```yaml
volumeAttributes:
  tuck.io/addr: "http://tuck-server.tuck.svc:8200"
  tuck.io/paths: "myapp"
  tuck.io/refresh-interval: "5m"   # опрашивать Tuck и перезаписывать файлы каждые ~5 минут
```

По умолчанию (атрибут не указан) секрет читается один раз, при монтировании — поведение не меняется для всех Pod-спек, написанных до появления этого атрибута. С `tuck.io/refresh-interval` `tuckcsi` в фоне периодически перечитывает секрет и **атомарно** (temp-файл + rename) перезаписывает содержимое тома, без пересоздания Pod'а. Подробности и нюансы — в разделе [«Как это работает внутри»](#как-это-работает-внутри) ниже.

---

## Параметры volumeAttributes

| Параметр | Значение по умолчанию | Описание |
|----------|----------------------|----------|
| `tuck.io/addr` | (обязательно) | Адрес сервера Tuck |
| `tuck.io/paths` | (обязательно) | Пути к секретам в Tuck относительно mount `secret/` (`myapp`, а не `secret/myapp`), через запятую |
| `tuck.io/auth` | `"token"` | `"kubernetes"` = вход через ServiceAccount Pod'а, `nodePublishSecretRef` не нужен; `"token"` = статический токен из `nodePublishSecretRef` |
| `tuck.io/k8s-cluster` | (кластер по умолчанию) | Имя доверенного кластера на сервере Tuck, если их несколько (только с `tuck.io/auth: kubernetes`) |
| `tuck.io/k8s-audience` | `""` | Audience токена, должен совпадать с `tokenRequests` в `CSIDriver` (только с `tuck.io/auth: kubernetes`) |
| `tuck.io/namespace` | root | Namespace Tuck |
| `tuck.io/kv-version` | `"1"` | Версия KV engine (`"1"` или `"2"`) — точечный пиннинг версии секрета не поддержан |
| `tuck.io/insecure` | `"false"` | `"true"` — пропустить проверку TLS-сертификата (только для dev) |
| `tuck.io/expand-keys` | `"false"` | `"true"` = файл на каждый ключ JSON-объекта, `"false"` = один файл с исходным значением |
| `tuck.io/mode` | `"0400"` | Права на смонтированные файлы (octal) |
| `tuck.io/refresh-interval` | (отключено) | Duration-строка (`"5m"`, `"1h"`) — период фонового обновления файлов; минимальная реальная гранулярность ~30с (значения меньше клэмпятся с warning в логах); не указан = том статичен на весь срок жизни Pod, как раньше |

---

## Как это работает внутри

```
Pod запускается
  ↓
kubelet вызывает NodePublishVolume на CSI Driver
  ↓
tuckcsi берёт токен из nodePublishSecretRef
(или, при tuck.io/auth=kubernetes, меняет токен ServiceAccount Pod'а
 на токен Tuck через /v1/auth/kubernetes/login)
  ↓
tuckcsi запрашивает GET /v1/<path> у Tuck сервера
  ↓
tuckcsi монтирует tmpfs на /var/lib/kubelet/pods/<uid>/volumes/...
  ↓
tuckcsi записывает файлы (в памяти, не на диск)
  ↓
контейнер видит файлы по mountPath
  ↓
Pod останавливается → kubelet вызывает NodeUnpublishVolume
  ↓
tmpfs размонтируется, данные исчезают
```

> **По умолчанию секреты через CSI статичны на весь срок жизни Pod** — `tuckcsi` читает секрет один раз, в момент `NodePublishVolume`, и без `tuck.io/refresh-interval` больше к нему не возвращается. Единственный способ подтянуть новое значение в этом режиме — пересоздать Pod.
>
> **С `tuck.io/refresh-interval` `tuckcsi` фоново перечитывает и перезаписывает файлы**, без пересоздания Pod'а — единый тикер раз в ~30 секунд проверяет, у каких смонтированных томов подошёл срок обновления (поэтому `refresh-interval: "10s"` на практике означает «раз в ~30с», а не буквально каждые 10 секунд — значения меньше 30с автоматически клэмпятся до этого порога). Запись на диск атомарна (temp-файл + rename): контейнер никогда не увидит наполовину записанный файл. Если фоновый запрос к Tuck временно падает (сеть, 5xx), старое содержимое файлов остаётся как есть — ошибка просто логируется, и попытка повторяется на следующем тике.
>
> **Статический токен (`nodePublishSecretRef`) используется для всех фоновых обновлений без изменений** — Kubernetes не передаёт `nodePublishSecretRef` повторно уже смонтированному тому. Если токен истечёт или будет отозван, фоновые обновления начнут молча падать, а том «застынет» на последнем успешно полученном значении (с warning в логах `tuckcsi` каждые ~30с) — без креша драйвера и без порчи файлов. Если секрету нужна ротация без пересоздания Pod'а, выставляй TTL токена заведомо дольше ожидаемого времени жизни Pod'а или переходи на вход через ServiceAccount.
>
> **Вход через ServiceAccount (`tuck.io/auth: kubernetes`) работает иначе.** `CSIDriver` объявляет `tokenRequests` и `requiresRepublish`, поэтому kubelet передаёт драйверу токен ServiceAccount Pod'а и до его истечения (по умолчанию час) повторно вызывает `NodePublishVolume` с новым токеном. Повторный вызов только подменяет учётные данные в памяти драйвера: tmpfs и файлы не трогаются. Токен Tuck драйвер получает при монтировании и меняет заново, как только Tuck ответит 401/403 (истёк срок роли). Если и токен ServiceAccount успел истечь, файлы остаются как есть, а в логе будет warning, пока kubelet не пришлёт новый. После перезапуска драйвера смонтированные тома подхватываются при ближайшем republish. Токен ServiceAccount уходит на `tuck.io/addr`, поэтому вне dev-окружения используйте HTTPS.
>
> **Audience.** По умолчанию токен выдаётся с audience API-сервера (`audience: ""`), потому что Tuck проверяет его через TokenReview без собственного audience. Отдельный audience для Tuck сейчас не поддержан на стороне сервера.
>
> Если приложению нужен настоящий Kubernetes `Secret`-объект, обновляемый живьём (а не просто файлы в tmpfs одного Pod'а) — используй [Operator/TuckSecret CRD](07-tucksecret-operator.md) вместо CSI.

---

## Установка без Helm (сырые манифесты)

```bash
kubectl apply -f deploy/csi/driver.yaml

# Только для способа B (статический токен): Secret с токеном
kubectl create secret generic tuck-csi-token \
  --from-literal=token="<токен>" \
  -n tuck
```

`deploy/csi/driver.yaml` уже содержит `tokenRequests` и `requiresRepublish` для способа A.

---

## Отладка

### CSI Driver не регистрируется

```bash
# Проверь логи
kubectl logs -n tuck -l app.kubernetes.io/component=csi -c tuckcsi
kubectl logs -n tuck -l app.kubernetes.io/component=csi -c node-driver-registrar

# Проверь socket
kubectl exec -n tuck <pod-csi> -c tuckcsi -- ls -la /csi/
```

### Volume не монтируется

```bash
# События Pod
kubectl describe pod myapp

# Частые причины:
# - "token not found" → проверь имя Secret в nodePublishSecretRef.name
# - "kubelet passed no ServiceAccount token" → у CSIDriver нет tokenRequests (csi.kubernetesAuth.enabled=false или старый манифест)
# - "kubernetes login ... HTTP 403" → нет роли для namespace/serviceaccount Pod'а (tuckcli auth kubernetes role put)
# - "connection refused" → проверь tuck.io/addr
# - "permission denied" → проверь политику токена (нужен path read)
```

### Неверный путь kubelet (RKE2, K3s)

```bash
# RKE2:
--set csi.kubeletRootDir=/var/lib/rancher/rke2/agent/kubelet

# K3s:
--set csi.kubeletRootDir=/var/lib/rancher/k3s/agent/kubelet
```

---

## Проверка

```bash
# 1. Запусти тестовый Pod
kubectl apply -f deploy/csi/example-pod.yaml

# 2. Проверь смонтированные файлы
kubectl exec example-pod -- ls /run/secrets/
kubectl exec example-pod -- cat /run/secrets/username

# 3. Убедись, что это tmpfs (ОЗУ, не диск)
kubectl exec example-pod -- df -h /run/secrets
# Filesystem      Size  Used Avail Use%  Mounted on
# tmpfs           64M   4.0K  64M   1%   /run/secrets

# 4. Удали Pod → файлы исчезли
kubectl delete pod example-pod
```

---

## Сравнение с Webhook Injector

| | CSI Provider | Webhook Injector |
|-|-------------|-----------------|
| Изменение Pod spec | Только `volumes:` + `volumeMounts:` | Sidecar добавляется автоматически |
| Требует cert-manager | Нет | Да (для TLS webhook) |
| Обновление секрета | При рестарте Pod, либо live через `tuck.io/refresh-interval` | При рестарте Pod |
| Поддержка Windows nodes | Нет | Нет |
| Поддержка KV v2 | Да | Да |

---

[← К списку гайдов](README.md)
