# Руководства пользователя Tuck

> Пошаговые инструкции: выбери сценарий → выполни шаги → получи результат.
> Каждый гайд заканчивается блоком **«Проверка»** с ожидаемым выводом.

**Общие переменные** (настрой один раз в терминале):

```powershell
$env:TUCK_ADDR  = "https://127.0.0.1:8200"   # или http://127.0.0.1:8200 в minikube
$env:TUCK_TOKEN = "tuck_..."                    # root или scoped token
```

Для `curl` на Windows с самоподписанным TLS добавляй `-k` (или используй `tuckcli`).

---

## Я хочу… (быстрый выбор)

| Задача | Гайд |
|--------|------|
| Быстро попробовать Tuck локально | [01 — Первый секрет](01-pervyy-sekret.md) |
| Ограничить доступ приложения (только read) | [02 — Политики и токены](02-politiki-i-dostup.md) |
| Версии секретов, откат, CAS | [03 — KV v2](03-kvv2.md) |
| Продакшн: Shamir, распечатывание после рестарта | [04 — Shamir seal](04-prod-shamir.md) |
| Auto-unseal через облачный KMS | [05 — Auto-unseal KMS](05-auto-unseal-kms.md) |
| Установить Tuck в Kubernetes | [06 — Helm в K8s](06-kubernetes-helm.md) |
| Синхронизировать секрет в K8s Secret (CRD) | [07 — TuckSecret оператор](07-tucksecret-operator.md) |
| Подложить секрет в Pod без etcd (tmpfs) | [08 — Webhook injector](08-webhook-injector.md) |
| Приложение в K8s логинится само (SA auth) | [09 — Kubernetes SA auth](09-kubernetes-auth.md) |
| CI/CD: машина-машина (AppRole) | [10 — AppRole](10-approle.md) |
| Логин через OIDC / JWT | [11 — JWT / OIDC](11-jwt-oidc.md) |
| Логин через LDAP / Active Directory | [12 — LDAP](12-ldap.md) |
| Временные креды PostgreSQL/MySQL | [13 — Database dynamic](13-dynamic-database.md) |
| Временные креды AWS / GCP / Azure | [14 — Cloud dynamic](14-dynamic-cloud.md) |
| Выпустить TLS-сертификат | [15 — PKI](15-pki.md) |
| Шифровать данные без выдачи ключа | [16 — Transit](16-transit.md) |
| SSH-сертификат через CA | [17 — SSH](17-ssh.md) |
| 2FA / TOTP | [18 — TOTP](18-totp.md) |
| Бэкап и восстановление | [19 — Backup / restore](19-backup-restore.md) |
| HA-кластер Raft (3 ноды) | [20 — Raft HA](20-ha-raft.md) |
| Безопасно передать секрет одноразовым токеном | [21 — Response Wrapping](21-response-wrapping.md) |
| Приватное хранилище одного токена | [22 — Cubbyhole](22-cubbyhole.md) |
| Работа через веб-интерфейс | [23 — Web UI](23-web-ui.md) |

---

## По роли

### Разработчик
- [01 Первый секрет](01-pervyy-sekret.md)
- [02 Политики и токены](02-politiki-i-dostup.md)
- [08 Webhook injector](08-webhook-injector.md)
- [16 Transit](16-transit.md)

### DevOps / Platform Engineer
- [06 Helm в K8s](06-kubernetes-helm.md)
- [07 TuckSecret](07-tucksecret-operator.md)
- [04 Shamir](04-prod-shamir.md) · [05 Auto-unseal](05-auto-unseal-kms.md)
- [19 Backup](19-backup-restore.md) · [20 Raft HA](20-ha-raft.md)

### Security / IAM
- [02 Политики и токены](02-politiki-i-dostup.md)
- [09 K8s SA auth](09-kubernetes-auth.md)
- [10 AppRole](10-approle.md) · [11 JWT](11-jwt-oidc.md) · [12 LDAP](12-ldap.md)
- [21 Response Wrapping](21-response-wrapping.md)

### SRE / DBA
- [13 Database dynamic](13-dynamic-database.md)
- [14 Cloud dynamic](14-dynamic-cloud.md)
- [19 Backup / restore](19-backup-restore.md)

---

## Справка

| Ресурс | Где |
|--------|-----|
| Архитектура, диаграммы | [../02-architecture.md](../02-architecture.md) |
| Полный справочник API/CLI | [../04-api-cli-reference.md](../04-api-cli-reference.md) |
| Minikube на Windows | [../../docs/MINIKUBE.md](../../docs/MINIKUBE.md) |
| E2E демо operator | [../../docs/E2E-DEMO.md](../../docs/E2E-DEMO.md) |
| Runbook (инциденты) | [../../docs/RUNBOOK.md](../../docs/RUNBOOK.md) |

---

[← К документации docs2](../README.md)
