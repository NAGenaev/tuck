# 05 — Auto-unseal (Transit)

[← Все гайды](README.md)

## Цель

Сервер автоматически распечатывается после рестарта без ручного ввода Shamir-ключей. Корневой ключ хранится зашифрованным ключом Transit в HashiCorp Vault или OpenBao (любом сервисе с Vault-совместимым API `transit/encrypt` и `transit/decrypt`). Собственного Transit в Tuck нет.

## Кому подходит

SRE, у которых уже есть Vault/OpenBao, управляемый независимо от Tuck. Это основной способ распечатки для кластера из 3 нод ([20 — Raft HA](20-ha-raft.md)).

## Предварительные условия

- Vault/OpenBao с включённым движком transit, ключом шифрования и токеном, у которого есть права `update` на `transit/encrypt/<ключ>` и `transit/decrypt/<ключ>`
- Печать выбирается при первом запуске Tuck и позже не меняется

## Шаги

### 1. Создать ключ в Transit

```bash
vault secrets enable transit
vault write -f transit/keys/tuck-root
vault policy write tuck-seal - <<'EOF'
path "transit/encrypt/tuck-root" { capabilities = ["update"] }
path "transit/decrypt/tuck-root" { capabilities = ["update"] }
EOF
vault token create -policy=tuck-seal -period=768h
```

Токен нужно продлевать или выдавать периодическим (`-period`), иначе после истечения следующий рестарт не сможет распечататься.

### 2. Запустить сервер с Transit seal

```bash
export TUCK_TRANSIT_TOKEN=<токен>
tuck --seal-type=transit \
  --seal-transit-addr=https://transit.example.com \
  --seal-transit-key=tuck-root \
  --seal-transit-key-file=/var/lib/tuck/tuck-transit.enc
```

Токен лучше передавать через переменную окружения `TUCK_TRANSIT_TOKEN`, а не флагом (виден в `ps`). Зашифрованный корневой ключ сохраняется в `--seal-transit-key-file`. В кластере он дополнительно реплицируется через Raft, поэтому все ноды распечатываются одним и тем же ключом.

При первом запуске Tuck печатает в лог **root-токен** (один раз). Shamir-долей при Transit-печати нет.

То же в конфиге:

```yaml
seal:
  type: transit
  transit:
    addr: https://transit.example.com
    key: tuck-root
    key_file: /var/lib/tuck/tuck-transit.enc
```

### 3. Проверить

```powershell
.\tuckcli.exe status
```

`sealed: false` без ручного `unseal`.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Рестарт pod/процесса | `sealed: false` за секунды |
| Логи сервера | сообщение `Transit seal` и успешный auto-unseal |
| Рестарт при недоступном Vault | процесс не падает: `sealed: true`, `/v1/sys/ready` отвечает `503`, раз в 5 секунд повтор; в логе `auto-unseal failed, staying SEALED and retrying every 5s` |
| Vault снова доступен | нода сама пишет `auto-unseal succeeded` и становится `sealed: false` (проверено на кластере из 3 нод и Vault 1.15) |
| Неверный или отозванный токен Transit при старте | то же состояние `sealed`, в логе `status 403: [permission denied]`; исправьте `TUCK_TRANSIT_TOKEN` и перезапустите под |
| Отзыв токена у уже работающей ноды | нода продолжает работать (ключ в памяти); проблема проявится при следующем рестарте |

## Важно

- Потеря ключа `tuck-root` в Vault (или его удаление) означает потерю всех данных Tuck: Shamir-долей для восстановления нет. Резервируйте Vault, не включайте `deletion_allowed` для ключа и держите снимки Tuck ([19 — Backup](19-backup-restore.md)).
- Доступность Transit становится зависимостью старта: при его недоступности нода остаётся sealed и ждёт, пока Vault вернётся. Под в Kubernetes не будет Ready, но и не уйдёт в CrashLoopBackOff.
- Первая попытка распечатки при старте может занять до 10 секунд (таймаут запроса к Vault), HTTP-порт ноды открывается только после неё. Учитывайте это в `startupProbe`.
- Не храните токен Transit в репозитории.

## Дальше

- [04 — Shamir](04-prod-shamir.md)
- [06 — Helm в K8s](06-kubernetes-helm.md)
