# 19 — Backup и restore

[← Все гайды](README.md)

## Цель

Создать снимок данных Tuck и восстановить кластер после сбоя.

## Шаги

### 1. Snapshot (встроенный API)

```powershell
curl -k "$env:TUCK_ADDR/v1/sys/snapshot" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -o tuck-snapshot.db
```

Потоковый дамп bbolt — работает на live-сервере (single-node file storage).

### 2. Проверить файл

```powershell
Get-Item tuck-snapshot.db | Select-Object Length, LastWriteTime
```

### 3. Restore (остановить сервер, заменить data file)

```powershell
# остановите tuck server
Copy-Item tuck-snapshot.db D:\tuck-data\tuck.db -Force
# запустите сервер снова
```

### 4. Unseal после restore

- Shamir: [04 — Shamir](04-prod-shamir.md)
- Auto-unseal: [05 — Auto-unseal](05-auto-unseal-kms.md)

### 5. Raft HA (альтернатива)

Для Raft-кластера используйте snapshot API storage backend — см. [20 — Raft HA](20-ha-raft.md) и [docs/RUNBOOK.md](../../docs/RUNBOOK.md).

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| snapshot файл | ненулевой размер |
| restore + unseal | `kv get` старых секретов работает |
| health | initialized true |

## Рекомендации

- Регулярные snapshot + off-site копия
- Тестируйте restore раз в квартал
- Root token после restore — из snapshot эпохи; планируйте ротацию

## Дальше

- [20 — Raft HA](20-ha-raft.md)
- [04 — Shamir](04-prod-shamir.md)
