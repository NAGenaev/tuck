# 05 — Auto-unseal (облачный KMS)

[← Все гайды](README.md)

## Цель

Сервер автоматически распечатывается после рестарта без ручного ввода Shamir-ключей.

## Кому подходит

SRE в AWS / GCP / Azure.

## Предварительные условия

- Инициализированный Tuck ([04 — Shamir](04-prod-shamir.md))
- Доступ к KMS в облаке (IAM role / service account)

## Шаги (AWS KMS — пример)

### 1. Создать KMS key в AWS

Зафиксируйте Key ARN.

### 2. Настроить seal в конфиге

```hcl
seal "awskms" {
  region     = "eu-central-1"
  kms_key_id = "arn:aws:kms:eu-central-1:123456789:key/..."
}
```

### 3. Перезапустить сервер

Сервер при старте запросит KMS и выполнит auto-unseal.

### 4. Проверить

```powershell
.\tuckcli.exe status
```

`sealed: false` без ручного `operator unseal`.

## Другие провайдеры

| Провайдер | Блок seal в HCL |
|-----------|-----------------|
| GCP | `seal "gcpckms" { project=...; region=...; key_ring=...; crypto_key=... }` |
| Azure | `seal "azurekeyvault" { tenant_id=...; vault_name=...; key_name=... }` |
| Transit (другой Tuck) | `seal "transit" { address=...; token=...; key_name=... }` |

Точные поля — в `docs/` и примере `deploy/server/`.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Рестарт pod/процесса | `sealed: false` за секунды |
| Логи сервера | сообщение об успешном auto-unseal |
| Отзыв KMS доступа | сервер остаётся sealed |

## Важно

- Shamir-ключи всё равно нужны как **recovery** при потере KMS.
- Не храните cloud credentials в репозитории.

## Дальше

- [04 — Shamir](04-prod-shamir.md)
- [06 — Helm в K8s](06-kubernetes-helm.md)
