# 15 — PKI: выпуск TLS-сертификата

[← Все гайды](README.md)

## Цель

Выпустить X.509 сертификат от внутреннего CA Tuck для сервиса или mTLS.

## Шаги

### 1. Сгенерировать root CA (если ещё нет)

```powershell
curl -X POST "$env:TUCK_ADDR/v1/pki/root/generate/internal" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"common_name":"Tuck Internal CA","ttl":"87600h"}'
```

### 2. Создать роль выпуска

```powershell
curl -X POST "$env:TUCK_ADDR/v1/pki/roles/web-service" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "allowed_domains": ["*.example.com","example.com"],
    "allow_subdomains": true,
    "max_ttl": "720h"
  }'
```

### 3. Выпустить сертификат

```powershell
.\tuckcli.exe pki issue web-service --cn=api.example.com --ttl=720h
```

### 4. Установить на сервер

Сохраните `certificate` и `private_key` из JSON в файлы `tls.crt` / `tls.key`.

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| `pki issue` | cert + key + issuing_ca |
| openssl verify | цепочка валидна |
| `pki revoke SERIAL` | cert в CRL |

## Дальше

- [16 — Transit](16-transit.md)
- [17 — SSH](17-ssh.md)
