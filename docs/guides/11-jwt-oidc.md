# 11 — JWT / OIDC login

[← Все гайды](README.md)

## Цель

Пользователь логинится в Tuck по JWT от IdP (Keycloak, Azure AD, Google) и получает token с политиками.

> Настройка JWT делается через HTTP API (в `tuckcli` есть только `auth jwt login`). В PowerShell `curl` это алиас `Invoke-WebRequest`, поэтому `curl.exe`. JSON-тело кладём в файл: PS 5.1 съедает двойные кавычки в аргументах, а pipe может дописать BOM. Самоподписанный TLS: `-k` у curl и `TUCK_INSECURE=1` у `tuckcli`.

## Шаги

### 1. Настроить JWT auth

`PUT`, обязательное поле `jwks_uri` (URL набора ключей IdP, обычно HTTPS). Остальное: `issuer` (ожидаемый `iss`), `audience` (ожидаемый `aud`), `default_ttl`.

```powershell
[IO.File]::WriteAllText("$PWD\jwt-config.json", '{"jwks_uri":"https://keycloak.example.com/realms/myrealm/protocol/openid-connect/certs","issuer":"https://keycloak.example.com/realms/myrealm","audience":"tuck","default_ttl":"1h"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/auth/jwt/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@jwt-config.json"
```

Ответ `{"status":"ok"}`. Токен IdP обязан иметь заголовок `kid`, claim `exp`, и подпись, проверяемую ключами из `jwks_uri`. Браузерный OIDC-вход в UI настраивается теми же `PUT /config` полями `oidc_*`.

### 2. Создать роль

Роль связывает claims токена с политиками Tuck (политика `app-readonly` из [02](02-politiki-i-dostup.md)). Поля: `policies`, `ttl`, `bound_subject`, `bound_claims`, `bound_audiences`, `bound_groups` (подходит любая из групп в claim `groups`), `tuck_namespace`.

```powershell
[IO.File]::WriteAllText("$PWD\jwt-role.json", '{"policies":["app-readonly"],"bound_audiences":["tuck"],"bound_groups":["devs"],"ttl":"1h"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/auth/jwt/role/developer" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@jwt-role.json"
```

### 3. Логин

```powershell
.\tuckcli.exe auth jwt login --jwt=eyJhbG...
```

Роль из запроса не выбирается: сервер берёт **первую** роль, чьи условия (`bound_*`) совпали с claims токена. Флаг `--role` принимается, но игнорируется. Ответ: `{"token":"tuck_...","policies":[...],"expires_at":"..."}`.

### 4. Проверить token

```powershell
$env:TUCK_TOKEN = "tuck_..."   # token из login
.\tuckcli.exe token lookup-self
```

## Проверка

| Действие | Ожидаемый результат |
|----------|---------------------|
| login с валидным JWT | `token` |
| login с чужим issuer, просроченным или без `kid` токеном | 401 `invalid or expired JWT` |
| login валидным токеном, но ни одна роль не подошла | 403 `no matching role for token claims` |
| lookup-self | `policies` из роли |

## Вход в UI через SSO (OIDC)

Сотрудники входят в веб-интерфейс через корпоративного провайдера (Keycloak, Azure AD, Okta и т. п.) без вставки токена. Это тот же JWT-провайдер и те же JWT-роли: после обмена кода Tuck проверяет `id_token` и подбирает роль так же, как при `auth jwt login`.

```
браузер ── GET /v1/auth/oidc/auth-url ──► Tuck: создаёт state, nonce, PKCE-verifier (5 минут), отдаёт URL
браузер ── редирект на провайдера ─────► вход пользователя
браузер ◄─ редирект /ui/auth/oidc/callback?code=…&state=… ─
браузер ── POST /v1/auth/oidc/callback {code,state} ─► Tuck: берёт и удаляет state, меняет code на id_token
                                                       (с PKCE-verifier), проверяет подпись/iss/aud/exp/nonce,
                                                       подбирает роль, выдаёт токен Tuck
```

### 1. Клиент у провайдера

Создайте OIDC-клиент (тип authorization code):

- **Redirect URI:** `https://<адрес-Tuck>/ui/auth/oidc/callback` (ровно этот адрес, он же значение `oidc_redirect_uri`).
- **PKCE:** метод S256 (Tuck всегда его использует).
- **Client ID** и, если клиент конфиденциальный, **client secret**. Публичный клиент (без секрета) тоже работает, защитой остаётся PKCE.
- **Группы в токене:** добавьте в `id_token` claim `groups` (в Keycloak: mapper «Group Membership» без полного пути; в Azure AD: groups claim). Имя claim фиксировано: `groups`.
- **Scopes:** `openid profile groups` (имена scope у провайдеров разные, ориентируйтесь на документацию провайдера).

### 2. Настроить Tuck

Поля `oidc_*` добавляются к обычной конфигурации JWT тем же `PUT /v1/auth/jwt/config` (или в UI: «Методы аутентификации», JWT / OIDC). Сохранение заменяет конфигурацию целиком, поэтому передавайте и `jwks_uri` с `issuer`.

```powershell
[IO.File]::WriteAllText("$PWD\jwt-config.json", '{"jwks_uri":"https://idp.example.com/realms/corp/protocol/openid-connect/certs","issuer":"https://idp.example.com/realms/corp","default_ttl":"8h","oidc_authorization_url":"https://idp.example.com/realms/corp/protocol/openid-connect/auth","oidc_token_url":"https://idp.example.com/realms/corp/protocol/openid-connect/token","oidc_client_id":"tuck","oidc_client_secret":"<секрет клиента>","oidc_redirect_uri":"https://tuck.example.com/ui/auth/oidc/callback","oidc_scopes":"openid profile groups"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/auth/jwt/config" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" -H "Content-Type: application/json" `
  --data-binary "@jwt-config.json"
```

| Поле | Назначение |
|------|------------|
| `oidc_authorization_url`, `oidc_token_url` | Эндпоинты провайдера. Только `https` (`http` допустим для `localhost`) |
| `oidc_client_id` | Client ID. Если `audience` не задан, `aud` у `id_token` обязан совпасть с ним |
| `oidc_client_secret` | Секрет клиента. Не возвращается в `GET` (там только `oidc_client_secret_set`). Не передан в `PUT`: прежний сохраняется. Пустая строка: удаляется |
| `oidc_redirect_uri` | Адрес возврата, как у провайдера |
| `oidc_scopes` | Через пробел; `openid` добавляется всегда. По умолчанию `openid profile` |
| `ca_cert` | PEM-цепочка внутреннего CA банка, которым подписан сертификат провайдера. Нужна, если Keycloak/ADFS работает на сертификате внутреннего УЦ: Tuck использует её для JWKS и для token endpoint и доверяет **только** ей. Пусто: системное хранилище. Не передан в `PUT`: прежний сохраняется. Пустая строка: удаляется |

Вход через SSO включается, как только заданы `oidc_authorization_url`, `oidc_token_url`, `oidc_client_id`, `oidc_redirect_uri` (а также `jwks_uri` и `issuer`). Страница входа сама покажет вкладку «SSO» (`GET /v1/auth/oidc/status`). Если поля `oidc_*` не заданы, JWT-вход работает как раньше.

Имя токена Tuck, выданного при входе через провайдера, берётся из claim `preferred_username` (иначе `email`, иначе `sub`): `jwt:anna.sso`. Оно же попадает в `identity` журнала аудита, поэтому в записях видно человека, а не UUID провайдера.

> **Отзыв доступа.** Tuck не может спросить провайдера, остаётся ли человек в группе, поэтому токен SSO живёт до конца `ttl` роли (в отличие от токенов LDAP, которые Tuck сам перепроверяет по каталогу). Для ролей SSO задавайте короткий `ttl` и при необходимости отзывайте токены вручную: `tuckcli token revoke <id>`.

### 3. Роли с привязкой к группам

Роль привязывает группы провайдера к политикам и namespace Tuck:

```powershell
[IO.File]::WriteAllText("$PWD\jwt-role.json", '{"bound_groups":["team-a-devs"],"policies":["team-a-rw"],"ttl":"8h","tuck_namespace":"team-a"}')
curl.exe -k -X PUT "$env:TUCK_ADDR/v1/auth/jwt/role/team-a-devs" `
  -H "X-Tuck-Token: $env:TUCK_TOKEN" -H "Content-Type: application/json" `
  --data-binary "@jwt-role.json"
```

`bound_groups` подходит, если в claim `groups` есть **любая** из перечисленных групп. Условия `bound_subject`, `bound_claims`, `bound_audiences` и `bound_groups` внутри роли объединяются по И. Токен выдаётся ровно на один namespace из `tuck_namespace` (политики и namespace должны существовать).

> **Роль без привязок подходит любому пользователю провайдера.** Первая подошедшая роль (в алфавитном порядке имён) побеждает, так что широкая роль без `bound_*` перекроет остальные и даст доступ всем, кто вошёл у провайдера. Для SSO всегда задавайте `bound_groups` или `bound_claims`.

### 4. Вход

Откройте страницу входа Tuck, вкладка «SSO», кнопка «Войти через SSO». После входа у провайдера вы вернётесь в UI уже авторизованным.

## Проверка (SSO)

| Действие | Ожидаемый результат |
|----------|---------------------|
| `GET /v1/auth/oidc/status` без OIDC-полей | `{"enabled":false}`, вкладки SSO нет |
| `GET /v1/auth/jwt/config` | секрета нет, есть `oidc_client_secret_set` |
| вход, группа из `bound_groups` | токен с политиками роли, привязанный к `tuck_namespace` |
| вход, группы не подходят ни одной роли | `403 no matching role for token claims` |
| повторный `POST /callback` с тем же `state` | `400 invalid or expired state` |
| `state` старше 5 минут | `400 invalid or expired state` |
| `id_token` с чужим `nonce`, `aud`, `iss` или просроченный | `401 invalid or expired id_token` |

## Ограничения и частые ошибки (SSO)

- **Каждый `state` одноразовый и сгорает при любой попытке**, даже неудачной: после ошибки начните вход заново. Между узлами HA защитой от повторного использования служит ещё и одноразовость кода у провайдера.
- **Одновременно висит не больше 1000 незавершённых входов**; при переполнении `auth-url` отвечает `429`. Просроченные записи удаляются автоматически.
- **`auth-url` и `callback` без токена, ограничены по частоте** (около 5 запросов в секунду на адрес). За прокси, который скрывает адрес клиента, лимит общий.
- **Claim групп называется `groups`**, другое имя настроить нельзя. Вложенные группы провайдер должен раскрыть сам.
- **Один провайдер на Tuck.** SSO использует тот же `jwks_uri`, `issuer` и роли, что и JWT-вход.
- **Пока не поддерживается:** logout на стороне провайдера (токен Tuck живёт до `ttl`, отозвать можно `tuckcli token revoke`), refresh-токены провайдера, вход SSO из `tuckcli`.
- **`redirect_uri mismatch` у провайдера:** адрес в клиенте и в `oidc_redirect_uri` должны совпадать до символа, включая `/ui` и схему.
- **`401` при верном входе:** проверьте `issuer` (точно как `iss` в токене), `audience` (по умолчанию это `oidc_client_id`) и что часы Tuck и провайдера не расходятся.
- **`400 authorization code rejected`:** неверный client secret или `oidc_redirect_uri`, либо код уже использован.
- **Страница после входа показывает «Сессия входа не найдена»:** вход начат в другой вкладке или браузере, либо хранилище браузера (sessionStorage) отключено; начните вход заново в одной вкладке.

## Дальше

- [12 — LDAP](12-ldap.md)
- [02 — Политики](02-politiki-i-dostup.md)
