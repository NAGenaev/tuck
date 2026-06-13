# 23 — Web UI (дашборд)

[← Все гайды](README.md)

## Цель

Управлять секретами, политиками и статусом кластера через браузер без CLI.

## Предварительные условия

- Tuck server с включённым UI (по умолчанию в dev и Helm)
- Token с нужными правами (не обязательно root)

## Шаги

### 1. Открыть UI

**Локальный dev:**

```
https://127.0.0.1:8200/ui/
```

Примите предупреждение о self-signed TLS или используйте `TUCK_INSECURE`.

**Kubernetes:**

```powershell
kubectl -n tuck port-forward svc/tuck 8200:8200
```

Браузер: `http://127.0.0.1:8200/ui/`

### 2. Войти

- Вставьте token в поле **Token**
- Или используйте метод auth (LDAP / OIDC), если настроен

### 3. Основные разделы

| Раздел | Что делать |
|--------|------------|
| **Secrets** | Просмотр KV, создание путей |
| **Access** | Политики, auth methods, entities |
| **Tools** | Wrap/unwrap, rotate, seal status |
| **Status** | Sealed, HA peers, version |

### 4. Типичный workflow

1. Policies → создать `app-readonly`
2. Secrets → `secret/myapp/...` → Add secret
3. Access → Tokens → Create token с policy

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| Login с root | полный доступ |
| Login scoped | только разрешённые пути |
| Seal indicator | совпадает с `tuckcli status` |

## OpenShift Console (опционально)

Для просмотра TuckSecret CRD в familiar UI: [docs/MINIKUBE.md](../../docs/MINIKUBE.md) раздел 8.

## Дальше

- CLI-эквиваленты: [04 — API/CLI справочник](../04-api-cli-reference.md)
- [01 — Первый секрет](01-pervyy-sekret.md)
