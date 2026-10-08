# 25 — Пилотное развёртывание (один сегмент)

[← Все гайды](README.md)

## Цель

Развернуть Tuck для пилота в одном сегменте: **KV v2 + Kubernetes auth + оператор `TuckSecret` + аудит**. Всё остальное (динамические секреты, PKI, SSH, TOTP) в пилот не входит и остаётся выключенным ([11 §4.1](../11-barrier-and-ciphers.md)).

## Топология

- Один Tuck на сегмент (не общий на все кластеры). Для отказоустойчивости — Raft на 3 узла ([20](20-ha-raft.md)); для первого пилота достаточно одного узла с бэкапами.
- Кластеры сегмента подключаются как `kubernetes.clusters` ([09](09-kubernetes-auth.md)). У каждого кластера — свой ServiceAccount рецензента (`system:auth-delegator`).
- Доставка в поды — **один** способ на пилот: оператор `TuckSecret` (рекомендуется: приложение не меняется, Secret обычный). Injector и CSI подключаются позже при необходимости.

## Шаги

### 1. Решения до установки

| Вопрос | Где решается |
|---|---|
| Шифр барьера (`aes-256-gcm` / `gost-kuznyechik-mgm`) — **фиксируется при первой инициализации** | [11 §2](../11-barrier-and-ciphers.md), [12 §1](../12-bank-readiness-todo.md) |
| Где живёт корневой ключ: Shamir вручную / auto-unseal через Transit (Barbican, другой Tuck) / HSM | [12 §1](../12-bank-readiness-todo.md); HSM не реализован |
| Куда писать аудит и в каком формате | `--audit-log`, [RUNBOOK](../RUNBOOK.md) |
| CA для TLS сервера и для Raft | корпоративный УЦ |

### 2. Конфигурация сервера

```yaml
# /etc/tuck/tuck.yaml
addr: 0.0.0.0:8200
data: /var/lib/tuck/tuck.db
tls:
  cert: /etc/tuck/tls.crt        # меняется без перезапуска (проверка раз в 15 с)
  key: /etc/tuck/tls.key
seal:
  type: shamir
  shamir: {config: /var/lib/tuck/tuck-shamir.json, n: 5, k: 3}
barrier:
  cipher: aes-256-gcm            # только для нового хранилища
require_mlock: true              # Linux: отказ запуска без mlock (без swap)
kubernetes:
  api: https://k8s-a.example:6443
  token_file: /etc/tuck/k8s-a.token
  ca_file: /etc/tuck/k8s-a.ca
  clusters:
    - {name: seg-b, api: https://k8s-b.example:6443, token_file: /etc/tuck/k8s-b.token, ca_file: /etc/tuck/k8s-b.ca}
# экспериментальные движки (enable_engines) не включать
```

Аудит: `tuck server --config /etc/tuck/tuck.yaml --audit-log /var/log/tuck/audit.log`. Файл ротируется (`--audit-max-size`, `--audit-max-backups`); для SIEM заберите его агентом сбора логов.

Для Raft (3 узла) добавьте `cluster:` с `tls_cert`/`tls_key`/`tls_ca` — **mutual TLS на Raft-транспорте**, CA отдельный, выпускающий только сертификаты узлов Tuck ([20](20-ha-raft.md), [MANUAL_TESTS §25](../MANUAL_TESTS.md)). HTTP API защищён обычным TLS; mTLS для клиентов API не реализован.

**Проверка сертификата сервера клиентами.** Если сертификат выпущен корпоративным УЦ, которого нет в системных корнях образов, положите CA в ConfigMap (ключ `ca.crt`) и передайте его всем потребителям — без `insecure`:

| Потребитель | Как задать CA |
|-------------|---------------|
| Оператор | `--tuck-ca-file=/etc/tuck/ca/ca.crt` (Helm: `server.tls.caConfigMap`) |
| CSI-драйвер | `--tuck-ca-file=...` (Helm: тот же `server.tls.caConfigMap`) |
| tuck-agent (инжектор) | аннотация пода `tuck.io/ca-configmap: <имя ConfigMap>` → монтируется в `/tuck/ca`, `TUCK_CA_FILE` |
| Go SDK | `client.New(addr, token, client.WithCAFile(path))` |

Битый или отсутствующий файл CA — ошибка (запросы не уходят), а не молчаливый откат на системные корни. `insecure` оставьте только для стендов.

### 3. Первый запуск и распечатывание

1. Запустите сервер. **Один раз** в логе будут показаны `ROOT TOKEN` и `SHAMIR SHARES` — раздайте доли разным людям, root-токен уберите в сейф. Вместе их хранить нельзя.
2. После каждого рестарта нужны `k` долей: `tuckcli unseal <share>` (`tuckcli status` — проверка).
3. Проверка шифра: `GET /v1/sys/seal-status` → `barrier_cipher`.

### 4. Политики и доступ оператора

```powershell
# узкая политика оператора: только чтение KV
tuckcli policy put operator-read '[{"path":"secret/*","capabilities":["read","list"]}]'
# роль оператора (логин по ServiceAccount, без статического токена)
curl -XPUT "$env:TUCK_ADDR/v1/auth/kubernetes/role/tuck/tuck-operator" -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" -d '{"policies":["operator-read"],"ttl":"5m"}'
```

Оператор ставится Helm с `operator.enforceNamespacePaths=true` — TuckSecret в namespace `team-a` читает только `team-a/...` ([07](07-tucksecret-operator.md)). Для нескольких кластеров задайте `operator.tuckCluster`.

### 5. Миграция из Git (ansible-vault)

1. На доверенной машине расшифруйте файлы: `ansible-vault decrypt` (или `view`) — вывод **не в Git и не на общий диск**.
2. Разложите по путям `<namespace>/<приложение>/<ключ>` и загрузите: `tuckcli kv put team-a/db/pass "…"`.
3. Замените подстановку в Helm values на `TuckSecret` ([07](07-tucksecret-operator.md)); чарт читает обычный Secret.
4. Проверьте, что приложение стартует с секретом из Tuck, и только после этого удалите зашифрованные файлы из Git и **смените** секреты — история Git сохраняет старые значения и ключ ansible-vault остаётся скомпрометированным.

### 6. Аварийный сценарий (репетировать до пилота)

| Ситуация | Действие |
|---|---|
| Рестарт Tuck | unseal `k` долями; оператор продолжает работу, уже созданные Secret остаются |
| Потеря узла Raft | кластер работает при 2 из 3; замените узел и `-cluster-join` |
| Потеря данных | `GET /v1/sys/snapshot` по расписанию + off-site копия; восстановление — [19](19-backup-restore.md); без `k` долей Shamir снимок не расшифровать |
| Компрометация DEK/диска | `POST /v1/sys/rotate-data-key?reencrypt=true`; при компрометации root-ключа — `tuckcli rotate` ([RUNBOOK §5](../RUNBOOK.md)) |
| Компрометация токена | `tuckcli token revoke <id>`; для SA-ролей — удалить роль |

Восстановление из снимка на чистом узле проверьте как минимум один раз до начала пилота.

### 7. Чек-лист перед запуском

- [ ] ответ ИБ по СКЗИ и расположению корневого ключа получен ([12 §1](../12-bank-readiness-todo.md))
- [ ] TLS-сертификат от корпоративного УЦ; замена без рестарта проверена ([MANUAL_TESTS §24](../MANUAL_TESTS.md))
- [ ] у каждой роли K8s задан `tuck_namespace`, чужой `X-Tuck-Namespace` даёт 403 ([§22](../MANUAL_TESTS.md))
- [ ] аудит пишется и доходит до SIEM
- [ ] бэкап и восстановление отработаны, доли Shamir розданы
- [ ] `--require-mlock` включён, swap на узле отключён
- [ ] экспериментальные движки выключены (`GET /v1/pki/ca/pem` → 404)

## Что пилот не покрывает

Независимый аудит безопасности, сертифицированное СКЗИ, HSM-хранение корневого ключа, mTLS клиентов API — см. [12](../12-bank-readiness-todo.md).
