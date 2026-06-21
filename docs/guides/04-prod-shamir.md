# 04 — Shamir seal (продакшн)

[← Все гайды](README.md)

## Цель

Запустить Tuck в продакшн-режиме: инициализация, распределённые ключи распечатывания, ручной unseal после рестарта.

## Кому подходит

SRE, platform engineer.

## Предварительные условия

- Сервер **без** флага `-dev`
- Постоянное хранилище (volume) для `data/`
- TLS-сертификат (не self-signed в prod)

## Шаги

### 1. Запуск сервера

```powershell
.\tuck.exe server -config tuck.hcl
```

Пример `tuck.hcl`:

```hcl
listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_cert_file = "/etc/tuck/tls.crt"
  tls_key_file  = "/etc/tuck/tls.key"
}

storage "file" {
  path = "/var/lib/tuck/data"
}
```

### 2. Инициализация (один раз)

```powershell
.\tuckcli.exe operator init -key-shares=5 -key-threshold=3
```

Сохраните **unseal keys** и **initial root token** в безопасное место (разные люди / сейф).

### 3. Распечатать (unseal)

Нужно `threshold` ключей (в примере — 3 из 5):

```powershell
.\tuckcli.exe operator unseal
# введите unseal key 1

.\tuckcli.exe operator unseal
# введите unseal key 2

.\tuckcli.exe operator unseal
# введите unseal key 3
```

### 4. Проверить статус

```powershell
.\tuckcli.exe status
```

Ожидается: `sealed: false`, `initialized: true`.

### 5. После рестарта сервера

Повторите шаг 3 (unseal) — данные на диске, но в памяти зашифрованы пока не unseal.

### 6. (Опционально) Запечатать вручную

```powershell
.\tuckcli.exe seal
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `status` до unseal | `sealed: true` |
| после 3 ключей | `sealed: false` |
| `kv put` после unseal | 204 / успех |
| `kv put` пока sealed | ошибка sealed |

## Частые ошибки

| Симптом | Решение |
|---------|---------|
| Потеряны unseal keys | Без threshold ключей данные **не восстановить** |
| Root token в git | Отзовите и выпустите новый через unseal + rotate |
| Каждый рестарт — unseal | Настройте [05 — Auto-unseal KMS](05-auto-unseal-kms.md) |

## Дальше

- [05 — Auto-unseal](05-auto-unseal-kms.md)
- [20 — Raft HA](20-ha-raft.md)
