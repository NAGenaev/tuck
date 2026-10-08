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

# CLI-клиент
go build -o tuckcli.exe ./cmd/tuckcli

# Убедиться что работает (у tuck нет подкоманд, только флаги)
.\tuck.exe -h
.\tuckcli.exe version
```

Бинари `tuck.exe` (сервер) и `tuckcli.exe` (клиент) появятся в корне проекта.

---

## 3. Локальный запуск (dev-режим)

Dev-режим — самый быстрый старт: сервер сам распечатывается (ключ лежит открытым текстом в файле `tuck-dev-rootkey`), данные хранятся в файле `tuck.db` в текущей папке и переживают перезапуск, TLS самоподписанный. У `tuck` нет подкоманд и флага `-dev`: всё задаётся флагами (`tuck.exe -h`), YAML-файлом (`--config`) или переменными окружения.

### Запуск сервера

Откройте первое окно PowerShell:

```powershell
cd D:\Projects\tuck
.\tuck.exe -tls-auto
```

При **первом** старте в логе печатается блок:

```
ROOT TOKEN (shown once — store it securely):
tuck_<длинная строка>
```

**Скопируйте root token** — повторно он не показывается. Адрес по умолчанию `127.0.0.1:8200`, схема `https` (самоподписанный сертификат). Чистый старт: остановите сервер и удалите `tuck.db` и `tuck-dev-rootkey`.

### Проверка работы

Откройте второе окно PowerShell:

```powershell
$TOKEN = "tuck_<ваш токен>"

# Health check (-k: самоподписанный сертификат)
curl.exe -k -s https://127.0.0.1:8200/v1/health | ConvertFrom-Json

# Записать секрет
curl.exe -k -s -X PUT https://127.0.0.1:8200/v1/secret/test/hello `
  -H "X-Tuck-Token: $TOKEN" `
  -H "Content-Type: text/plain" `
  -d "world"

# Прочитать секрет
curl.exe -k -s https://127.0.0.1:8200/v1/secret/test/hello `
  -H "X-Tuck-Token: $TOKEN" | ConvertFrom-Json
```

### Запуск с сохранением данных (production-режим)

Для продакшна печать секретов по Shamir, свой каталог данных и YAML-конфиг; подробно в [04 — Shamir](04-prod-shamir.md). Коротко:

```powershell
# Первый старт: в логе ROOT TOKEN и 5 SHAMIR SHARES (показываются один раз)
.\tuck.exe -seal-type shamir -data D:\tuck-data\tuck.db -tls-auto -addr 127.0.0.1:8200

# После каждого перезапуска нужны любые 3 доли (k из n)
.\tuckcli.exe unseal <share>
```

Или одним файлом: `.\tuck.exe --config tuck.yaml` (поля `addr`, `data`, `tls`, `seal`, ...; см. [25 — Пилот](25-pilot-deployment.md)).

---

## 4. Web UI — управление секретами через браузер

Tuck включает встроенный веб-интерфейс для управления секретами без CLI.

### Открыть UI

Пока сервер запущен (шаг 3), откройте в браузере:

```
https://127.0.0.1:8200/ui/
```

Браузер предупредит о самоподписанном сертификате (dev-режим): подтвердите исключение.

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
| Namespaces | мультитенантность |

---

## 5. CLI — базовые команды

CLI это `tuckcli.exe`; `tuck.exe` это только сервер.

```powershell
# Переменные окружения (чтобы не вводить каждый раз)
$env:TUCK_ADDR     = "https://127.0.0.1:8200"
$env:TUCK_TOKEN    = "tuck_<ваш токен>"
$env:TUCK_INSECURE = "1"   # только для самоподписанного dev-сертификата

# Секреты (KV v1); путь указывается относительно mount secret/
.\tuckcli.exe kv put db/password "supersecret"
.\tuckcli.exe kv get db/password
.\tuckcli.exe kv list db/
.\tuckcli.exe kv delete db/password

# Токены
.\tuckcli.exe token create --name=demo --policy=app-readonly --ttl=24h
.\tuckcli.exe token get <token-id>
.\tuckcli.exe token revoke <token-id>

# Политики: JSON-массив правил, из stdin (PS 5.1 портит кавычки в аргументах)
'[{"path":"secret/prod/*","capabilities":["read"]}]' | .\tuckcli.exe policy put prod-ro -
.\tuckcli.exe policy get prod-ro

# Identity
.\tuckcli.exe identity entity create --name=app-service
.\tuckcli.exe identity group create --name=ops-team
```

`tuckcli` работает только с KV v1. Версионированный KV v2 доступен по HTTP API, см. [03 — KV v2](03-kvv2.md). Полный список команд: `tuckcli.exe` без аргументов.

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
# Токен печатается в логе при первом старте (строка после заголовка)
kubectl -n tuck logs deployment/tuck-server | Select-String "ROOT TOKEN" -Context 0,1

# Сохранить для остальных команд (скрипты scripts\*.ps1 читают этот файл)
"tuck_<токен>" | Set-Content testdata\minikube-root-token.txt
```

Файл `testdata\minikube-root-token.txt` в git не хранится (создаётся скриптом `scripts\minikube-lab.ps1` или вручную).

### Port-forward к Tuck в minikube

```powershell
# В отдельном окне PowerShell (оставить запущенным)
kubectl -n tuck port-forward svc/tuck-server 8201:8200
```

### Проверить что Tuck работает в k8s

```powershell
$K8S_TOKEN = Get-Content testdata\minikube-root-token.txt

# Health
curl.exe -s http://127.0.0.1:8201/v1/health | ConvertFrom-Json

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

Token: root token из лога сервера (см. выше)

### TuckSecret — синхронизация секретов в K8s

Создать CR для синхронизации секрета из Tuck в K8s Secret. Оператор логинится в Tuck через ServiceAccount, поэтому сначала нужна роль (политика `root` только для локального стенда):

```powershell
$env:TUCK_ADDR  = "http://127.0.0.1:8201"
$env:TUCK_TOKEN = Get-Content testdata\minikube-root-token.txt
.\tuckcli.exe auth kubernetes role put tuck tuck-operator --policies=root --ttl=5m
```

```yaml
# my-secret.yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: demo-app-credentials
  namespace: tuck
spec:
  tuckPath: demo-app/credentials        # путь в KV, без префикса secret/
  secretName: demo-app-credentials      # имя K8s Secret (в namespace CR)
  secretKey: value                      # ключ внутри Secret
  refreshInterval: "30s"
```

```powershell
# Записать секрет в Tuck
curl.exe -s -X PUT http://127.0.0.1:8201/v1/secret/demo-app/credentials `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: text/plain" `
  -d "my-secret-value"

# Применить CR
kubectl apply -f my-secret.yaml

# Убедиться что K8s Secret создан и прочитать значение
$b64 = kubectl -n tuck get secret demo-app-credentials -o jsonpath='{.data.value}'
[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($b64))
```

Подробнее про поля TuckSecret: [07 — оператор](07-tucksecret-operator.md).

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

Ожидаемый результат: `0 failed`; итоги в `RESULTS.md`.

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

Ctrl+C в окне где запущен `tuck.exe`.

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

Собрать бинари: `go build -o tuck.exe ./cmd/tuck` и `go build -o tuckcli.exe ./cmd/tuckcli`

### `connection refused` на порту 8200 / 8201

- 8200: убедитесь что `tuck.exe -tls-auto` запущен (адрес `https://127.0.0.1:8200`)
- 8201: убедитесь что `kubectl port-forward svc/tuck-server 8201:8200` запущен в отдельном окне

### UI: Invalid token после входа

Токен невалиден или истёк. Используйте root token из лога сервера при первом старте (повторно не показывается; при потере сбросьте `tuck.db` и `tuck-dev-rootkey` в dev-режиме).

### Образ в minikube не обновился (старая версия)

```powershell
minikube ssh "docker rmi -f tuck-server:local"
minikube image load tuck-server:local
kubectl -n tuck rollout restart deployment/tuck-server
```

### Root token в minikube не работает

После пересоздания PVC (minikube delete + start) токен меняется. Получить актуальный:

```powershell
kubectl -n tuck logs deployment/tuck-server | Select-String "ROOT TOKEN" -Context 0,1
```

И обновить файл:

```powershell
# Сохранить новый токен
"tuck_<новый токен>" | Set-Content testdata\minikube-root-token.txt
```

### Тест 9b (TuckSecret) падает

Оператор потерял роль K8s auth. Исправить:

```powershell
$env:TUCK_ADDR  = "http://127.0.0.1:8201"
$env:TUCK_TOKEN = Get-Content testdata\minikube-root-token.txt
.\tuckcli.exe auth kubernetes role put tuck tuck-operator --policies=root --ttl=5m
```

`root` здесь — только для локального стенда. В продакшне дайте роли узкую политику и включите `--enforce-namespace-paths` (см. [07](07-tucksecret-operator.md)).

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
