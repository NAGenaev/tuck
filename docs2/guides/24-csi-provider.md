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
  --set csi.enabled=true \
  --set csi.tokenSecretName=tuck-csi-token
```

Или в `values.yaml`:

```yaml
csi:
  enabled: true
  tokenSecretName: tuck-csi-token
  kubeletRootDir: /var/lib/kubelet  # измени для RKE2: /var/lib/rancher/rke2/agent/kubelet
```

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
          tuckAddr: "http://tuck-server.tuck.svc:8200"
          path: "secret/myapp"      # путь в KV v1
          expandKeys: "true"         # каждый ключ = отдельный файл
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

### KV v2 (с версиями)

```yaml
volumeAttributes:
  tuckAddr: "http://tuck-server.tuck.svc:8200"
  path: "kv/myapp"
  kvVersion: "2"          # включает KV v2
  version: "3"            # конкретная версия (по умолчанию: latest)
  expandKeys: "true"
```

### Один файл (без expandKeys)

```yaml
volumeAttributes:
  path: "secret/db-password"
  expandKeys: "false"   # весь JSON пишется в файл "secret"
```

### Настройка прав на файлы

```yaml
volumeAttributes:
  path: "secret/myapp"
  filePermission: "0400"   # только для владельца (по умолчанию: 0640)
```

---

## Параметры volumeAttributes

| Параметр | Значение по умолчанию | Описание |
|----------|----------------------|----------|
| `tuckAddr` | `TUCK_ADDR` из окружения | Адрес сервера Tuck |
| `path` | (обязательно) | Путь к секрету в Tuck |
| `kvVersion` | `"1"` | Версия KV engine (`"1"` или `"2"`) |
| `version` | latest | Версия секрета (только KV v2) |
| `expandKeys` | `"true"` | `"true"` = файл на каждый ключ, `"false"` = один файл |
| `filePermission` | `"0640"` | Права на смонтированные файлы (octal) |

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
# - "token not found" → проверь tokenSecretName
# - "connection refused" → проверь tuckAddr
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
| Обновление секрета | При рестарте Pod | При рестарте Pod |
| Поддержка Windows nodes | Нет | Нет |
| Поддержка KV v2 | Да | Да |

---

[← К списку гайдов](README.md)
