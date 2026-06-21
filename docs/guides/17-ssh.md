# 17 — SSH: подпись сертификата

[← Все гайды](README.md)

## Цель

Войти на сервер по SSH с короткоживущим сертификатом, подписанным CA Tuck — без статического authorized_keys на каждой машине.

## Шаги

### 1. Настроить SSH engine

```powershell
curl -X POST "$env:TUCK_ADDR/v1/ssh/config/ca" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"generate_signing_key": true}'
```

### 2. Получить public key CA

```powershell
curl "$env:TUCK_ADDR/v1/ssh/public_key" -H "X-Tuck-Token: $env:TUCK_TOKEN"
```

Добавьте в `sshd_config` на серверах: `TrustedUserCAKeys /etc/ssh/tuck_ca.pub`.

### 3. Создать роль

```powershell
curl -X POST "$env:TUCK_ADDR/v1/ssh/roles/ops" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "key_type": "ca",
    "allow_user_certificates": true,
    "allowed_users": "ubuntu,admin",
    "default_extensions": {"permit-pty": ""},
    "ttl": "30m"
  }'
```

### 4. Сгенерировать ключ пользователя (локально)

```powershell
ssh-keygen -t ed25519 -f tuck_id -N ""
```

### 5. Подписать

```powershell
.\tuckcli.exe ssh sign ops tuck_id.pub --ttl=30m
```

### 6. Подключиться

```powershell
ssh -i tuck_id -i tuck_id-cert.pub ubuntu@server.example.com
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| ssh sign | signed_key в ответе |
| ssh login | успех без пароля |
| после TTL | Permission denied |

## Дальше

- [15 — PKI](15-pki.md)
- [02 — Политики](02-politiki-i-dostup.md)
