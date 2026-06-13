# 20 — Raft HA (кластер из 3 нод)

[← Все гайды](README.md)

## Цель

Запустить отказоустойчивый кластер Tuck на встроенном Raft storage.

## Предварительные условия

- 3+ ноды с общим сетевым доступом
- TLS между нодами
- Shamir или auto-unseal

## Шаги

### 1. Первая нода (leader)

```hcl
storage "raft" {
  path    = "/var/lib/tuck/raft"
  node_id = "node1"
}
```

```powershell
.\tuck.exe server -config node1.hcl
.\tuckcli.exe operator init
# unseal...
```

### 2. Join вторая нода

```hcl
storage "raft" {
  path    = "/var/lib/tuck/raft"
  node_id = "node2"
}
retry_join {
  leader_api_addr = "https://node1:8200"
}
```

```powershell
.\tuck.exe server -config node2.hcl
.\tuckcli.exe operator unseal   # на каждой ноде
```

### 3. Третья нода — аналогично node2

### 4. Проверить peers

```powershell
curl "$env:TUCK_ADDR/v1/sys/storage/raft/configuration" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| raft configuration | 3 сервера, 1 leader |
| Остановить follower | кластер работает |
| Остановить 2 ноды | quorum потерян — read-only / unavailable |

## Дальше

- [19 — Backup](19-backup-restore.md)
- [05 — Auto-unseal](05-auto-unseal-kms.md)
