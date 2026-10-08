# 04 — Shamir seal (продакшн)

[← Все гайды](README.md)

## Цель

Запустить Tuck с Shamir seal: корневой ключ делится на N долей, для распечатывания нужно K из них. Ни одна доля не хранится на сервере.

## Кому подходит

SRE, platform engineer.

## Предварительные условия

- Постоянный том для файла данных (`-data`) и файла `tuck-shamir.json`
- TLS-сертификат внутреннего CA (`-tls-cert` / `-tls-key`); `-tls-auto` (самоподписанный) только для стенда
- Бинарь `tuck` не имеет подкоманд: всё настраивается флагами или YAML-конфигом (`--config`)

## Шаги

### 1. Первый запуск (инициализация)

Отдельной команды `init` нет: инициализация происходит при первом старте, если файла `tuck-shamir.json` ещё нет.

```powershell
.\tuck.exe -addr 0.0.0.0:8200 `
  -data D:\tuck\data\tuck.db `
  -tls-cert D:\tuck\tls.crt -tls-key D:\tuck\tls.key `
  -seal-type shamir `
  -seal-shamir-config D:\tuck\data\tuck-shamir.json `
  -seal-shamir-n 5 -seal-shamir-k 3
```

То же в YAML (`.\tuck.exe --config tuck.yaml`):

```yaml
addr: 0.0.0.0:8200
data: D:\tuck\data\tuck.db
tls:
  cert: D:\tuck\tls.crt
  key: D:\tuck\tls.key
seal:
  type: shamir
  shamir:
    config: D:\tuck\data\tuck-shamir.json
    n: 5
    k: 3
```

В логе **один раз** печатаются `ROOT TOKEN` и N долей (`SHAMIR SHARES`). Больше их не показать нигде: перенесите в разные сейфы (разным людям) и очистите лог. После инициализации сервер уже распечатан. Разрешено `2 <= k <= n <= 255`.

В `tuck-shamir.json` лежат только `{"n":5,"k":3}`, ключевого материала там нет. Файл нужен при перезапуске; при его потере сервер примет флаги `-seal-shamir-n/-k` за первый запуск и не откроет существующие данные.

### 2. Настроить CLI

```powershell
$env:TUCK_ADDR  = "https://tuck.example.com:8200"
$env:TUCK_TOKEN = "tuck_..."   # root token из лога
```

### 3. После перезапуска сервера: распечатать

После каждого рестарта сервер стартует запечатанным. Подайте K долей (в примере 3 из 5), каждая отдельной командой, можно от разных операторов:

```powershell
.\tuckcli.exe unseal <доля-1>
.\tuckcli.exe unseal <доля-2>
.\tuckcli.exe unseal <доля-3>
```

Первые ответы показывают прогресс (`received_shards` из `required_shards`), последний: `"message": "unseal complete"`. Подать ту же долю дважды нельзя (`duplicate shard`). Команда не требует токена.

### 4. Проверить статус

```powershell
.\tuckcli.exe status
```

Ожидается `"sealed": false`. Пока сервер запечатан, видно `"sealed": true` и `required_shards`. Для проб готовности есть `GET /v1/sys/ready` (`{"ready":true,"sealed":false}`) и `GET /v1/health`.

### 5. Запечатать вручную

```powershell
.\tuckcli.exe seal
```

Ключ стирается из памяти, запросы получают `503 sealed`. Чтобы вернуть сервер в работу, снова подайте K долей (перезапуск не нужен).

### 6. Сменить корневой ключ и доли

Если доля скомпрометирована или состав операторов поменялся:

```powershell
.\tuckcli.exe rotate
```

Нужен root-токен. В ответе приходят **новые** N долей (поле `shares`), старые перестают работать. Раздайте новые доли сразу, ответ больше не повторится. Данные при этом не перешифровываются.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `status` после рестарта | `"sealed": true`, `"required_shards": 3` |
| `unseal` ×2 | `received_shards: 2`, всё ещё `sealed: true` |
| `unseal` третьей долей | `unseal complete`, `status` показывает `sealed: false` |
| `kv put` пока sealed | `HTTP 503: {"error":"sealed"}` |
| `kv get` после unseal | данные на месте |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| Потеряно больше N−K долей | Без K долей данные **не восстановить** |
| Доли или root token попали в лог/чат/git | Отзовите токен (`token revoke`), выполните `rotate`, раздайте новые доли |
| `duplicate shard` | Эта доля уже подана в текущем раунде |
| Каждый рестарт требует K долей | Это цель Shamir; для автоматики см. [05 — Auto-unseal Transit](05-auto-unseal-transit.md) |

## Дальше

- [05 — Auto-unseal](05-auto-unseal-transit.md)
- [20 — Raft HA](20-ha-raft.md)
