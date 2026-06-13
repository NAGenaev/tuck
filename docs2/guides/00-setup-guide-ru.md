# Tuck — полное руководство по установке и запуску

[← Все гайды](README.md)

Это главный гайд: от нуля до работающего Tuck на локалке и в Kubernetes.  
Все команды для Windows PowerShell. Результат: локальный сервер + minikube-лаб + прогон интеграционных тестов.

---

## Содержание

1. [Предварительные условия](#1-предварительные-условия)
2. [Сборка бинаря](#2-сборка-бинаря)
3. [Локальный запуск (dev-режим)](#3-локальный-запуск-dev-режим)
4. [Web UI — управление секретами через браузер](#4-web-ui--управление-секретами-через-браузер)
5. [CLI — базовые команды](#5-cli--базовые-команды)
6. [Minikube — запуск в Kubernetes](#6-minikube--запуск-в-kubernetes)
7. [Интеграционные тесты](#7-интеграционные-тесты)
8. [Остановка и очистка](#8-остановка-и-очистка)
9. [Частые ошибки](#9-частые-ошибки)

---

## 1. Предварительные условия

### Обязательные инструменты

| Инструмент | Зачем | Проверить |
|---|---|---|
| Go 1.25+ | сборка Tuck | `go version` |
| Git | клонирование репо | `git --version` |

### Для minikube-лаба

| Инструмент | Зачем | Проверить |
|---|---|---|
| Docker Desktop | образы и minikube driver | `docker version` |
| minikube | локальный Kubernetes | `minikube version` |
| kubectl | управление кластером | `kubectl version --client` |
| helm | установка Tuck chart | `helm version` |
| curl.exe | HTTP-запросы в тестах | `curl --version` |

### Установка (если чего-то нет)

```powershell
# Go — скачать с https://go.dev/dl/ и установить (Windows MSI)

# Docker Desktop — https://www.docker.com/products/docker-desktop/

# minikube
winget install Kubernetes.minikube

# kubectl
winget install Kubernetes.kubectl

# helm
winget install Helm.Helm
```

После установки перезапустите PowerShell и проверьте все версии.

---

## 2. Сборка бинаря

```powershell
cd D:\Projects\tuck

# Сборка
go build -o tuck.exe ./cmd/tuck

# Убедиться что работает
.\tuck.exe --help
```

Бинарь `tuck.exe` появится в корне проекта.

---

## 3. Локальный запуск (dev-режим)

Dev-режим — самый быстрый старт: хранит данные в памяти, не требует TLS и запечатывания.

### Запуск сервера

Откройте первое окно PowerShell:

```powershell
cd D:\Projects\tuck
.\tuck.exe server -dev
```

Вы увидите:

```
Tuck server started
Mode:      dev
Address:   http://127.0.0.1:8200
Root token: tuck_<длинная строка>
```

**Скопируйте root token** — он нужен для всех операций.

### Проверка работы

Откройте второе окно PowerShell:

```powershell
$TOKEN = "tuck_<ваш токен>"

# Health check
curl.exe -s http://127.0.0.1:8200/v1/sys/health | ConvertFrom-Json

# Записать секрет
curl.exe -s -X PUT http://127.0.0.1:8200/v1/secret/test/hello `
  -H "X-Tuck-Token: $TOKEN" `
  -H "Content-Type: text/plain" `
  -d "world"

# Прочитать секрет
curl.exe -s http://127.0.0.1:8200/v1/secret/test/hello `
  -H "X-Tuck-Token: $TOKEN" | ConvertFrom-Json
```

### Запуск с сохранением данных (production-режим)

```powershell
# Инициализация (один раз)
.\tuck.exe server init --data-dir D:\tuck-data

# Сервер сохраняет данные на диск
.\tuck.exe server --data-dir D:\tuck-data --addr 127.0.0.1:8200
```

После перезапуска сервера в production-режиме потребуется **unseal** ключами из init.

---

## 4. Web UI — управление секретами через браузер

Tuck включает встроенный веб-интерфейс для управления секретами без CLI.

### Открыть UI

Пока сервер запущен (шаг 3), откройте в браузере:

```
http://127.0.0.1:8200/ui/
```

Или для minikube (после port-forward, см. шаг 6):

```
http://127.0.0.1:8201/ui/
```

### Вход

1. Введите root token в поле **Token**
2. Нажмите **Sign In**
3. Токен сохраняется в `localStorage` — после закрытия браузера входить снова не нужно

### KV v1 — Secrets Explorer

- Вкладка **Secrets** — файловый проводник по секретам
- Клик на папку (📁) — переход внутрь
- Клик на ключ (🔑) — кнопка **View** показывает значение (маскированное ••••••••)
- Кнопка **👁 Reveal** — раскрыть значение
- Кнопка **📋 Copy** — скопировать в буфер
- Кнопка **Del** — удалить с подтверждением
- Навигация через breadcrumb (🏠 root › folder › subfolder)
- Форма внизу страницы — записать новый секрет

### KV v2 — версионированные секреты

- Вкладка **KV v2** — те же возможности + версии
- В карточке секрета можно выбрать версию
- Кнопка **Show Metadata** — история версий

### Остальные разделы

| Вкладка | Назначение |
|---|---|
| Tokens | создание, просмотр, отзыв токенов |
| Policies | ACL-политики |
| Auth | AppRole, JWT, LDAP, K8s auth |
| Leases | аренды динамических секретов |
| Namespaces | мультитенантность |

---

## 5. CLI — базовые команды

```powershell
# Переменные окружения (чтобы не вводить каждый раз)
$env:TUCK_ADDR  = "http://127.0.0.1:8200"
$env:TUCK_TOKEN = "tuck_<ваш токен>"

# Секреты (KV v1)
.\tuck.exe kv put secret/db/password "supersecret"
.\tuck.exe kv get secret/db/password
.\tuck.exe kv list secret/db/
.\tuck.exe kv delete secret/db/password

# Секреты (KV v2)
.\tuck.exe kv put v2/secret/app/config "production"
.\tuck.exe kv get v2/secret/app/config
.\tuck.exe kv get v2/secret/app/config --version 1

# Токены
.\tuck.exe token create --policy default --ttl 24h
.\tuck.exe token lookup <token-id>
.\tuck.exe token revoke <token-id>

# Политики
.\tuck.exe policy write prod-ro - <<'EOF'
path "secret/prod/*" { capabilities = ["read"] }
EOF
.\tuck.exe policy read prod-ro

# Identity
.\tuck.exe identity entity create --name app-service
.\tuck.exe identity group create --name ops-team
```

---

## 6. Minikube — запуск в Kubernetes

### Первый запуск (setup с нуля)

```powershell
cd D:\Projects\tuck

# Запустить minikube (если ещё не запущен)
minikube start --driver=docker --memory=2048 --cpus=2

# Собрать Docker-образы Tuck
docker build -f build/Dockerfile.server  -t tuck-server:local  .
docker build -f build/Dockerfile.operator -t tuck-operator:local .

# Загрузить образы в minikube
minikube image load tuck-server:local
minikube image load tuck-operator:local

# Установить Tuck через Helm
helm install tuck deploy/helm/tuck `
  -n tuck --create-namespace `
  -f deploy/helm/tuck/values-minikube.yaml

# Подождать готовности
kubectl -n tuck rollout status deployment/tuck-server --timeout=120s
kubectl -n tuck rollout status deployment/tuck-operator --timeout=120s
kubectl -n tuck get pods
```

### Получить root token

```powershell
# Токен хранится в файле (создаётся при первом init)
Get-Content testdata\minikube-root-token.txt

# Если файла нет — достать из логов pod'а
kubectl -n tuck logs deployment/tuck-server | Select-String "Root Token"
```

### Port-forward к Tuck в minikube

```powershell
# В отдельном окне PowerShell (оставить запущенным)
kubectl -n tuck port-forward svc/tuck 8201:8200
```

### Проверить что Tuck работает в k8s

```powershell
$K8S_TOKEN = Get-Content testdata\minikube-root-token.txt

# Health
curl.exe -s http://127.0.0.1:8201/v1/sys/health | ConvertFrom-Json

# Записать секрет
curl.exe -s -X PUT http://127.0.0.1:8201/v1/secret/k8s-test `
  -H "X-Tuck-Token: $K8S_TOKEN" `
  -H "Content-Type: text/plain" `
  -d "hello-from-k8s"

# Прочитать
curl.exe -s http://127.0.0.1:8201/v1/secret/k8s-test `
  -H "X-Tuck-Token: $K8S_TOKEN" | ConvertFrom-Json
```

### Web UI для minikube

```
http://127.0.0.1:8201/ui/
```

Token: содержимое `testdata\minikube-root-token.txt`

### TuckSecret — синхронизация секретов в K8s

Создать CR для синхронизации секрета из Tuck в K8s Secret:

```yaml
# my-secret.yaml
apiVersion: secrets.tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: demo-app-credentials
  namespace: tuck
spec:
  path: secret/demo-app/credentials
  targetName: demo-app-credentials
  targetNamespace: tuck
  refreshIntervalSeconds: 30
```

```powershell
# Записать секрет в Tuck
curl.exe -s -X PUT http://127.0.0.1:8201/v1/secret/demo-app/credentials `
  -H "X-Tuck-Token: $K8S_TOKEN" `
  -H "Content-Type: text/plain" `
  -d "my-secret-value"

# Применить CR
kubectl apply -f my-secret.yaml

# Убедиться что K8s Secret создан
kubectl -n tuck get secret demo-app-credentials -o jsonpath='{.data.value}' | `
  [System.Convert]::FromBase64String([System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String([System.Text.Encoding]::UTF8.GetString([System.Text.Encoding]::UTF8.GetBytes($_)))))
```

### Обновление образа в minikube (после изменений кода)

```powershell
cd D:\Projects\tuck

# Пересобрать
docker build -f build/Dockerfile.server -t tuck-server:local .

# ВАЖНО: удалить старый образ из minikube (иначе не обновится!)
minikube ssh "docker rmi -f tuck-server:local"

# Загрузить новый
minikube image load tuck-server:local

# Перезапустить pod
kubectl -n tuck rollout restart deployment/tuck-server
kubectl -n tuck rollout status deployment/tuck-server --timeout=120s
```

---

## 7. Интеграционные тесты

### Быстрый прогон (без unit-тестов)

Убедитесь что:
- `tuck.exe` собран (шаг 2)
- minikube запущен и port-forward на `8201` активен (шаг 6)

```powershell
cd D:\Projects\tuck

# Быстро (без go test, ~30 секунд)
.\scripts\run-lab-results.ps1 -SkipUnit

# Результат: lab-results\runs\<timestamp>\RESULTS.md
```

### Полный прогон (с unit-тестами)

```powershell
# Включает go test ./... (~2-3 минуты)
.\scripts\run-lab-results.ps1
```

### Полный прогон с пересборкой образа

```powershell
# Собирает Docker-образ и перезагружает в minikube
.\scripts\run-lab-results.ps1 -ReloadK8sImage
```

### Что проверяют тесты

| ID | Что проверяется | Окружение |
|---|---|---|
| BUILD | сборка tuck.exe | local |
| L-READY | health endpoint | local:8202 |
| 1 | health sealed=false | local |
| 2 | seal status dev | local |
| 3a–3d | KV put / get / 404 / delete | local |
| 4a–4b | auth без токена / с неверным | local |
| 5 | Policy ACL | local |
| 6 | token create | local |
| 7a–7b | manual seal + 503 при sealed | local |
| 10 | persist: данные после перезапуска | local |
| UI-LIST | GET ?list=true (browser-compat) | local |
| UI-KV2 / UI-KV2-LIST | KV v2 write + metadata list | local |
| K8S-PF | port-forward :8201 | minikube |
| K8S-1 | health в k8s | minikube |
| 8 | K8s ServiceAccount auth | minikube |
| 9a–9c | TuckSecret operator sync + CRD | minikube |
| UI-K8S-LIST | UI list в k8s | minikube |
| K8S-PODS / K8S-OP | pod'ы Running | minikube |
| CLUSTER / CRD | node Ready + CRD установлен | minikube |

Ожидаемый результат: **32 passed, 0 failed**.

### Просмотр результатов

```powershell
# Последний прогон
$run = Get-ChildItem lab-results\runs\ | Sort-Object Name | Select-Object -Last 1
Get-Content "$($run.FullName)\RESULTS.md"

# Ошибки (если есть)
Get-Content "$($run.FullName)\FAILURES.md"
```

---

## 8. Остановка и очистка

### Остановить локальный сервер

Ctrl+C в окне где запущен `tuck.exe server -dev`.

### Остановить port-forward

Ctrl+C в окне с `kubectl port-forward`.

### Остановить minikube

```powershell
# Пауза (сохраняет состояние)
minikube stop

# Полное удаление (данные потеряются, root token сбросится)
minikube delete
```

### Удалить Tuck из кластера

```powershell
helm -n tuck uninstall tuck
kubectl delete namespace tuck
```

---

## 9. Частые ошибки

### `tuck.exe: command not found`

Собрать бинарь: `go build -o tuck.exe ./cmd/tuck`

### `connection refused` на порту 8200 / 8201

- 8200: убедитесь что `tuck.exe server -dev` запущен
- 8201: убедитесь что `kubectl port-forward svc/tuck 8201:8200` запущен в отдельном окне

### UI: Invalid token после входа

Токен невалиден или истёк. Используйте root token из вывода `tuck.exe server -dev` или из `testdata\minikube-root-token.txt`.

### Образ в minikube не обновился (старая версия)

```powershell
minikube ssh "docker rmi -f tuck-server:local"
minikube image load tuck-server:local
kubectl -n tuck rollout restart deployment/tuck-server
```

### Root token в minikube не работает

После пересоздания PVC (minikube delete + start) токен меняется. Получить актуальный:

```powershell
kubectl -n tuck logs deployment/tuck-server | Select-String "Root Token"
```

И обновить файл:

```powershell
# Сохранить новый токен
"tuck_<новый токен>" | Set-Content testdata\minikube-root-token.txt
```

### Тест 9b (TuckSecret) падает

Оператор потерял роль K8s auth. Исправить:

```powershell
$K8S_TOKEN = Get-Content testdata\minikube-root-token.txt

curl.exe -s -X PUT http://127.0.0.1:8201/v1/auth/kubernetes/role/tuck/tuck-operator `
  -H "X-Tuck-Token: $K8S_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"bound_service_account_names":["tuck-operator"],"bound_service_account_namespaces":["tuck"],"policies":["root"]}'
```

### `go test` flaky (port conflict)

```powershell
# Запускать с -p 1 (без параллельного запуска)
go test -p 1 ./...
```

---

## Ссылки

- [Остальные гайды](README.md)
- [01 — Первый секрет](01-pervyy-sekret.md)
- [06 — Kubernetes + Helm](06-kubernetes-helm.md)
- [07 — TuckSecret оператор](07-tucksecret-operator.md)
- [09 — Kubernetes auth](09-kubernetes-auth.md)
- [23 — Web UI](23-web-ui.md)
