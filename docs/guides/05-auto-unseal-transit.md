# 05 — Auto-unseal (Transit)

[← Все гайды](README.md)

## Цель

Сервер автоматически распечатывается после рестарта без ручного ввода Shamir-ключей. Корневой ключ хранится зашифрованным ключом Transit другого Tuck-кластера (или Vault-совместимого сервиса).

## Кому подходит

SRE, у которых есть отдельный «корневой» Tuck/Transit, управляемый независимо от основного.

## Предварительные условия

- Инициализированный Tuck ([04 — Shamir](04-prod-shamir.md))
- Рабочий Transit-эндпоинт с ключом шифрования и токеном, у которого есть права `encrypt`/`decrypt` на этот ключ

## Шаги

### 1. Создать ключ в Transit

```bash
curl -X POST -H "X-Tuck-Token: $TOKEN" https://transit.example.com/v1/transit/keys/tuck-root
```

### 2. Запустить сервер с Transit seal

```bash
export TUCK_TRANSIT_TOKEN=<токен>
tuck --seal-type=transit \
  --seal-transit-addr=https://transit.example.com \
  --seal-transit-key=tuck-root \
  --seal-transit-key-file=/var/lib/tuck/tuck-transit.enc
```

Токен лучше передавать через переменную окружения `TUCK_TRANSIT_TOKEN`, а не флагом (виден в `ps`). Зашифрованный корневой ключ сохраняется в `--seal-transit-key-file`.

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

`sealed: false` без ручного `operator unseal`.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Рестарт pod/процесса | `sealed: false` за секунды |
| Логи сервера | сообщение `Transit seal` и успешный auto-unseal |
| Отзыв токена Transit | сервер остаётся sealed |

## Важно

- Shamir-ключи (или резервная копия) всё равно нужны как **recovery** при потере Transit-сервера.
- Доступность Transit становится зависимостью старта: при его недоступности сервер остаётся sealed.
- Не храните токен Transit в репозитории.

## Дальше

- [04 — Shamir](04-prod-shamir.md)
- [06 — Helm в K8s](06-kubernetes-helm.md)
