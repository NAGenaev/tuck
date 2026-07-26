# Руководство по участию в разработке

## 1. Общие положения

Настоящий документ устанавливает порядок участия во внешней разработке проекта Tuck. Принимаются сообщения об ошибках, запросы на добавление функциональности, улучшения документации и изменения программного кода.

---

## 2. Требования к среде разработки

### 2.1. Предварительные условия

- Go 1.25+
- Node.js 22+ (для сборки веб-панели в `web/`)
- Docker Desktop (для интеграционных тестов с Minikube)
- `golangci-lint` v2 (конфиг `.golangci.yml` — формата v2, `golangci-lint` v1 его не поймёт): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`

### 2.2. Локальная сборка

```bash
git clone https://github.com/NAGenaev/tuck.git
cd tuck

# ОБЯЗАТЕЛЬНО первым шагом: internal/ui встраивает собранную веб-панель
# через //go:embed assets, а internal/ui/assets/ не хранится в git (только
# .gitkeep-заглушка). Без этого шага ЛЮБАЯ go-команда — go build ./...,
# go test ./..., golangci-lint — падает с "pattern assets: contains no
# embeddable files". vite.config.ts сам кладёт сборку в internal/ui/assets.
cd web && npm ci && npm run build && cd ..

# Сборка всех компонентов
go build ./...

# Запуск тестов
go test ./...

# Сборка бинарного файла сервера
go build -o bin/tuck ./cmd/tuck

# Запуск сервера в режиме разработки (dev-seal, распечатан сразу, порт 8200 — всё значения по умолчанию)
./bin/tuck

# Для разработки самой веб-панели с hot-reload — отдельный dev-сервер
# (порт 3333, проксирует /v1, /v2 и т.д. на :8200), не нужен для go build:
cd web && npm run dev
```

### 2.3. Структура проекта

```
cmd/tuck/             Бинарный файл сервера
cmd/tuck-operator/    Бинарный файл оператора Kubernetes (CRD-синхронизация TuckSecret)
cmd/tuckcsi/          Бинарный файл CSI-драйвера (монтирование секретов в под)
cmd/tuck-injector/    Webhook-инжектор секретов в под как sidecar
cmd/tuck-agent/       Агент для внешних (не-k8s) процессов
cmd/tuckcli/          CLI-клиент
internal/
  api/                HTTP-обработчики и маршрутизация
  audit/              Журнал аудита с цепочкой хэшей
  auth/               Методы аутентификации (AppRole, LDAP, JWT/OIDC, GitHub)
  barrier/            Криптографический барьер AES-256-GCM
  core/               Бизнес-логика (секреты, токены, политики)
  csi/                Реализация CSI Node-плагина
  cubbyhole/          Изолированное хранилище на токен
  dynamic/            Движки динамических секретов: database, aws, gcp, azure, pki, ssh, totp, transit
  identity/           Сущности/группы identity-модели
  injector/           Логика webhook-инжектора
  k8s/                Аутентификация Kubernetes TokenReview
  kvsecret/           KV v1
  kvv2/               KV v2 (версионирование)
  lease/              Общий реестр lease для динамических секретов
  metrics/            Метрики Prometheus
  mount/              Управление точками монтирования движков
  namespace/          Мультитенантные namespace
  operator/           CRD-контроллер и выбор лидера
  physical/           Физические бэкенды хранения (bbolt, in-memory, raft)
  plugin/             Каталог внешних плагинов
  policy/             ACL на основе glob-масок путей
  ratelimit/          Token bucket на IP-адрес/токен
  replication/        Primary/secondary репликация и WAL
  seal/               Бэкенды снятия печати (dev, shamir, transit, kms)
  shamir/             Разделение секрета в GF(256)
  sysconfig/          Системная конфигурация (rate-limit и др.), сохраняемая через API
  tlsutil/            Вспомогательные функции TLS
  token/              Хранилище токенов
  ui/                 Встроенная сборка веб-панели (embed)
  wrapping/           Response wrapping (одноразовая передача секрета)
pkg/client/           Go SDK
contrib/terraform-provider-tuck/  Terraform-провайдер
web/                  Веб-панель (React/Vite/Mantine, см. docs/10-web-ui.md)
deploy/               Kubernetes-манифесты, Helm-чарт, CRD
docs/                 Архитектура, модель угроз, эксплуатация, гайды
```

---

## 3. Порядок оформления вкладов

### 3.1. Сообщения об ошибках

Используется шаблон [отчёта об ошибке](.github/ISSUE_TEMPLATE/bug_report.yml). В сообщении указываются:

- версия системы (`tuck --version`);
- шаги воспроизведения;
- ожидаемое и фактическое поведение;
- соответствующие записи журнала (токены и значения секретов должны быть удалены).

### 3.2. Запросы на добавление функциональности

Используется шаблон [запроса функциональности](.github/ISSUE_TEMPLATE/feature_request.yml). Перед оформлением запроса рекомендуется ознакомиться с [дорожной картой](docs/ROADMAP.md).

### 3.3. Pull Requests

1. Выполнить форк репозитория и создать ветку функциональности: `git checkout -b feat/my-change`
2. Написать тесты для нового поведения. Целевое покрытие для пакетов `crypto`/`auth` — не менее 70%.
3. Выполнить полный прогон тестов до открытия PR:
   ```bash
   go test ./...
   golangci-lint run ./...
   go build ./...
   ```
4. Каждый PR должен содержать одно изменение или исправление.
5. В описании PR указывается ссылка на связанную задачу: `Closes #123`.

---

## 4. Стандарты оформления кода

### 4.1. Стиль сообщений коммитов

Применяется формат [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(barrier): add Rekey() for root-key rotation
fix(api): binary-safe KV response for non-UTF8 values
docs(threat-model): add bbolt exfiltration scenario
test(operator): add UpdateStatus mock to controller test
```

### 4.2. Требования к коду

- Соответствие требованиям `gofmt` и `goimports` (проверяется в CI)
- Отсутствие неиспользуемых экспортируемых идентификаторов во внутренних пакетах
- Директивы `//nolint` допускаются только с поясняющим комментарием
- Ключевой материал в тестовых данных не допускается; для ключевых файлов используется `t.TempDir()`

---

## 5. Вопросы безопасности

Публикация сведений об уязвимостях безопасности в открытых Issues не допускается. Процедура координированного раскрытия описана в [SECURITY.md](SECURITY.md).

---

## 6. Лицензионные условия

Внося вклад в проект, участник соглашается с тем, что его материалы распространяются на условиях [лицензии Apache-2.0](LICENSE).
