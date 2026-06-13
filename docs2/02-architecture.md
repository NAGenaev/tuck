# 02 — Архитектура и карта функций

[← Назад: Обзор](01-overview.md) · [К оглавлению](README.md) · [Далее: Sequence-диаграммы →](03-sequence-diagrams.md)

---

## 2.1. Архитектурные принципы

Эти принципы зафиксированы в проекте как «не нарушать»:

1. **Один бинарь** — никаких внешних runtime-зависимостей (хранилище встроенное).
2. **Всё через барьер** — plaintext никогда не касается физического хранилища.
3. **Узкий `barrierIface`** — каждый движок видит только `Get/Put/Delete/List`, никогда — полный `physical.Backend`.
4. **Единый паттерн dynamic-движков** — `config CRUD + role CRUD + creds + lease CRUD`.
5. **Фоновый GC каждые 15 минут** — истёкшие токены / leases / wrapping-токены.
6. **Deny-правила побеждают** — deny в любой политике блокирует, независимо от allow.
7. **Ключевой материал зануляется** после использования.
8. **Audit перед операцией** — fail-closed: нет audit → нет операции.

---

## 2.2. Слоистая архитектура (C4 — уровень контейнеров/слоёв)

```mermaid
flowchart TB
    subgraph clients["Клиенты"]
        CLI[tuckcli]
        SDK["Go SDK pkg/client"]
        CURL["curl / HTTP"]
        OP[tuck-operator]
        INJ["tuck-agent / injector"]
        UIc["Web Dashboard /ui/"]
    end

    subgraph api["Слой API internal/api"]
        TLS[TLS termination]
        MW["Middleware rate-limit audit metrics"]
        ROUTES["Маршрутизатор net/http 194+ роутов"]
    end

    subgraph core["Ядро internal/core"]
        AUTH["Authenticate / TrackUse"]
        ACL["EnforceAccess policy glob deny"]
        ORCH["Оркестрация движков и хранилищ"]
    end

    subgraph engines["Движки"]
        KV["KV v1/v2 Cubbyhole Wrapping"]
        AUTHENG["Auth token k8s jwt approle ldap"]
        DYN["Dynamic db aws gcp azure"]
        CRYPTO["Crypto pki transit ssh totp"]
        IDENT["Identity Namespaces Mounts Plugins"]
    end

    subgraph barrier["Barrier internal/barrier"]
        SEAL["Seal/Unseal state"]
        ENC["AES-256-GCM encrypt/decrypt"]
    end

    subgraph physical["Physical internal/physical"]
        BBOLT[("bbolt single file")]
        RAFT[("Raft FSM HA 3-5 нод")]
    end

    SEALPROV["Seal backends dev shamir transit KMS"]

    clients --> TLS --> MW --> ROUTES --> AUTH --> ACL --> ORCH
    ORCH --> KV
    ORCH --> AUTHENG
    ORCH --> DYN
    ORCH --> CRYPTO
    ORCH --> IDENT
    KV --> ENC
    AUTHENG --> ENC
    DYN --> ENC
    CRYPTO --> ENC
    IDENT --> ENC
    SEALPROV -.->|root key| SEAL
    ENC --> BBOLT
    ENC --> RAFT
```

---

## 2.3. Карта пакетов (структура кода)

```mermaid
flowchart LR
    subgraph cmd["cmd точки входа"]
        c1["tuck сервер"]
        c2["tuckcli CLI"]
        c3[tuck-operator]
        c4[tuck-injector]
        c5[tuck-agent]
        c6[tuckcsi]
    end

    subgraph internal["internal ядро"]
        i_api["api HTTP роутинг"]
        i_core["core оркестрация"]
        i_barrier["barrier AES-256-GCM"]
        i_seal["seal root key"]
        i_shamir["shamir GF256"]
        i_phys["physical + raft"]
        i_token["token модель"]
        i_policy["policy ACL"]
        i_kvv2["kvv2 / kvsecret"]
        i_cub["cubbyhole / wrapping"]
        i_auth["auth jwt approle ldap"]
        i_dyn["dynamic aws gcp azure db pki"]
        i_id["identity namespace mount"]
        i_obs["audit metrics telemetry"]
        i_k8s["k8s operator injector csi"]
    end

    subgraph pkg["pkg"]
        p_client["client Go SDK"]
    end

    cmd --> internal
    pkg --> i_api
```

### Назначение ключевых пакетов

| Пакет | Ответственность |
|-------|-----------------|
| `internal/api` | HTTP-слой: 194+ роутов, middleware (audit, rate-limit, metrics), сериализация |
| `internal/core` | Оркестрация: аутентификация, проверка доступа, маршрутизация в движки |
| `internal/barrier` | Криптобарьер AES-256-GCM, состояние sealed/unsealed |
| `internal/seal` | Жизненный цикл root key: dev / shamir / transit / awskms / gcpkms / azurekv |
| `internal/shamir` | Shamir's Secret Sharing над GF(256): Split / Combine |
| `internal/physical` | Физический слой: bbolt (single file), in-memory (тесты) |
| `internal/physical/raft` | Raft-реплицируемый backend (HA 3–5 нод) |
| `internal/token` | Модель токенов: генерация, TTL, accessor, roles, MaxUses |
| `internal/policy` | ACL: glob-сопоставление путей, capability-проверки, deny-first |
| `internal/kvv2`, `kvsecret` | KV v2 (версии/CAS/soft-delete) и KV v1 |
| `internal/cubbyhole`, `wrapping` | Приватное хранилище токена; одноразовые wrapping-токены |
| `internal/auth/*` | JWT/OIDC, AppRole, LDAP/AD, GitHub OIDC |
| `internal/dynamic/*` | 8 движков: aws, gcp, azure, database, pki, transit, ssh, totp |
| `internal/identity` | Entities, aliases, groups, group-aliases |
| `internal/namespace` | Изоляция по неймспейсам (мультиарендность) |
| `internal/mount`, `plugin` | Mount table; каталог плагинов |
| `internal/replication` | WAL и режимы primary/secondary |
| `internal/audit` | Tamper-evident лог (SHA-256 hash chain), audit sinks |
| `internal/metrics`, `telemetry` | Prometheus; OpenTelemetry (OTLP) |
| `internal/ratelimit` | Per-IP и per-token token-bucket |
| `internal/k8s`, `operator`, `injector`, `csi` | K8s TokenReview; CRD-контроллер; webhook; CSI |
| `internal/tlsutil`, `ui`, `config`, `sysconfig` | TLS-хелперы; embedded дашборд; конфиг-файл; runtime-конфиг |
| `pkg/client` | Типизированный Go SDK |

---

## 2.4. Карта функций ядра (`core.Core`)

`core.Core` — центральный фасад. Ниже сгруппированы публичные методы (карта функций).

```mermaid
flowchart TB
    ROOT(("core.Core"))
    subgraph LC["Жизненный цикл"]
        LC1[Start / Seal / UnsealShard / StartGC]
    end
    subgraph AA["Аутентификация и авторизация"]
        AA1[Authenticate / TrackUse / EnforceAccess]
    end
    subgraph TK["Токены"]
        TK1[CreateToken / Lookup / Renew / Revoke]
        TK2[Accessor / Token Roles]
    end
    subgraph PL["Политики"]
        PL1[PutPolicy / GetPolicy / ListPolicies]
    end
    subgraph KV["KV-секреты"]
        KV1[GetSecret / PutSecret / DeleteSecret]
        KV2[KVv2 store]
    end
    subgraph AE["Auth-движки"]
        AE1[LoginK8s / LoginJWT / LoginGitHub]
        AE2[AppRole / LDAP]
    end
    subgraph CE["Crypto-движки"]
        CE1[Transit encrypt/decrypt/sign]
        CE2[PKI issue/revoke / SSH sign / TOTP]
    end
    subgraph OP["Операции"]
        OP1[RotateKey / Snapshotter / ClusterBackend / WAL]
    end
    ROOT --> LC
    ROOT --> AA
    ROOT --> TK
    ROOT --> PL
    ROOT --> KV
    ROOT --> AE
    ROOT --> CE
    ROOT --> OP
```

---

## 2.5. Криптографическая модель (envelope encryption)

```mermaid
flowchart TD
    SEALBK["Seal backend dev shamir transit KMS"] -->|предоставляет| RK["root key 32 байта в памяти"]
    RK -->|AES-256-GCM wrap| DEK["barrier DEK ciphertext в keyring"]
    DEK -->|AES-256-GCM + nonce| DATA["записи данных"]
    DATA --> PHYS[("Physical backend только шифртекст")]
```

**Суть:** root key шифрует DEK; DEK шифрует записи. При ротации меняется только обёртка DEK — данные не перешифровываются. На рестарте: `seal.Unseal()` → root key → `barrier.Unseal()` → DEK расшифрован → сервер готов.

---

## 2.6. Типы seal (распечатывание)

```mermaid
flowchart LR
    subgraph dev["dev"]
        d1["root key в файле автостарт DEV ONLY"]
    end
    subgraph shamir["shamir"]
        s1["root key N долей K для unseal"]
    end
    subgraph transit["transit"]
        t1["root key завёрнут Transit auto-unseal"]
    end
    subgraph kms["awskms gcpkms azurekv"]
        k1["root key зашифрован KMS auto-unseal"]
    end
```

| Тип | Кейс | Распечатывание | Креды |
|-----|------|----------------|-------|
| `dev` | Локальная разработка | Авто (plaintext-файл) | — |
| `shamir` | On-prem, multi-operator | Ручное, K-of-N долей | — |
| `transit` | Облако, есть внешний Vault/Transit | Авто | токен Transit (через env) |
| `awskms` | EKS / EC2 | Авто | IRSA / instance role |
| `gcpkms` | GKE | Авто | Workload Identity / ADC |
| `azurekv` | AKS / Azure | Авто | Managed Identity / DefaultAzureCredential |

---

## 2.7. Раскладка логических ключей в хранилище

Все значения зашифрованы барьером. Примеры логических путей:

| Логический ключ | Содержимое |
|-----------------|------------|
| `barrier/keyring` | DEK, зашифрованный root key |
| `auth/token/<hash>` | JSON-запись токена (включает accessor) |
| `auth/accessor/<accessor>` | Индекс accessor → token ID |
| `auth/policy/<name>` | JSON политики |
| `auth/<method>/...` | Конфиги/роли auth-движков (k8s, jwt, approle, ldap, github) |
| `secret/<path>` | KV v1 значение |
| `kvv2/<path>/meta`, `kvv2/<path>/v/<n>` | KV v2 метаданные и версии |
| `dynamic/<engine>/config` | Конфиг dynamic-движка (секреты внутри зашифрованы) |
| `dynamic/<engine>/roles/<name>` | Роль движка |
| `dynamic/<engine>/leases/<id>` | Lease выданных кредов |
| `dynamic/pki/ca`, `.../certs/<serial>` | CA + записи сертификатов (без приватных ключей leaf) |
| `dynamic/transit/keys/<name>` | Все версии Transit-ключа |
| `dynamic/ssh/ca`, `dynamic/totp/keys/<name>` | SSH CA; TOTP-секреты |
| `sys/wrapping/<id>` | Запись wrapping-токена |
| `cubbyhole/<token_id>/<path>` | Приватное хранилище токена |
| `audit/last_hash` | Последний хеш цепочки аудита |

---

## 2.8. Карта движков секретов

```mermaid
flowchart TB
    subgraph static["Статические секреты"]
        KV1[KV v1]
        KV2["KV v2 версии CAS"]
        CUB[Cubbyhole]
        WRAP[Response Wrapping]
    end
    subgraph dyn["Динамические секреты"]
        DB[Database PG/MySQL]
        AWS[AWS IAM/STS]
        GCP[GCP SA-key/OAuth2]
        AZ[Azure AD secrets]
    end
    subgraph cr["Криптография как сервис"]
        PKI["PKI X.509 CA"]
        TR["Transit encrypt/sign"]
        SSH["SSH CA certs"]
        TOTP["TOTP OTP"]
    end
    BAR[(Barrier)]
    EXT["AWS GCP Azure DB"]
    static --> BAR
    dyn --> BAR
    cr --> BAR
    dyn -.-> EXT
```

**Единый паттерн dynamic-движков (11 эндпоинтов):** `config` (PUT/GET/DELETE/LIST) + `roles` (PUT/GET/DELETE/LIST) + `creds` (POST) + `lease` (GET/DELETE/LIST). Это делает API предсказуемым и упрощает обучение.

---

## 2.9. Карта аутентификации и авторизации

```mermaid
flowchart LR
    subgraph methods["Auth-методы"]
        T[Token]
        K["K8s SA TokenReview"]
        J["JWT/OIDC JWKS"]
        A["AppRole role_id+secret_id"]
        L["LDAP/AD bind+search"]
        G[GitHub OIDC]
    end
    methods -->|login| ISSUE["Выпуск Tuck-токена"]
    ISSUE --> REQ["Запрос X-Tuck-Token"]
    REQ --> AUTHN["Authenticate"]
    AUTHN --> USE["TrackUse MaxUses"]
    USE --> AUTHZ["EnforceAccess glob"]
    AUTHZ --> DENY{"deny matched?"}
    DENY -->|да| BLOCK["403 Forbidden"]
    DENY -->|нет| ALLOW{"allow matched?"}
    ALLOW -->|да| OK["Операция OK"]
    ALLOW -->|нет| BLOCK
```

**Модель ACL:** двухпроходная — сначала deny-проход (любой совпавший deny → отказ), затем allow-проход. Capabilities: `read`, `write`, `delete`, `list`, `deny`. Glob: `secret/db/*` совпадает с `secret/db/password`, но не с `secret/db/sub/key`; `secret/**` — рекурсивно. Root-политика совпадает со всем.

---

## 2.10. HA: Raft-кластер

```mermaid
flowchart LR
    subgraph n1["Нода 1 leader"]
        s1[tuck server]
        f1[(fsm.db)]
        r1[(raft.db)]
    end
    subgraph n2["Нода 2 follower"]
        s2[tuck server]
        f2[(fsm.db)]
    end
    subgraph n3["Нода 3 follower"]
        s3[tuck server]
        f3[(fsm.db)]
    end
    CL[Клиент] -->|write| s1
    CL -->|write follower| s2
    s2 -.->|503 retry| CL
    s1 ==>|Raft log| s2
    s1 ==> s3
```

- Все записи идут через Raft-лог (leader → кворум большинства → commit).
- Реплицируется **только шифртекст** AES-256-GCM.
- FSM на bbolt применяет committed-команды `put`/`delete`.
- Запись на follower → `503 not leader`, клиент повторяет к лидеру.
- Онлайн-изменение состава: `AddVoter` / `RemoveServer` через HTTP API лидера.

---

## 2.11. Kubernetes-интеграция

```mermaid
flowchart TB
    subgraph operator["Поток оператора"]
        CRD["TuckSecret CRD"] --> CTRL["operator controller Lease"]
        CTRL -->|K8s SA login| TUCK1[(Tuck server)]
        TUCK1 -->|GET secret| CTRL
        CTRL -->|apply| KSEC["K8s Secret"]
    end
    subgraph inject["Поток инжектора"]
        POD["Pod tuck.io/inject"] --> WH["MutatingWebhook"]
        WH -->|JSON Patch| POD2["Pod с init и tmpfs"]
        POD2 --> AGENT["tuck-agent init"]
        AGENT -->|fetch secrets| TUCK2[(Tuck server)]
        AGENT -->|write 0400| VOL["/tuck/secrets/"]
    end
```

**Оператор**: синхронизирует секреты из Tuck в нативные K8s Secret; кэширует Tuck-токен (TTL ~4 мин, обновление за 30с); только лидер реконсайлит; status conditions (`Synced`, `Ready`); финализатор при `deletionPolicy: Delete`.

**Инжектор**: webhook добавляет в Pod `emptyDir{medium: Memory}` (tmpfs) + init-контейнер `tuck-agent`, который атомарно пишет секреты (mode `0400`) до старта app-контейнеров. Секреты живут только в памяти Pod — **etcd не затрагивается**.

---

## 2.12. Наблюдаемость

| Канал | Эндпоинт / механизм | Содержимое |
|-------|---------------------|------------|
| Метрики | `GET /metrics` | счётчики/гистограммы по route+status, `tuck_sealed`, auth success/fail, uptime |
| Трейсинг | OTLP exporter | спаны HTTP-запросов (опционально) |
| Аудит | hash-chain лог + sinks (file/stdout/webhook/syslog) | кто (accessor), что (path+capability), когда, результат; **значения секретов не логируются** |
| Логи | structured slog (JSON), request-id | без секретов |
| Health | `/v1/health`, `/v1/sys/ready`, `/v1/sys/seal-status` | liveness/readiness/seal-состояние |

**Аудит — tamper-evident:** `hash_n = SHA256(hash_{n-1} || entry_json)`. Разрыв цепочки детектируется. Запись в аудит — перед операцией (fail-closed).

---

[← Назад: Обзор](01-overview.md) · [К оглавлению](README.md) · [Далее: Sequence-диаграммы →](03-sequence-diagrams.md)
