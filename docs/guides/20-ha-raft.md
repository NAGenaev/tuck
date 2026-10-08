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

## Что делает кластер при записи и отказе

- Пишет только **leader**. Follower на запись отвечает `503 not leader` (проксирования нет), а вход приложения по K8s-токену — это запись, поэтому трафик клиентов нужно направлять **на leader**. Читать можно с любой ноды, но с небольшим отставанием репликации.
- `GET /v1/sys/leader` (без токена): `200` — активная нода (или обычный режим без Raft), `429` — standby, `503` — запечатана. Используйте для балансировщика вне Kubernetes.
- Свежая нода-follower не инициализирует хранилище сама: она ждёт, пока leader создаст его и состояние реплицируется (в логе `followers wait for the leader to initialize the store`).
- Для Transit-печати обёрнутый корневой ключ реплицируется через кластер: все ноды распечатываются одним и тем же ключом, в том числе после `rotate`. Ротацию ключа запускайте на leader. Печать `dev` в кластере не работает (ключ у каждой ноды свой).
- Падение leader: выборы занимают единицы секунд (проверено: остановка leader → новая нода принимает записи, прежние данные читаются, вернувшаяся нода становится follower).

## Kubernetes (Helm)

```yaml
server:
  replicaCount: 3            # нечётное, >= 3
  sealType: transit          # или shamir; dev запрещён
  transitSeal:
    addr: https://vault.corp:8200
    tokenSecretName: tuck-transit-token
  persistence: { enabled: true }
  cluster:
    enabled: true
```

Чарт сам создаёт:

- headless-Service `<release>-server-headless` со стабильными DNS-именами подов (Raft: `<pod>.<release>-server-headless.<ns>.svc:8201`) и `publishNotReadyAddresses`;
- `podManagementPolicy: Parallel` и статический bootstrap всех трёх нод (`--cluster-bootstrap --cluster-peers=...`);
- `startupProbe` на `/v1/health`: follower не слушает HTTP, пока leader не инициализировал хранилище;
- метку `tuck.io/leader=true|false` на каждом поде (нода сама обновляет её по своей роли, `--cluster-label-leader`) и Service `<release>-server`, который выбирает **только** `tuck.io/leader=true`: оператор, CSI и agent ходят на leader без изменений;
- `Role` с `get, patch` на поды **только этого релиза** (`resourceNames`) для обновления метки.

Ограничения:

- Состав кластера фиксируется при установке. Менять `replicaCount` уже работающего кластера нельзя — добавляйте/удаляйте ноды через `cluster join` / `DELETE /v1/sys/cluster/node/{id}`.
- Между отказом leader и появлением нового проходит несколько секунд без доступного endpoint: клиенты (оператор по `refreshInterval`, CSI, SDK) повторят запрос при следующей попытке.
- Raft-трафик между подами — plain TCP, пока не заданы `cluster.tls_*` (mTLS). В банке включите его.
- Без `shamir`-автоматизации после рестарта каждый под ждёт ручной распечатки; для пилота используйте Transit.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| raft configuration | 3 сервера, 1 leader |
| Остановить follower | кластер работает |
| Остановить 2 ноды | quorum потерян — read-only / unavailable |

## Дальше

- [19 — Backup](19-backup-restore.md)
- [05 — Auto-unseal](05-auto-unseal-transit.md)
