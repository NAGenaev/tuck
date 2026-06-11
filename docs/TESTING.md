# Tuck — тест-кейсы и сценарии

## Требования

- Windows + Docker Desktop
- Go 1.22+
- `curl` (в PowerShell или Git Bash)
- Опционально: minikube для k8s-сценариев

---

## Часть 1: Локальный запуск (dev seal)

### Сборка

```powershell
cd D:\Projects\tuck

# Сборка сервера
go build -o tuck.exe ./cmd/tuck

# Сборка оператора
go build -o tuck-operator.exe ./cmd/tuck-operator
```

### Запуск сервера (dev режим)

```powershell
.\tuck.exe --addr=127.0.0.1:8200 --data=tuck.db --dev-seal-key=tuck-rootkey.bin
```

В логах появится:
```
ROOT TOKEN (shown once — store it securely):
  tuck_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
tuck: unsealed (dev seal), serving on http://127.0.0.1:8200
```

**Сохрани root token** — он нужен для всех следующих запросов.

```powershell
# Удобно сохранить в переменную (PowerShell)
$TUCK_TOKEN = "tuck_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
$TUCK_ADDR  = "http://127.0.0.1:8200"
```

---

## Тест 1: Health check

```powershell
curl $TUCK_ADDR/v1/health
```

**Ожидаемый ответ:**
```json
{"sealed":false}
```

---

## Тест 2: Seal status

```powershell
curl $TUCK_ADDR/v1/sys/seal-status
```

**Ожидаемый ответ:**
```json
{"sealed":false,"type":"dev"}
```

---

## Тест 3: Базовые операции с секретами

### Записать секрет

```powershell
curl -X PUT $TUCK_ADDR/v1/secret/db/password `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -d "supersecret123"
```
**Ожидаемый ответ:** `204 No Content`

### Прочитать секрет

```powershell
curl $TUCK_ADDR/v1/secret/db/password `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:**
```json
{"path":"db/password","value":"supersecret123"}
```

### Несуществующий секрет

```powershell
curl $TUCK_ADDR/v1/secret/db/nonexistent `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `404 Not Found`

### Удалить секрет

```powershell
curl -X DELETE $TUCK_ADDR/v1/secret/db/password `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `204 No Content`

### Проверить что удалён

```powershell
curl $TUCK_ADDR/v1/secret/db/password `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `404 Not Found`

---

## Тест 4: Аутентификация

### Запрос без токена

```powershell
curl $TUCK_ADDR/v1/secret/anything
```
**Ожидаемый ответ:** `401 Unauthorized`

### Запрос с неверным токеном

```powershell
curl $TUCK_ADDR/v1/secret/anything `
  -H "X-Tuck-Token: tuck_invalid"
```
**Ожидаемый ответ:** `401 Unauthorized`

---

## Тест 5: Политики и ACL

### Создать политику (только read для secret/prod/*)

```powershell
curl -X PUT $TUCK_ADDR/v1/policy/prod-readonly `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"rules":[{"path":"secret/prod/*","capabilities":["read"]}]}'
```
**Ожидаемый ответ:** `204 No Content`

### Прочитать политику

```powershell
curl $TUCK_ADDR/v1/policy/prod-readonly `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:**
```json
{"name":"prod-readonly","rules":[{"path":"secret/prod/*","capabilities":["read"]}]}
```

### Создать ограниченный токен

```powershell
curl -X POST $TUCK_ADDR/v1/auth/token `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"display_name":"prod-reader","policies":["prod-readonly"]}'
```
**Ожидаемый ответ:** `201 Created`
```json
{"id":"tuck_...","display_name":"prod-reader","policies":["prod-readonly"],...}
```

```powershell
# Сохранить ID ограниченного токена
$LIMITED_TOKEN = "tuck_..."
```

### Seed секрета root-токеном

```powershell
curl -X PUT $TUCK_ADDR/v1/secret/prod/api-key `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -d "prod-api-key-value"
```

### Ограниченный токен МОЖЕТ читать secret/prod/*

```powershell
curl $TUCK_ADDR/v1/secret/prod/api-key `
  -H "X-Tuck-Token: $LIMITED_TOKEN"
```
**Ожидаемый ответ:** `200` с value

### Ограниченный токен НЕ МОЖЕТ писать

```powershell
curl -X PUT $TUCK_ADDR/v1/secret/prod/new-key `
  -H "X-Tuck-Token: $LIMITED_TOKEN" `
  -d "val"
```
**Ожидаемый ответ:** `403 Forbidden`

### Ограниченный токен НЕ МОЖЕТ читать secret/staging/*

```powershell
curl $TUCK_ADDR/v1/secret/staging/key `
  -H "X-Tuck-Token: $LIMITED_TOKEN"
```
**Ожидаемый ответ:** `403 Forbidden`

---

## Тест 6: Управление токенами

### Создать токен с TTL

```powershell
curl -X POST $TUCK_ADDR/v1/auth/token `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"display_name":"temp-token","policies":[],"ttl":"1h"}'
```

### Посмотреть токен

```powershell
$TEMP_TOKEN_ID = "tuck_..."   # ID из предыдущего ответа
curl $TUCK_ADDR/v1/auth/token/$TEMP_TOKEN_ID `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```

### Отозвать токен

```powershell
curl -X DELETE $TUCK_ADDR/v1/auth/token/$TEMP_TOKEN_ID `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `204 No Content`

### Использовать отозванный токен

```powershell
curl $TUCK_ADDR/v1/secret/anything `
  -H "X-Tuck-Token: $TEMP_TOKEN_ID"
```
**Ожидаемый ответ:** `401 Unauthorized`

---

## Тест 7: Seal / Unseal (dev seal)

### Запечатать вручную

```powershell
curl -X POST $TUCK_ADDR/v1/sys/seal `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `200 OK` + `{"sealed":true}`

### Проверить статус

```powershell
curl $TUCK_ADDR/v1/sys/seal-status
```
**Ожидаемый ответ:** `{"sealed":true,"type":"dev"}`

### Запрос к запечатанному серверу

```powershell
curl $TUCK_ADDR/v1/secret/anything `
  -H "X-Tuck-Token: $TUCK_TOKEN"
```
**Ожидаемый ответ:** `503 Service Unavailable`

### Dev seal: нельзя unseal через API (только рестарт)

```powershell
curl -X POST $TUCK_ADDR/v1/sys/unseal `
  -H "Content-Type: application/json" `
  -d '{"key":"anything"}'
```
**Ожидаемый ответ:** `400 Bad Request` ("seal does not support interactive unseal")

### Рестарт сервера — auto-unseal

Остановить `tuck.exe` (Ctrl+C) и запустить снова с теми же флагами.
Сервер должен подняться unsealed без вывода root token
(root token уже сохранён в bbolt, повторно не генерируется).

---

## Часть 2: Shamir seal

### Запуск с Shamir seal (3-of-5)

Для тестирования нужно добавить флаги в main.go — или запустить тест напрямую.

Пока в `cmd/tuck/main.go` хардкодирован `seal.NewDev(...)`.
Протестировать Shamir можно через unit тесты:

```powershell
go test ./internal/seal/... -v -run TestShamir
go test ./internal/api/... -v -run TestSys
```

**Ожидаемый вывод для `TestSysShamirUnseal`:**
```
--- PASS: TestSysShamirUnseal (0.XXs)
```

Для полного теста через HTTP запустить сервер с Shamir потребуется M5 (CLI/конфигурация).
Пока Shamir протестирован через integration tests в `internal/api/sys_test.go`.

---

## Часть 3: Kubernetes (minikube + Docker Desktop)

### Подготовка окружения

```powershell
# Установить minikube (если нет)
winget install Kubernetes.minikube

# Запустить minikube на Docker Desktop
minikube start --driver=docker --cpus=2 --memory=2048

# Проверить
kubectl get nodes
# NAME       STATUS   ROLES           AGE   VERSION
# minikube   Ready    control-plane   Xm    v1.X.X
```

### Сборка Docker-образов

Образы: `build/Dockerfile.server`, `build/Dockerfile.operator`.

```powershell
docker build -f build/Dockerfile.server -t tuck-server:local .
docker build -f build/Dockerfile.operator -t tuck-operator:local .

minikube image load tuck-server:local
minikube image load tuck-operator:local
```

> Краткий гайд: [MINIKUBE.md](MINIKUBE.md)

### Деплой Tuck-сервера в k8s

Манифесты: `deploy/server/`

```powershell
kubectl apply -f deploy/server/

# Ждать готовности
kubectl -n tuck rollout status deployment/tuck-server

# Получить root token из логов
kubectl -n tuck logs deployment/tuck-server
# Искать строку: ROOT TOKEN (shown once — store it securely):
```

### Проброс порта для тестирования

```powershell
# Временный port-forward (держать открытым в отдельном окне)
kubectl -n tuck port-forward svc/tuck 8200:8200

# В другом окне
$TUCK_ADDR = "http://127.0.0.1:8200"
curl $TUCK_ADDR/v1/health
```

---

## Тест 8: Kubernetes ServiceAccount auth

### Создать тестовый ServiceAccount

```yaml
# test-sa.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-app
  namespace: default
```

```powershell
kubectl apply -f test-sa.yaml
```

### Получить SA токен

```powershell
# K8s 1.24+ — создать временный токен
$SA_TOKEN = kubectl create token test-app --duration=1h
```

### Зарегистрировать role binding в Tuck

```powershell
# Создать политику
curl -X PUT $TUCK_ADDR/v1/policy/app-policy `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"rules":[{"path":"secret/app/*","capabilities":["read"]}]}'

# Привязать SA к политике
curl -X PUT "$TUCK_ADDR/v1/auth/kubernetes/role/default/test-app" `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["app-policy"],"ttl":"1h"}'
```

### Залогиниться через SA токен

```powershell
curl -X POST $TUCK_ADDR/v1/auth/kubernetes/login `
  -H "Content-Type: application/json" `
  -d "{`"token`":`"$SA_TOKEN`"}"
```

**Ожидаемый ответ:**
```json
{"token":"tuck_..."}
```

```powershell
$APP_TOKEN = "tuck_..."

# Прочитать секрет с токеном приложения
curl -X PUT $TUCK_ADDR/v1/secret/app/config `
  -H "X-Tuck-Token: $TUCK_TOKEN" -d "app-config-value"

curl $TUCK_ADDR/v1/secret/app/config `
  -H "X-Tuck-Token: $APP_TOKEN"
# => {"path":"app/config","value":"app-config-value"}
```

---

## Тест 9: TuckSecret CRD + оператор

### Применить CRD

```powershell
kubectl apply -f deploy/crd/
kubectl get crd tucksecrets.tuck.io
```

### Создать role binding для оператора

```powershell
# Оператор запускается от ServiceAccount tuck-operator (namespace tuck)
curl -X PUT "$TUCK_ADDR/v1/auth/kubernetes/role/tuck/tuck-operator" `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["root"],"ttl":"5m"}'
```

> В production оператор должен иметь ограниченную политику (только read нужных путей).
> Для теста используем root политику.

### Запустить оператор

```powershell
kubectl apply -f deploy/operator/deployment.yaml
kubectl apply -f deploy/operator/local.yaml   # локальные образы + http://tuck
kubectl -n tuck rollout status deployment/tuck-operator
```

### Записать секрет в Tuck

```powershell
curl -X PUT $TUCK_ADDR/v1/secret/myapp/db-password `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -d "postgres-secret-xyz"
```

### Создать TuckSecret ресурс

```yaml
# test-tucksecret.yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: default
spec:
  tuckPath: myapp/db-password
  secretName: myapp-db-credentials
  secretKey: password
  refreshInterval: "30s"
```

```powershell
kubectl apply -f test-tucksecret.yaml

# Ждать синхронизации (30 сек)
kubectl get tucksecret db-password -o wide
# TUCKPATH              SECRETNAME            LASTSYNCED
# myapp/db-password     myapp-db-credentials  2024-...
```

### Проверить созданный K8s Secret

```powershell
kubectl get secret myapp-db-credentials -o jsonpath='{.data.password}' | `
  ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($_)) }
# => postgres-secret-xyz
```

### Проверить синхронизацию при изменении

```powershell
# Обновить секрет в Tuck
curl -X PUT $TUCK_ADDR/v1/secret/myapp/db-password `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -d "new-postgres-secret-abc"

# Подождать refreshInterval (30 сек), затем:
kubectl get secret myapp-db-credentials -o jsonpath='{.data.password}' | `
  ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($_)) }
# => new-postgres-secret-abc
```

---

## Тест 10: Рестарт с сохранением данных

```powershell
# 1. Записать секрет
curl -X PUT $TUCK_ADDR/v1/secret/persist-test `
  -H "X-Tuck-Token: $TUCK_TOKEN" -d "should-survive-restart"

# 2. Остановить tuck.exe (Ctrl+C)
# 3. Запустить снова с теми же флагами (тот же tuck.db и rootkey.bin)
.\tuck.exe --addr=127.0.0.1:8200 --data=tuck.db --dev-seal-key=tuck-rootkey.bin

# В логах НЕТ "ROOT TOKEN" — сервер поднялся с существующей БД
# 4. Прочитать секрет
curl $TUCK_ADDR/v1/secret/persist-test `
  -H "X-Tuck-Token: $TUCK_TOKEN"
# => {"path":"persist-test","value":"should-survive-restart"}
```

---

## Unit тесты

```powershell
# Все тесты
go test ./... -v

# Только Shamir математика
go test ./internal/shamir/... -v

# Только API (включая sys/unseal flow)
go test ./internal/api/... -v

# Только seal пакет (ShamirSeal, TransitSeal)
go test ./internal/seal/... -v

# С покрытием
go test ./... -cover
```

---

## Автоматический прогон

```powershell
.\scripts\run-all-tests.ps1
```

Результаты сохраняются в [TEST_RESULTS.md](TEST_RESULTS.md).

Для minikube root token (после первого деплоя, если в логах уже нет):

```powershell
kubectl -n tuck logs deployment/tuck-server | Select-String "tuck_"
# сохранить в testdata/minikube-root-token.txt (файл в .gitignore)
```

---

## Чеклист готовности к тестированию

- [x] `go build ./...` без ошибок
- [x] `go test ./...` — все PASS
- [x] Тест 1-7: локальный сервер (dev seal) — PASS 2026-06-11
- [x] Shamir: unit + TestSysShamirUnseal — PASS 2026-06-11
- [x] Тест 8: k8s SA auth (minikube) — PASS 2026-06-11
- [x] Тест 9: TuckSecret CRD (minikube + оператор) — PASS 2026-06-11
- [x] Тест 10: персистентность (локально + pod restart) — PASS 2026-06-11

Полный отчёт: [TEST_RESULTS.md](TEST_RESULTS.md) — **37/37 PASS**
