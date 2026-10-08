# 23 — Web UI (дашборд)

[← Все гайды](README.md)

## Цель

Управлять секретами, политиками и статусом кластера через браузер без CLI.

## Предварительные условия

- Tuck server (UI встроен в бинарь и отдаётся с `/ui/`, отдельно ставить не нужно)
- Token с нужными правами (не обязательно root)

## Шаги

### 1. Открыть UI

**Локальный dev:**

```
https://127.0.0.1:8200/ui/
```

Сервер запущен как `.\tuck.exe -tls-auto` ([01](01-pervyy-sekret.md)): примите в браузере предупреждение о самоподписанном сертификате. `TUCK_INSECURE` относится к `tuckcli`, а не к браузеру.

**Kubernetes:**

```powershell
kubectl -n tuck port-forward svc/tuck-server 8200:8200
```

Браузер: `http://127.0.0.1:8200/ui/` (в чарте по умолчанию TLS выключен).

### 2. Войти

- Вставьте token в поле **Token** и нажмите **Sign in**
- Или войдите через LDAP (логин и пароль, см. [12 — LDAP](12-ldap.md)) либо SSO/OIDC, если администратор их настроил ([11](11-jwt-oidc.md), [13](13-onboarding-team.md)). Права определяются ролью, подошедшей по группам пользователя

### 3. Основные разделы

| Раздел | Что делать |
|--------|------------|
| **Status** | Sealed/unsealed, версия, uptime |
| **Secrets** → KV v1 / KV v2 | Просмотр и запись секретов, версии KV v2 |
| **Secrets** → Wrapping | Wrap/unwrap, список и отзыв капсул |
| **Access** → Tokens, Token Roles | Выпуск и отзыв токенов, роли токенов |
| **Access** → Policies | Политики доступа |
| **Access** → Auth Methods | AppRole, JWT/OIDC, LDAP, Kubernetes |
| **Access** → Namespaces | Мультитенантность |
| **Engines** → Mounts | Смонтированные движки и их настройки |
| **Operations** → Cluster, Audit Sinks | Raft-узлы; приёмники аудита |

### 4. Типичный workflow

1. Access → Policies → создать `app-readonly`
2. Secrets → KV v1 → путь `myapp/...` → добавить секрет
3. Access → Tokens → создать токен с этой политикой

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Login с root | полный доступ |
| Login scoped | только разрешённые пути |
| Seal indicator | совпадает с `tuckcli status` |

## OpenShift Console (опционально)

Для просмотра TuckSecret CRD в консоли OpenShift на minikube есть скрипт `scripts\start-openshift-console.ps1` (см. шапку скрипта).

## Дальше

- CLI-эквиваленты: [04 — API/CLI справочник](../04-api-cli-reference.md)
- [01 — Первый секрет](01-pervyy-sekret.md)
