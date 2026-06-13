# 03 — Sequence-диаграммы ключевых сценариев

[← Назад: Архитектура](02-architecture.md) · [К оглавлению](README.md) · [Далее: API/CLI →](04-api-cli-reference.md)

Здесь — последовательности взаимодействия для наиболее важных потоков Tuck.

---

## 3.1. Запуск и распечатывание (Shamir)

```mermaid
sequenceDiagram
    autonumber
    actor Op as Оператор
    participant Tuck as tuck server
    participant Seal as seal shamir
    participant Bar as barrier
    participant Phys as physical bbolt

    Op->>Tuck: запуск seal-type=shamir
    Tuck->>Phys: открыть bbolt
    Tuck->>Bar: состояние SEALED
    Note over Tuck: первый старт: генерация root key и N долей
    Op->>Tuck: POST /v1/sys/unseal доля 1
    Tuck->>Seal: собрать долю 1/K
    Op->>Tuck: POST /v1/sys/unseal доля 2
    Op->>Tuck: POST /v1/sys/unseal доля K
    Seal->>Seal: Combine долей в root key
    Seal->>Bar: Unseal root key
    Bar->>Phys: прочитать barrier/keyring
    Bar->>Bar: расшифровать DEK
    Bar-->>Tuck: состояние UNSEALED
    Tuck-->>Op: 200 unsealed
```

---

## 3.2. Авто-распечатывание (AWS KMS / GCP KMS / Azure KV)

```mermaid
sequenceDiagram
    autonumber
    participant Tuck as tuck server
    participant Seal as seal awskms
    participant KMS as Cloud KMS
    participant Bar as barrier

    Tuck->>Seal: старт, читать tuck-awskms.enc
    Seal->>KMS: Decrypt ciphertext via IRSA
    KMS-->>Seal: root key в памяти
    Seal->>Bar: Unseal root key
    Bar-->>Tuck: UNSEALED без участия человека
    Note over Tuck,KMS: рестарт пода без ручных действий
```

---

## 3.3. Запись и чтение KV-секрета (с проверкой ACL)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Клиент
    participant API as api.Server
    participant RL as rate-limiter
    participant Core as core.Core
    participant Pol as policy
    participant Bar as barrier
    participant Phys as physical
    participant Aud as audit

    Client->>API: PUT /v1/secret/db/password
    API->>RL: проверить лимиты IP и token
    API->>Core: Authenticate token
    Core-->>API: токен валиден
    API->>Core: TrackUse token
    API->>Core: PutSecret ns path value
    Core->>Pol: EnforceAccess path write
    Pol->>Pol: deny-pass затем allow-pass glob
    Pol-->>Core: разрешено
    Core->>Bar: Put secret path value
    Bar->>Bar: AES-256-GCM encrypt DEK
    Bar->>Phys: записать шифртекст
    Core->>Aud: запись в hash-chain
    API-->>Client: 204 No Content
```

При чтении (`GET`) поток аналогичен, но capability — `read`, и барьер расшифровывает значение перед отдачей.

---

## 3.4. Аутентификация приложения через Kubernetes SA

```mermaid
sequenceDiagram
    autonumber
    participant Pod as Pod workload
    participant Tuck as tuck server
    participant K8s as Kubernetes API
    participant Core as core.Core

    Pod->>Tuck: POST /v1/auth/kubernetes/login SA JWT
    Tuck->>K8s: TokenReview SA JWT
    K8s-->>Tuck: valid namespace serviceaccount
    Tuck->>Core: найти K8sRole namespace sa
    Core->>Core: создать scoped Tuck-токен
    Core-->>Pod: token policies expires_at
    Pod->>Tuck: запросы с X-Tuck-Token
```

---

## 3.5. Динамические креды БД (с lease и авто-отзывом)

```mermaid
sequenceDiagram
    autonumber
    actor App as Приложение
    participant Tuck as tuck server
    participant DBEng as database engine
    participant DB as PostgreSQL MySQL
    participant GC as GC reaper

    App->>Tuck: POST /v1/database/creds/readonly
    Tuck->>DBEng: GenerateCreds role readonly
    DBEng->>DB: creation_statements CREATE USER
    DB-->>DBEng: ok
    DBEng->>Tuck: записать Lease TTL username
    Tuck-->>App: username password lease_id ttl
    Note over App,DB: App использует временные креды
    GC->>DBEng: RevokeExpired по таймеру
    DBEng->>DB: revocation_statements DROP USER
    Note over Tuck: или DELETE /v1/database/lease/id
```

---

## 3.6. Выпуск TLS-сертификата (PKI)

```mermaid
sequenceDiagram
    autonumber
    actor Op as Оператор CI
    participant Tuck as tuck server
    participant PKI as pki engine
    participant Bar as barrier

    Op->>Tuck: POST /v1/pki/generate/root
    Tuck->>PKI: создать CA ECDSA P-256
    PKI->>Bar: сохранить CA cert и приватный ключ
    Op->>Tuck: PUT /v1/pki/roles/web
    Op->>Tuck: POST /v1/pki/issue/web
    PKI->>PKI: валидировать CN SAN по роли
    PKI->>PKI: сгенерировать ключ и подписать cert
    PKI->>Bar: сохранить CertRecord
    Tuck-->>Op: certificate private_key ca_chain serial
    Note over Op,Tuck: GET /v1/pki/ca/pem без токена
```

---

## 3.7. Transit — шифрование как сервис + ротация и rewrap

```mermaid
sequenceDiagram
    autonumber
    actor App as Приложение
    participant Tuck as tuck server
    participant TR as transit engine

    App->>Tuck: POST /v1/transit/keys/payments
    App->>Tuck: POST /v1/transit/encrypt/payments
    TR-->>App: ciphertext vault:v1
    Note over App: ключ не покидает Tuck
    App->>Tuck: POST /v1/transit/keys/payments/rotate
    TR->>TR: добавить версию ключа v2
    App->>Tuck: POST /v1/transit/rewrap/payments
    TR->>TR: decrypt v1 encrypt v2
    TR-->>App: ciphertext vault:v2
```

---

## 3.8. Response Wrapping — безопасная передача секрета

```mermaid
sequenceDiagram
    autonumber
    actor Issuer as Издатель
    actor Consumer as Получатель
    participant Tuck as tuck server

    Issuer->>Tuck: POST /v1/sys/wrapping/wrap
    Tuck->>Tuck: сохранить payload sys/wrapping/id
    Tuck-->>Issuer: wrapping_token tuck_wrap
    Issuer-->>Consumer: передаёт wrapping_token
    Consumer->>Tuck: POST /v1/sys/wrapping/unwrap
    Tuck->>Tuck: проверить одноразовость и TTL
    Tuck->>Tuck: отдать payload и удалить запись
    Tuck-->>Consumer: payload
    Note over Consumer,Tuck: повторный unwrap вернёт 400
```

---

## 3.9. Синхронизация TuckSecret оператором в K8s Secret

```mermaid
sequenceDiagram
    autonumber
    actor User as Пользователь
    participant API as kube-apiserver
    participant Ctrl as tuck-operator
    participant Lease as coordination Lease
    participant Tuck as tuck server

    User->>API: kubectl apply TuckSecret
    API-->>Ctrl: watch event ADDED
    Ctrl->>Lease: acquire leadership
    alt лидер
        Ctrl->>Tuck: POST /v1/auth/kubernetes/login
        Tuck-->>Ctrl: Tuck-токен кэш 4 мин
        Ctrl->>Tuck: GET /v1/secret/tuckPath
        Tuck-->>Ctrl: значение секрета
        Ctrl->>API: apply native Secret
        Ctrl->>API: status Synced True
    else не лидер
        Note over Ctrl: реконсайл пропускается
    end
```

---

## 3.10. Webhook-инъекция секретов в Pod (минуя etcd)

```mermaid
sequenceDiagram
    autonumber
    participant API as kube-apiserver
    participant WH as tuck-injector
    participant Pod as Pod
    participant Agent as tuck-agent
    participant Tuck as tuck server

    API->>WH: AdmissionReview Pod inject=true
    WH->>WH: BuildPatch RFC 6902
    WH-->>API: patch emptyDir init agent mount
    API->>Pod: создать изменённый Pod
    Pod->>Agent: запуск init-контейнера
    Agent->>Tuck: fetch секретов pkg/client
    Tuck-->>Agent: значения
    Agent->>Pod: запись файлов 0400 на tmpfs
    Note over Agent: fail fast если секрет отсутствует
    Pod->>Pod: запуск app-контейнеров
```

---

## 3.11. HA-запись через Raft (forwarding на лидера)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Клиент
    participant F as Follower-нода
    participant L as Leader-нода
    participant Qrm as Кворум
    participant FSM as bbolt FSM

    Client->>F: PUT /v1/secret write
    F-->>Client: 503 not leader
    Client->>L: повтор на лидере
    L->>L: barrier encrypt команда put
    L->>Qrm: репликация Raft-лога
    Qrm-->>L: commit кворум
    L->>FSM: применить put к bbolt
    L-->>Client: 204 No Content
```

---

## 3.12. Ротация root-ключа без простоя

```mermaid
sequenceDiagram
    autonumber
    actor Op as Оператор
    participant Tuck as tuck server
    participant Seal as seal
    participant Bar as barrier

    Op->>Tuck: POST /v1/sys/rotate
    Tuck->>Seal: сгенерировать новый root key
    Seal->>Bar: re-wrap DEK новым root key
    Bar->>Bar: обновить barrier/keyring
    Note over Seal: shamir новые доли KMS новый ciphertext
    Tuck-->>Op: 200 новые доли при shamir
```

---

[← Назад: Архитектура](02-architecture.md) · [К оглавлению](README.md) · [Далее: API/CLI →](04-api-cli-reference.md)
