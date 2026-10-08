# 20 — Raft HA (кластер из 3 нод)

[← Все гайды](README.md)

## Цель

Запустить отказоустойчивый кластер Tuck на встроенном Raft storage.

## Предварительные условия

- 3+ ноды с общим сетевым доступом (HTTP API и порт Raft)
- TLS между нодами (`--cluster-tls-cert/-key/-ca`)
- Transit auto-unseal ([05](05-auto-unseal-transit.md)) или Shamir ([04](04-prod-shamir.md))

## Шаги

Пример для 3 нод с Transit-печатью. Флаги печати у всех нод одинаковы:

```bash
SEAL="--seal-type=transit --seal-transit-addr=https://vault.corp:8200 --seal-transit-key=tuck-root"
export TUCK_TRANSIT_TOKEN=<токен>
```

### 1. Запустить три ноды

Первая нода создаёт кластер сразу со всеми голосующими участниками (`--cluster-bootstrap` + `--cluster-peers`), остальные запускаются без bootstrap:

```bash
PEERS="node1=node1:8201,node2=node2:8201,node3=node3:8201"

# node1
tuck $SEAL --seal-transit-key-file=/var/lib/tuck/transit.enc --addr=0.0.0.0:8200 \
  --cluster --cluster-node-id=node1 --cluster-bind-addr=0.0.0.0:8201 --cluster-advertise=node1:8201 \
  --cluster-dir=/var/lib/tuck/raft --cluster-bootstrap --cluster-peers="$PEERS"

# node2, node3: те же флаги, свои --cluster-node-id и --cluster-advertise, без --cluster-bootstrap и --cluster-peers
```

`--cluster-bootstrap` на уже работающей ноде игнорируется, поэтому оставлять флаг в unit-файле безопасно. Лидер выбирается сам, инициализирует хранилище и печатает в лог root-токен (один раз). Остальные ноды ждут инициализации и распечатываются тем же Transit-ключом автоматически.

### 2. Добавить ноду позже

```bash
tuck $SEAL ... --cluster --cluster-node-id=node4 --cluster-join=https://node1:8200
```

`--cluster-join` просит лидера принять ноду; нужен `TUCK_TOKEN` администратора в окружении запускаемой ноды.

### 3. Проверить состав

```bash
curl -s -H "X-Tuck-Token: $TUCK_TOKEN" "$TUCK_ADDR/v1/sys/cluster"
for n in node1 node2 node3; do curl -s -o /dev/null -w "$n %{http_code}\n" http://$n:8200/v1/sys/leader; done   # 200 у лидера, 429 у остальных
```

## Что делает кластер при записи и отказе

- Пишет только **leader**. Follower на запись отвечает `503 not leader` (проксирования нет), а вход приложения по K8s-токену — это запись, поэтому трафик клиентов нужно направлять **на leader**. Читать можно с любой ноды, но с небольшим отставанием репликации.
- `GET /v1/sys/leader` (без токена): `200` — активная нода (или обычный режим без Raft), `429` — standby, `503` — запечатана. Используйте для балансировщика вне Kubernetes.
- Свежая нода-follower не инициализирует хранилище сама: она ждёт, пока leader создаст его и состояние реплицируется (в логе `followers wait for the leader to initialize the store`).
- Для Transit-печати обёрнутый корневой ключ реплицируется через кластер: все ноды распечатываются одним и тем же ключом, в том числе после `rotate`. Ротацию ключа запускайте на leader. Печать `dev` в кластере не работает (ключ у каждой ноды свой).
- Падение leader: выборы занимают единицы секунд (проверено на 3 локальных нодах с Transit-печатью в Vault: остановка leader → оставшаяся нода принимает записи, прежние секреты читаются с любой ноды; холодный рестарт всех трёх нод: все распечатываются сами, данные на месте).
- Нода, перезапущенная при недоступном Vault, не падает, а остаётся sealed и сама распечатывается, когда Vault вернётся; остальные ноды всё это время обслуживают запросы (проверено).

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
- Raft-трафик между подами — plain TCP, пока не задан `server.cluster.tls.secretName` (mTLS). В банке включите его (раздел ниже).
- При `shamir` после рестарта каждый под ждёт ручной распечатки; для пилота используйте Transit.

### mTLS на Raft-транспорте

```yaml
server:
  cluster:
    enabled: true
    tls:
      secretName: tuck-raft-tls   # tls.crt, tls.key, ca.crt (формат cert-manager)
      serverName: ""              # необязательно: ожидаемое имя в сертификате пира
```

Сертификат один на все поды, поэтому в SAN нужен шаблон `*.<release>-server-headless.<ns>.svc`, а в extended key usage — и `serverAuth`, и `clientAuth`. Поды стартуют с `--cluster-tls-cert/-key/-ca`; в логе должна быть строка «Raft RPC secured with mutual TLS». Проверено на minikube: 3 пода, общий сертификат, выбор лидера, запись и чтение через Service, смена лидера после удаления пода.

## Обновление версии без остановки

Обновляйте узлы **по одному** и переходите к следующему только когда предыдущий распечатан и кластер здоров. Порядок: сначала standby-узлы, затем передача лидерства, затем бывший лидер.

```bash
# 0. До начала: все три узла распечатаны (200 у лидера, 429 у остальных)
for n in 1 2 3; do curl -s -o /dev/null -w "node$n %{http_code}\n" https://node$n:8200/v1/sys/leader; done

# 1. Для каждого standby-узла: заменить образ/бинарник, запустить, внести k долей Shamir,
#    дождаться, что у него снова 429 (а не 503), и только потом идти дальше
tuckcli --addr https://node2:8200 unseal <доля>     # k раз

# 2. Передать лидерство и обновить бывший лидер
curl -X POST -H "X-Tuck-Token: $TOKEN" https://tuck.bank.local:8200/v1/sys/cluster/step-down
```

Не трогайте лидера, пока хотя бы один из остальных узлов запечатан: без кворума распечатанных узлов кластер перестаёт отвечать (данные при этом целы, после внесения долей он возвращается). При ручной распечатке обновление трёх узлов требует трёх комплектов долей. Если это неприемлемо, используйте авто-распечатку (Transit, совместимый с Vault).

## Адрес клиента и лимиты запросов

Если перед Tuck стоит L7-балансировщик, он должен передавать `X-Forwarded-For`, а его адрес указывается в `trusted_proxies` (см. [25](25-pilot-deployment.md)). Пример для HAProxy, который терминирует TLS, повторно шифрует трафик до Tuck с проверкой сертификата и направляет запросы только лидеру:

```
frontend tuck_api
  bind *:8200 ssl crt /etc/haproxy/tuck.pem
  option forwardfor
  default_backend tuck_leader
backend tuck_leader
  option httpchk
  http-check send meth GET uri /v1/sys/leader ver HTTP/1.1 hdr Host tuck.bank.local
  http-check expect status 200
  default-server check check-ssl ssl verify required ca-file /etc/haproxy/ca.crt verifyhost tuck.bank.local sni str(tuck.bank.local) inter 2s fall 2 rise 1
  server node1 node1:8200
  server node2 node2:8200
  server node3 node3:8200
```

Лимиты запросов на адрес клиента и на токен задаются флагами `--rate-ip-rps/-burst` и `--rate-token-rps/-burst` (по умолчанию 100/200 и 50/100) и меняются без рестарта через `PUT /v1/sys/config` (поля `ip_rate_limit_rps`, `ip_rate_limit_burst`, `token_rate_limit_rps`, `token_rate_limit_burst`): `0` оставляет значение флага, отрицательное значение отключает лимит. Каждый узел перечитывает сохранённую настройку каждые 10 секунд, поэтому изменение действует на весь кластер и переживает смену лидера.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| raft configuration | 3 сервера, 1 leader |
| Остановить follower | кластер работает |
| Остановить 2 ноды | quorum потерян — read-only / unavailable |

## Дальше

- [19 — Backup](19-backup-restore.md)
- [05 — Auto-unseal](05-auto-unseal-transit.md)
