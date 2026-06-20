# Tuck — Руководство по начальному развёртыванию

Настоящий документ описывает порядок развёртывания и начальной настройки системы Tuck.

---

## 1. Предварительные требования

- Go 1.25+ (для сборки из исходного кода) **или** готовый исполняемый файл из раздела [Releases](https://github.com/NAGenaev/tuck/releases)
- Для развёртывания в Kubernetes: `kubectl` + работающий кластер, `helm` 3.x

---

## 2. Установка

### 2.1. Использование готового исполняемого файла

```sh
# Linux / macOS (amd64)
curl -Lo tuck https://github.com/NAGenaev/tuck/releases/latest/download/tuck_linux_amd64
chmod +x tuck && mv tuck /usr/local/bin/

# CLI
curl -Lo tuckcli https://github.com/NAGenaev/tuck/releases/latest/download/tuckcli_linux_amd64
chmod +x tuckcli && mv tuckcli /usr/local/bin/
```

### 2.2. Сборка из исходного кода

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck
go install ./cmd/tuck ./cmd/tuckcli
```

---

## 3. Запуск в режиме разработки

Режим разработки обеспечивает автоматическое снятие печати при каждом запуске — предназначен для локальной разработки и тестирования.

```sh
tuck --seal-type=dev --tls-auto
```

Ожидаемый вывод:

```
tuck: TLS enabled (auto-generated self-signed — dev only)
==========================================================
ROOT TOKEN (отображается однократно — сохранить в защищённом хранилище):
  tuck_XXXXXXXXXXXXXXXXXXXX
==========================================================
tuck: unsealed (dev seal) — https://127.0.0.1:8200
```

Корневой токен необходим для всех последующих операций.

> Режим разработки не предназначен для использования в производственных средах.

---

## 4. Запись и чтение первого секрета

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_XXXXXXXXXXXXXXXXXXXX

# Запись секрета
tuckcli kv put myapp/db-password "s3cr3t"

# Чтение секрета
tuckcli kv get myapp/db-password
# {"path":"myapp/db-password","value":"s3cr3t"}

# Перечисление секретов по префиксу
tuckcli kv list myapp/
# {"keys":["db-password"]}
```

Вариант с использованием `curl` (пропуск проверки TLS для самоподписанного сертификата):

```sh
curl -sk -X PUT https://127.0.0.1:8200/v1/secret/myapp/db-password \
  -H "X-Tuck-Token: $TUCK_TOKEN" -d 's3cr3t'

curl -sk https://127.0.0.1:8200/v1/secret/myapp/db-password \
  -H "X-Tuck-Token: $TUCK_TOKEN"
```

Веб-интерфейс управления доступен по адресу: **https://127.0.0.1:8200/ui/**

---

## 5. Создание областного токена и политики

Предоставление приложению доступа только для чтения к пространству `myapp/*`:

```sh
# Создание политики
tuckcli policy put myapp-ro '[{"path":"myapp/*","capabilities":["read","list"]}]'

# Создание кратковременного токена с указанной политикой
tuckcli token create --name=myapp --policy=myapp-ro --ttl=24h
# {"id":"tuck_YYYY...","ttl":"24h","policies":["myapp-ro"]}

# Проверка разграничения доступа
TUCK_TOKEN=tuck_YYYY... tuckcli kv get myapp/db-password   # Разрешено
TUCK_TOKEN=tuck_YYYY... tuckcli kv put myapp/x y           # 403 Forbidden
```

---

## 6. Конфигурационный файл (рекомендуется для производственной среды)

Вместо набора флагов командной строки создаётся файл `tuck.yaml`:

```yaml
addr: "0.0.0.0:8200"
data: "/var/lib/tuck/tuck.db"

tls:
  cert: "/etc/tuck/tls.crt"
  key:  "/etc/tuck/tls.key"

seal:
  type: "shamir"
  shamir:
    n: 5   # общее число долей
    k: 3   # число долей, необходимых для снятия печати
```

Запуск:

```sh
tuck --config=/etc/tuck/tuck.yaml
```

Флаги командной строки имеют приоритет над значениями конфигурационного файла. Путь к конфигурационному файлу по умолчанию задаётся переменной среды `TUCK_CONFIG`.

**Требования к конфиденциальным параметрам:** значения секретов не допускается помещать в конфигурационный файл. Используются переменные среды:

```sh
export TUCK_TRANSIT_TOKEN=hvs.XXXXXX
tuck --config=/etc/tuck/tuck.yaml
```

---

## 7. Производственная среда: схема Шамира

```sh
tuck --config=/etc/tuck/tuck.yaml   # первый запуск формирует доли
```

```
ROOT TOKEN: tuck_...
ДОЛИ ШАМИРА (распределить операторам):
  [1] a1b2c3...
  [2] d4e5f6...
  [3] ...
  [4] ...
  [5] ...
```

После перезапуска для снятия печати:

```sh
tuckcli unseal <доля-1>
tuckcli unseal <доля-2>
tuckcli unseal <доля-3>
# tuck: unsealed
```

---

## 8. Развёртывание в Kubernetes

### 8.1. Helm (рекомендуемый способ)

```sh
helm install tuck deploy/helm/tuck \
  --namespace tuck-system --create-namespace \
  --set seal.type=awskms \
  --set seal.awskms.keyId=alias/tuck-seal \
  --set seal.awskms.region=us-east-1
```

При использовании AWS KMS на EKS автоматическое снятие печати осуществляется через IRSA. Ручное снятие печати не требуется.

### 8.2. CRD TuckSecret

Синхронизация секрета Tuck в нативный Kubernetes Secret:

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: myapp
spec:
  path: myapp/db-password
  destination:
    name: db-password
    key: password
```

```sh
kubectl apply -f tucksecret.yaml
kubectl get secret db-password -n myapp -o jsonpath='{.data.password}' | base64 -d
# s3cr3t
```

### 8.3. Инжектор агента

Аннотирование пода для внедрения секретов в том tmpfs (без записи в etcd):

```yaml
metadata:
  annotations:
    tuck.io/inject: "true"
    tuck.io/role: "myapp"
    tuck.io/secret-path: "myapp/db-password"
    tuck.io/secret-dest: "/run/secrets/db-password"
```

---

## 9. Динамические учётные данные

Генерация краткосрочных учётных данных по запросу вместо статических паролей:

```sh
# PostgreSQL
tuckcli db creds my-pg-role
# {"username":"v-myapp-abc123","password":"A1b2C3...","lease_duration":"1h"}

# AWS
tuckcli aws creds my-s3-role
# {"access_key":"AKIA...","secret_key":"...","session_token":"...","lease_duration":"1h"}
```

Учётные данные истекают автоматически. Дополнительные скрипты ротации не требуются.

---

## 10. Выдача TLS-сертификатов через PKI

```sh
tuckcli pki issue my-role --cn=api.internal --ttl=720h --alt-name=api.svc.cluster.local
```

---

## 11. Справочная информация

| Задача | Источник |
|--------|----------|
| Полный справочник CLI | `tuckcli --help` |
| Справочник API | `https://127.0.0.1:8200/openapi.json` |
| Настройка HA-кластера | `docs/RUNBOOK.md` |
| Модель безопасности | `docs/THREAT_MODEL.md` |
| Участие в разработке | `CONTRIBUTING.md` |

---

## 12. Краткий справочник команд

```sh
# Сервер
tuck --config=tuck.yaml
tuck --seal-type=dev --tls-auto          # режим разработки

# Секреты
tuckcli kv put <путь> <значение>
tuckcli kv get <путь>
tuckcli kv delete <путь>
tuckcli kv list [префикс]

# Токены
tuckcli token create --policy=<политика> --ttl=24h
tuckcli token lookup-self
tuckcli token revoke <id>

# Динамические учётные данные
tuckcli db creds <роль>
tuckcli aws creds <роль>

# Криптографические операции
tuckcli transit encrypt <ключ> <открытый_текст>
tuckcli pki issue <роль> --cn=<имя>
tuckcli totp code <ключ>

# Аутентификация
tuckcli auth approle login --role-id=... --secret-id=...
```
