# Tuck — план до Production Ready

> Дорожная карта от текущего состояния (M0–M4 готовы) до GA-релиза **v1.0**,
> пригодного к эксплуатации в продакшене.
> Менеджер секретов — это корень доверия инфраструктуры, поэтому планка
> намеренно высокая: безопасность и операционная надёжность важнее фич.

**Как читать:** задачи сгруппированы по фазам и имеют ID (`SEC-1`, `OPS-2`, …),
приоритет и оценку. Приоритеты:

- **P0** — блокер: без этого нельзя ставить в прод вообще.
- **P1** — обязательно для GA v1.0.
- **P2** — желательно к v1.0 / можно в v1.x.

Оценки: **S** ≈ 0.5–1 день, **M** ≈ 2–4 дня, **L** ≈ 1–2 недели, **XL** ≈ 3+ недели.

---

## 1. Текущее состояние (что уже есть)

| Слой | Готово |
|---|---|
| Крипто-ядро (`barrier`) | ✅ envelope encryption, AES-256-GCM, seal/unseal |
| Хранилище (`physical`) | ✅ bbolt (single file) + in-memory |
| Seal-стратегии | ✅ dev / Shamir SSS / Transit (Vault-совместимый) |
| Identity | ✅ bearer-токены, TTL, root-токен |
| ACL | ✅ path-glob политики, capabilities |
| K8s auth | ✅ TokenReview, role-bindings, короткоживущие токены |
| Оператор | ✅ `TuckSecret` CRD, list+watch+refresh sync |
| HTTP API | ✅ secret / token / policy / k8s / sys |
| Деплой | ✅ манифесты server/operator/CRD/RBAC, Dockerfiles |
| Тесты | ✅ unit + integration, 37/37 e2e на minikube |

**Чего нет (укрупнённо):** TLS, audit-лог, защита токенов в хранилище,
graceful shutdown, метрики, ротация ключей, backup/restore, HA, CI/CD,
LICENSE, релизный пайплайн, CLI, KV-версионирование.

### Оценка зрелости по измерениям

| Измерение | Сейчас | Цель v1.0 |
|---|---|---|
| Функциональность | 🟢 7/10 | 9/10 |
| Безопасность | 🔴 3/10 | 9/10 |
| Надёжность / HA | 🟠 4/10 | 7/10 |
| Наблюдаемость | 🔴 2/10 | 8/10 |
| Эксплуатация (Day-2) | 🟠 4/10 | 8/10 |
| Релиз-инженерия | 🔴 1/10 | 8/10 |
| Документация | 🟢 7/10 | 9/10 |

---

## 2. Definition of Done для v1.0 (чек-лист GA)

- [ ] Весь трафик по TLS; токены/секреты никогда не идут plaintext по сети.
- [ ] Токены и чувствительные ключи не восстановимы из дампа хранилища.
- [ ] Полный tamper-evident audit-лог всех обращений к секретам и auth-событий.
- [ ] Чистый graceful shutdown по SIGTERM, корректное закрытие bbolt.
- [ ] Liveness/readiness-пробы; sealed ⇒ not ready.
- [ ] Backup/restore хранилища с консистентным снапшотом.
- [ ] Ротация barrier-ключа и rekey root-ключа без простоя данных.
- [ ] Prometheus-метрики + структурированные логи без утечки секретов.
- [ ] Rate limiting / lockout на auth и unseal.
- [ ] CI: build, race, vet, lint, gosec, govulncheck, coverage-гейт.
- [ ] Релиз: подписанные мультиарх-бинари + образы, SBOM, checksums.
- [ ] LICENSE, SECURITY.md, threat model, CHANGELOG, semver.
- [ ] Внешний security-review / аудит крипто-ядра.
- [ ] Нагрузочное и soak-тестирование пройдено.

---

## 3. Майлстоуны и порядок

```
v0.5  Security hardening   → Фаза 1 (P0)            [блокеры безопасности]
v0.6  Reliability & Ops    → Фаза 2
v0.7  Observability        → Фаза 3
v0.8  Operator hardening   → Фаза 4
v0.9  HA + API completeness→ Фазы 5–6
v1.0  Release engineering  → Фаза 7 + аудит (Фаза 8)
```

Рекомендация: **строго начать с Фазы 1** — пока не закрыта безопасность,
остальные улучшения наращивают поверхность атаки на дырявом фундаменте.

---

## Фаза 1 — Security hardening (v0.5, P0-блокеры)

#### SEC-1 — Хешировать токены и чувствительные ключи в хранилище `[P0 · M]`
- **Проблема:** `barrier` шифрует только *значение* записи; *ключ* bbolt
  (`auth/token/<raw-id>`, `secret/<path>`) пишется plaintext. Дамп `tuck.db`
  раскрывает валидные bearer-токены и пути секретов.
- **Что сделать:** хранить токены под `auth/token/HMAC-SHA256(token, barrierKey)`,
  выдавать пользователю отдельный непрозрачный `accessor` для lookup/revoke.
  Рассмотреть HMAC-обфускацию имён ключей секретов (или явно задокументировать
  утечку путей-метаданных как принятый риск).
- **Done:** в дампе `.db` нет ни одного валидного токена; `TestTokenNotInPlaintext`
  гриппит дамп и не находит ID; lookup/revoke работают через accessor.
- **Файлы:** `internal/token/store.go`, `internal/token/token.go`, `internal/core/core.go`.

#### SEC-2 — TLS / mTLS на сервере `[P0 · M]`
- **Что сделать:** `ListenAndServeTLS`, флаги `-tls-cert/-tls-key/-tls-client-ca`,
  опциональный mTLS, авто-генерация self-signed для dev, перенаправление/запрет
  plaintext. В деплое — отдать сертификат через cert-manager.
- **Done:** сервер слушает HTTPS; plaintext-запрос отклоняется; e2e на minikube по TLS.
- **Файлы:** `cmd/tuck/main.go`, `internal/api/server.go`, `deploy/server/*`.

#### SEC-3 — Audit-лог (append-only, tamper-evident) `[P0 · L]`
- **Что сделать:** новый пакет `internal/audit`. Хэш-цепочка записей
  (`hash_n = H(hash_{n-1} || entry)`), JSON-строки, отдельный sink (файл/stdout).
  Логировать: кто (accessor), что (path+capability), когда, результат, request-id.
  **Секреты-значения не логировать**, только пути и метаданные. Перед выполнением
  операции запись в audit обязательна (fail-closed: нет audit → отказ).
- **Done:** все обращения к секретам и auth-события в audit; цепочка верифицируется
  утилитой; разрыв цепочки детектируется тестом.
- **Файлы:** `internal/audit/*` (новый), интеграция в `core` и `api`.

#### SEC-4 — Хардненинг HTTP-сервера `[P0 · S]`
- **Что сделать:** выставить `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`,
  `IdleTimeout`, `MaxHeaderBytes`; убрать Slowloris-вектор; сохранить лимит тела.
- **Done:** `http.Server` сконфигурирован явно; линтер `gosec` G112 чист.
- **Файлы:** `cmd/tuck/main.go`, `internal/api/server.go`.

#### SEC-5 — Rate limiting / brute-force lockout `[P1 · M]`
- **Что сделать:** лимит на `/v1/auth/*` и `/v1/sys/unseal` (token-bucket по IP +
  глобально), экспоненциальная задержка после серии неуспехов, метрика отказов.
- **Done:** нагрузочный тест: >N неверных токенов/с → 429; легитимный трафик не страдает.
- **Файлы:** `internal/api/*`, новый `internal/ratelimit`.

#### SEC-6 — Гигиена памяти `[P1 · M]`
- **Что сделать:** зануление (`zero`) копий root-ключа, расшифрованных секретов и
  marshalled-байтов токенов после использования; опциональный `mlockall` для
  защиты от свопа; запрет core dump.
- **Done:** ключевой материал зануляется (проверяемо в тестах через хук); на Linux
  страницы залочены при `-mlock`.
- **Файлы:** `internal/barrier/*`, `internal/core/core.go`, `cmd/tuck/main.go`.

#### SEC-7 — Управление root-токеном и регенерация `[P1 · M]`
- **Что сделать:** flow `operator generate-root` (Vault-стиль): отозвать root,
  пересоздать его, авторизуя операцию кворумом Shamir-шардов; запрет долгоживущего
  root в обычной работе (рекомендовать revoke после bootstrap).
- **Done:** root можно отозвать и регенерировать через unseal-кворум; документировано.
- **Файлы:** `internal/core/core.go`, `internal/api/sys.go`, `internal/seal/shamir.go`.

#### SEC-8 — Threat model + security-документация `[P1 · S]`
- **Что сделать:** `docs/THREAT_MODEL.md` (активы, доверенные границы, векторы,
  что in/out of scope), `SECURITY.md` (политика раскрытия уязвимостей).
- **Done:** оба файла в репозитории, привязаны к задачам фаз.

---

## Фаза 2 — Reliability & Operations (v0.6)

#### OPS-1 — Graceful shutdown `[P0 · S]`
- **Что сделать:** перехват SIGTERM/SIGINT, `srv.Shutdown(ctx)` с дренажом
  in-flight, затем `barrier.Seal()` и `backend.Close()`. k8s `terminationGracePeriod`.
- **Done:** при SIGTERM активные запросы дозавершаются, bbolt закрыт чисто; тест на сигнал.
- **Файлы:** `cmd/tuck/main.go`, `cmd/tuck-operator/main.go`.

#### OPS-2 — Liveness/readiness разделение `[P0 · S]`
- **Что сделать:** `/v1/sys/health` (liveness — процесс жив) и
  readiness (sealed/неинициализирован ⇒ 503/not-ready). Пробы в манифестах.
- **Done:** sealed-под не получает трафик; деплой использует обе пробы.
- **Файлы:** `internal/api/sys.go`, `deploy/server/deployment.yaml`.

#### OPS-3 — Backup / restore / snapshot `[P0 · M]`
- **Что сделать:** `GET /v1/sys/snapshot` (консистентный bbolt `Tx.WriteTo`,
  только root), `POST /v1/sys/restore`; CLI-обёртки; документировать ротацию бэкапов.
- **Done:** снапшот на работающем сервере → restore в чистый инстанс → данные целы; e2e.
- **Файлы:** `internal/physical/bbolt.go`, `internal/api/sys.go`.

#### OPS-4 — GC протухших токенов `[P1 · M]`
- **Что сделать:** фоновый reaper, периодически листит `auth/token/*` и удаляет
  истёкшие; метрика числа живых/удалённых; backpressure-safe.
- **Done:** истёкшие токены физически удаляются; тест с коротким TTL.
- **Файлы:** `internal/token/store.go`, `internal/core/core.go`.

#### OPS-5 — Ротация ключей: barrier rotate + root rekey `[P1 · L]`
- **Что сделать:** (1) `rotate` — новый DEK, перешифровка keyring, поддержка
  нескольких term-ключей для чтения старых данных; (2) `rekey` — смена root-ключа
  (и перераздача Shamir-шардов) без перешифровки данных.
- **Done:** после rotate/rekey старые данные читаются, новые пишутся новым ключом; e2e.
- **Файлы:** `internal/barrier/*`, `internal/seal/*`, `internal/api/sys.go`.

#### OPS-6 — Конфигурация: env + файл, без секретов во флагах `[P1 · M]`
- **Что сделать:** конфиг через файл (HCL/YAML) и env; sensitive-значения
  (transit token) не передавать `-flag` (видно в `ps`); валидация на старте.
- **Done:** запуск из конфиг-файла; transit-token только из env/файла; флаги-секреты
  помечены deprecated.
- **Файлы:** новый `internal/config`, `cmd/tuck/main.go`.

#### OPS-7 — Контекст-таймауты в стораджных операциях `[P2 · S]`
- **Что сделать:** прокидывать `ctx` в bbolt-обёртки, уважать отмену/дедлайн.
- **Done:** отменённый запрос не висит на сторадже.
- **Файлы:** `internal/physical/bbolt.go`.

---

## Фаза 3 — Observability (v0.7)

#### OBS-1 — Prometheus-метрики `[P1 · M]`
- **Что сделать:** `/metrics`: счётчики/гистограммы запросов по route+status,
  gauge `tuck_sealed`, счётчики auth success/fail, unseal-события, размер хранилища,
  reaper-статистика, operator sync ok/fail.
- **Done:** `/metrics` отдаёт корректные значения; дашборд-пример в `docs/`.
- **Файлы:** новый `internal/metrics`, интеграция в `api`/`core`/`operator`.

#### OBS-2 — Структурированное логирование (slog) `[P1 · M]`
- **Что сделать:** заменить `log` на `log/slog`, уровни, JSON-формат, request-id,
  **гарантия не-логирования значений секретов**; audit — отдельный канал (SEC-3).
- **Done:** логи структурированы; статический тест/ревью на отсутствие секретов в логах.
- **Файлы:** весь код, использующий `log`.

#### OBS-3 — Tracing (OpenTelemetry) `[P2 · M]`
- **Что сделать:** опциональный OTLP-экспорт спанов запросов.
- **Done:** трейсы видны в коллекторе при включённом флаге.

---

## Фаза 4 — Operator hardening (v0.8)

#### OP-1 — Leader election `[P1 · M]`
- **Проблема:** несколько реплик оператора будут двойной записью конкурировать.
- **Что сделать:** lease-based leader election (k8s `coordination.k8s.io/Lease`),
  только лидер реконсайлит.
- **Done:** 2+ реплик, пишет только лидер; failover при падении лидера; тест.
- **Файлы:** `internal/operator/*`, `cmd/tuck-operator/main.go`.

#### OP-2 — Status-условия на CRD `[P1 · M]`
- **Что сделать:** `.status.conditions` (`Synced`, `Ready`), `observedGeneration`,
  `lastSyncTime`, сообщение об ошибке; печать в `kubectl get tucksecret`.
- **Done:** статус отражает реальность; `kubectl get` показывает Ready/ошибку.
- **Файлы:** `deploy/crd/tucksecret.yaml`, `internal/operator/*`.

#### OP-3 — Backoff и метрики реконсайла `[P1 · S]`
- **Что сделать:** экспоненциальный backoff на ошибках reconcile (сейчас фикс. 5с),
  метрики OBS-1, ограничение «грозы» одновременных синков.
- **Done:** при недоступности Tuck оператор не спамит; backoff растёт; метрики идут.
- **Файлы:** `internal/operator/controller.go`.

#### OP-4 — Политика удаления Secret (опционально) `[P2 · S]`
- **Что сделать:** поле `spec.deletionPolicy: Retain|Delete` + ownerReference-режим.
- **Done:** поведение настраиваемо; default `Retain` (текущее).
- **Файлы:** `internal/operator/types.go`, `controller.go`.

---

## Фаза 5 — High Availability (v0.9)

#### HA-1 — Raft-реплицируемый storage backend `[P2 · XL]`
- **Что сделать:** реализация `physical.Backend` поверх `hashicorp/raft` +
  bbolt-лог-стор; лидер-форвардинг записей; снапшоты Raft; 3/5-нодовый кворум.
  Это нарушает принцип «zero deps», но остаётся встроенным (без внешнего etcd).
- **Done:** 3-нодовый кластер переживает падение ноды без потери данных; chaos-тест.
- **Файлы:** новый `internal/physical/raft`, `internal/core` (forwarding).
- **Альтернатива (меньше работы):** официально поддержать внешний бэкенд
  (Postgres/etcd) как `Backend`, оставив bbolt дефолтом — но это идёт против
  позиционирования «no external DB». Решить продуктово (см. Риски).

#### HA-2 — Авто-unseal на всех нодах `[P2 · M]`
- **Что сделать:** при HA каждая нода поднимается через transit/KMS auto-unseal
  без ручного кворума; standby-ноды в hot-standby.
- **Done:** рестарт любой ноды авто-распечатывается; нет ручных шагов.

---

## Фаза 6 — API / продуктовая полнота (v0.9)

#### API-1 — Binary-safe значения секретов `[P1 · S]`
- **Проблема:** значение отдаётся как JSON-строка → бинарные/не-UTF8 секреты бьются.
- **Что сделать:** base64 в ответе или структурный KV (несколько key/value на путь).
- **Done:** бинарный секрет round-trip без потерь; тест с не-UTF8.
- **Файлы:** `internal/api/kv.go`, `internal/core/core.go`.

#### API-2 — KV v2: версии, метаданные, soft-delete, CAS `[P2 · L]`
- **Что сделать:** версионирование значений, `metadata`, undelete/destroy,
  check-and-set; обратная совместимость с KV v1.
- **Done:** история версий, откат, CAS-конфликты; e2e.
- **Файлы:** новый `internal/kv`, `internal/api/kv.go`.

#### API-3 — Эндпоинты list `[P1 · S]`
- **Что сделать:** `LIST /v1/secret/<prefix>`, list токенов/политик; завязать на
  существующую capability `CapList`.
- **Done:** list отдаёт ключи с учётом ACL; тест на скрытие вне-scope.
- **Файлы:** `internal/api/*`, `internal/core/core.go`.

#### API-4 — Token renew / leases `[P1 · M]`
- **Что сделать:** `POST /v1/auth/token/renew`, lease-учёт, каскадный revoke
  дочерних токенов, `max_ttl`.
- **Done:** токен продлевается до max_ttl; revoke родителя гасит детей.
- **Файлы:** `internal/token/*`, `internal/core/core.go`, `internal/api/tokens.go`.

#### API-5 — CLI-клиент `tuck` `[P1 · M]`
- **Что сделать:** подкоманды `login`, `kv get/put/list`, `token`, `policy`,
  `operator unseal`, `status`; конфиг через env `TUCK_ADDR`/`TUCK_TOKEN`. DX —
  наш дифференциатор, curl недостаточно.
- **Done:** базовые сценарии из README выполняются через CLI, не curl.
- **Файлы:** новый `cmd/tuck-cli` (или `cmd/tuck` subcommand split).

#### API-6 — Go SDK + OpenAPI `[P2 · M]`
- **Что сделать:** `sdk/go` тонкий клиент; OpenAPI-спека `api/openapi.yaml`,
  генерация docs.
- **Done:** SDK покрывает все эндпоинты; спека валидна.

---

## Фаза 7 — Release engineering (v1.0)

#### REL-1 — CI-пайплайн `[P0 · M]`
- **Что сделать:** GitHub Actions: `go build`, `go test -race`, `go vet`,
  `golangci-lint`, `gosec`, `govulncheck`, coverage-гейт (напр. ≥70% на крипто/auth),
  e2e на `kind`. Блокировать merge при красном.
- **Done:** PR не мёржится без зелёного CI; бейджи в README.
- **Файлы:** `.github/workflows/*`.

#### REL-2 — LICENSE `[P0 · S]`
- **Что сделать:** выбрать и зафиксировать (рекомендация: **Apache-2.0** —
  patent grant, дружелюбна бизнесу; либо **MPL-2.0** как у OpenBao). Добавить
  заголовки/`NOTICE` при необходимости.
- **Done:** `LICENSE` в корне; README обновлён; CLA/DCO-политика выбрана.

#### REL-3 — Релизный пайплайн `[P1 · M]`
- **Что сделать:** `goreleaser` — мультиарх бинари (linux/darwin/windows,
  amd64/arm64), checksums, подпись `cosign`, SBOM `syft`, GitHub Releases,
  публикация образов в registry.
- **Done:** тег `vX.Y.Z` выпускает подписанные артефакты + SBOM автоматически.
- **Файлы:** `.goreleaser.yaml`, `.github/workflows/release.yml`.

#### REL-4 — Хардненинг контейнерных образов `[P1 · S]`
- **Что сделать:** перейти на `distroless`/`scratch`, non-root (uid 65532),
  read-only rootfs, без shell; pin digest базового образа; scan trivy в CI.
- **Done:** образ проходит trivy без HIGH/CRITICAL; рантайм non-root + RO-FS.
- **Файлы:** `build/Dockerfile.server`, `build/Dockerfile.operator`, `deploy/*`.

#### REL-5 — Версионирование и совместимость `[P1 · S]`
- **Что сделать:** semver, `CHANGELOG.md` (keep-a-changelog), политика стабильности
  API (`/v1` заморожен на 1.0), schema-версия формата хранилища + миграции.
- **Done:** есть CHANGELOG; задокументированы гарантии совместимости.

#### REL-6 — Community-файлы `[P2 · S]`
- **Что сделать:** `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, issue/PR-шаблоны,
  `CODEOWNERS`.
- **Done:** файлы на месте; новый контрибьютор видит процесс.

---

## Фаза 8 — Pre-1.0 hardening (gate на GA)

#### QA-1 — Race / fuzz / coverage `[P1 · M]`
- **Что сделать:** `-race` в CI; fuzz-тесты для парсеров (Shamir-шарды, glob-ACL,
  JSON-входы, secretKey-нормализация); coverage-гейт на security-критичных пакетах.
- **Done:** fuzz без падений N часов; race чисто; coverage-порог достигнут.

#### QA-2 — Нагрузочное и soak-тестирование `[P1 · M]`
- **Что сделать:** бенч `k6`/`vegeta` (RPS, p99 latency), 24h soak на утечки
  памяти/дескрипторов, проверка под рестартами/seal-циклами.
- **Done:** профиль производительности задокументирован; нет утечек за 24h.

#### QA-3 — Внешний security-review / аудит крипто-ядра `[P1 · L]`
- **Что сделать:** независимое ревью `barrier`/`seal`/`shamir`/auth; прогон
  `gosec`/`govulncheck`; устранение находок до GA. По возможности — внешний аудит.
- **Done:** отчёт ревью + закрытые находки; нет открытых High/Critical.

#### QA-4 — Disaster-recovery runbook `[P2 · S]`
- **Что сделать:** `docs/RUNBOOK.md`: потеря ноды, повреждение bbolt, утрата
  Shamir-кворума, ротация при компрометации, restore из бэкапа.
- **Done:** прогон каждого сценария по runbook в стейджинге.

---

## 4. Сводная таблица приоритетов

| Приоритет | Задачи |
|---|---|
| **P0 (блокеры)** | SEC-1, SEC-2, SEC-3, SEC-4, OPS-1, OPS-2, OPS-3, REL-1, REL-2 |
| **P1 (для GA)** | SEC-5, SEC-6, SEC-7, SEC-8, OPS-4, OPS-5, OPS-6, OBS-1, OBS-2, OP-1, OP-2, OP-3, API-1, API-3, API-4, API-5, REL-3, REL-4, REL-5, QA-1, QA-2, QA-3 |
| **P2 (v1.x)** | OPS-7, OBS-3, OP-4, HA-1, HA-2, API-2, API-6, REL-6, QA-4 |

---

## 5. Продуктовые развилки (решить заранее)

1. **HA-стратегия (HA-1).** Встроенный Raft (верность принципу «no external DB»,
   но XL по объёму) **vs** официальная поддержка внешнего бэкенда (быстрее, но
   размывает позиционирование). Рекомендация: к v1.0 — отлаженный single-node +
   backup/restore + быстрый рестарт через auto-unseal; Raft вынести в v1.1.
2. **Лицензия (REL-2).** Apache-2.0 (бизнес-дружелюбна, patent grant) vs MPL-2.0
   (как OpenBao, copyleft на файлы). Влияет на контрибуции и принятие.
3. **KV v1 vs v2 (API-2).** Минимум для GA — binary-safe v1 (API-1); полноценный
   v2 с версиями можно в v1.x.
4. **Cloud KMS-native auto-unseal.** Сейчас только Vault-Transit-совместимый seal.
   Нативные AWS KMS / GCP KMS / Azure Key Vault — частый прод-запрос; добавить как
   отдельные `seal`-реализации (кандидат в P1, если целевая аудитория — облако).

---

## 6. Рекомендованный первый спринт

Начать строго с безопасности (фундамент доверия):

1. **SEC-1** — токены/ключи не plaintext в хранилище.
2. **SEC-2** — TLS.
3. **OPS-1 + OPS-2** — graceful shutdown + readiness (дёшево, сразу k8s-корректно).
4. **SEC-4** — таймауты сервера.
5. **REL-1 + REL-2** — CI и лицензия (чтобы дальше всё ехало на зелёном CI).
6. **SEC-3** — audit-лог.

После этого спринта Tuck впервые становится «можно осторожно ставить в прод
с известными ограничениями», а дальше идём по фазам к полноценному GA.
