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

- Kubernetes 1.20+
- Tuck запущен и доступен изнутри кластера (через Service)
- Helm 3.x
- Токен Tuck с правами на чтение нужных секретов

---

## Установка через Helm

### 1. Создай токен для CSI Driver

```bash
# Создай ограниченный токен
TOKEN=$(tuckcli token create \
  --policies=csi-readonly \
  --ttl=0 \
  --format=json | jq -r .id)

# Сохрани в K8s Secret
kubectl create secret generic tuck-csi-token \
  --from-literal=token="$TOKEN" \
  -n tuck
```

Создай политику `csi-readonly` заранее (пример):

```json
{
  "paths": [
    {"path": "secret/*", "capabilities": ["read"]},
    {"path": "kv/*",     "capabilities": ["read"]}
  ]
}
```

```bash
tuckcli policy put csi-readonly '{"paths":[{"path":"secret/*","capabilities":["read"]}]}'
```

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
```

> Имя K8s Secret с токеном (`tuck-csi-token` в примерах ниже) — не настройка чарта, а обычный `nodePublishSecretRef.name` в спеке конкретного Pod'а: чарт разворачивает только сам драйвер (DaemonSet), а не индивидуальные mount'ы, поэтому у него нет и не может быть единого "чартового" токена на все Pod'ы.

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
          tuck.io/paths: "secret/myapp"       # путь в KV v1 (через запятую — несколько путей)
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

После запуска в контейнере:

```
/run/secrets/
  username    ← содержимое поля "username" из secret/myapp
  password    ← содержимое поля "password" из secret/myapp
```

### KV v2

```yaml
volumeAttributes:
  tuck.io/addr: "http://tuck-server.tuck.svc:8200"
  tuck.io/paths: "kv/myapp"
  tuck.io/kv-version: "2"          # включает KV v2
  tuck.io/expand-keys: "true"
```

Монтирование всегда отдаёт актуальную (latest) версию секрета — точечного пиннинга на конкретную версию KV v2 через CSI нет (в отличие, например, от `tuckcli kv get --version`).

### Один файл (без expand-keys)

```yaml
volumeAttributes:
  tuck.io/paths: "secret/db-password"
  tuck.io/expand-keys: "false"   # весь JSON пишется в один файл с именем "db-password"
```

### Настройка прав на файлы

```yaml
volumeAttributes:
  tuck.io/paths: "secret/myapp"
  tuck.io/mode: "0400"   # только чтение для владельца (по умолчанию: "0400")
```

### Live-refresh без пересоздания Pod'а

```yaml
volumeAttributes:
  tuck.io/addr: "http://tuck-server.tuck.svc:8200"
  tuck.io/paths: "secret/myapp"
  tuck.io/refresh-interval: "5m"   # опрашивать Tuck и перезаписывать файлы каждые ~5 минут
```

По умолчанию (атрибут не указан) секрет читается один раз, при монтировании — поведение не меняется для всех Pod-спек, написанных до появления этого атрибута. С `tuck.io/refresh-interval` `tuckcsi` в фоне периодически перечитывает секрет и **атомарно** (temp-файл + rename) перезаписывает содержимое тома, без пересоздания Pod'а. Подробности и нюансы — в разделе [«Как это работает внутри»](#как-это-работает-внутри) ниже.

---

## Параметры volumeAttributes

| Параметр | Значение по умолчанию | Описание |
|----------|----------------------|----------|
| `tuck.io/addr` | (обязательно) | Адрес сервера Tuck |
| `tuck.io/paths` | (обязательно) | Пути к секретам в Tuck, через запятую |
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
tuckcsi читает токен из nodePublishSecretRef
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
> **Токен, переданный через `nodePublishSecretRef` при монтировании, используется для всех последующих фоновых обновлений без изменений** — Kubernetes не передаёт `nodePublishSecretRef` повторно уже смонтированному тому. Если токен истечёт или будет отозван, фоновые обновления начнут молча падать, а том «застынет» на последнем успешно полученном значении (с warning в логах `tuckcsi` каждые ~30с) — без креша драйвера и без порчи файлов, но и без подсказки об этом иначе, чем через логи. Если секрету нужна ротация без пересоздания Pod'а, выставляй TTL токена заведомо дольше ожидаемого времени жизни Pod'а.
>
> Если приложению нужен настоящий Kubernetes `Secret`-объект, обновляемый живьём (а не просто файлы в tmpfs одного Pod'а) — используй [Operator/TuckSecret CRD](07-tucksecret-operator.md) вместо CSI.

---

## Установка без Helm (сырые манифесты)

```bash
kubectl apply -f deploy/csi/driver.yaml

# Создай Secret с токеном
kubectl create secret generic tuck-csi-token \
  --from-literal=token="<токен>" \
  -n tuck
```

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
