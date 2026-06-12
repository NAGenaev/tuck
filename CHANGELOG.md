# Changelog

All notable changes to Tuck are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

---

## [Unreleased]

---

## [0.30.0] — 2026-06-12

### Added

#### UI: Auth Methods (`internal/ui`)

New **Auth Methods** page in the embedded dashboard covering all four authentication
backends:

- **AppRole** — list roles, create/update role (policies, token TTL, secret-ID TTL &
  max-uses), generate secret-ID (shown once)
- **JWT/OIDC** — load/save config (JWKS URI, issuer, audience, default TTL), list
  roles, create/update role (bound_subject, policies, TTL)
- **LDAP** — load/save config (URL, base DN, bind DN, user attribute), list roles,
  create/update role (groups, policies, TTL)
- **Kubernetes** — create/update/lookup/delete role keyed by namespace + service account

#### UI: Dynamic Secrets (`internal/ui`)

New **Dynamic Secrets** page with five tabs:

- **Database** — manage connections (plugin + connection URL), roles (SQL templates),
  generate credentials (username + password shown once)
- **AWS** — config (access key, region), roles (iam_user / assumed_role, policy/role
  ARNs), generate credentials
- **GCP** — config (service account JSON), roles (credential type, SA email, OAuth2
  scopes), generate credentials
- **Azure** — config (tenant/client ID + secret), roles (application object ID +
  application ID, TTL), generate credentials
- **Leases** — list all active leases across Database / AWS / GCP / Azure engines with
  per-lease revocation

---

## [0.29.0] — 2026-06-12

### Added

#### Token MaxUses (`internal/token`, `internal/core`, `internal/api`)

Tokens can now be created with a **`max_uses`** limit. Once a token has been used
for `max_uses` authenticated API calls it is automatically revoked — any further
request returns 401.

Use cases:
- **Bootstrap tokens** — one-time tokens for agent initialisation (`max_uses: 1`)
- **Limited-blast-radius tokens** — tokens that expire after a fixed number of
  operations, minimising exposure if leaked
- **AppRole-style tight coupling** — issue a short-lived, single-use token per
  deploy pipeline run

**How it works:**

1. `Token.MaxUses int` (0 = unlimited) and `Token.UseCount int` are stored in the
   barrier alongside the token record.
2. The HTTP middleware (`requireToken`) calls `core.TrackUse` **exactly once per
   authenticated request**. `Authenticate` and `EnforceAccess` never touch the
   counter — double-counting is impossible.
3. When `UseCount` exceeds `MaxUses` the token is deleted atomically and the
   request is rejected with HTTP 401.

**API:**
```json
POST /v1/auth/token
{
  "display_name": "bootstrap",
  "policies": ["agent"],
  "ttl": "5m",
  "max_uses": 1
}
```

**CLI:**
```sh
tuckcli token create --name=bootstrap --policy=agent --ttl=5m --max-uses=1
```

**Tests:** 3 new integration tests:
- `TestTokenMaxUses` — max_uses=2 succeeds twice, fails on third call
- `TestTokenMaxUsesOne` — max_uses=1 succeeds once, fails on second
- `TestTokenMaxUsesZeroMeansUnlimited` — max_uses=0 (default) has no limit

---

## [0.28.0] — 2026-06-12

### Added

#### Renewable Tokens with MaxTTL (`internal/token`, `internal/core`)

Tokens can now be created as **renewable** with an optional **max TTL** cap that
bounds the total lifetime regardless of how many times the token is renewed.

**Token struct changes:**
- `Renewable bool` — false by default; set to true via `renewable: true` in create request
- `MaxTTL time.Duration` — zero means no cap; otherwise caps renewal at `created_at + max_ttl`

**Enforcement in `RenewToken`:**
1. Returns `ErrNotRenewable` (HTTP 400) if `Renewable == false`
2. Caps `ExpiresAt` to `CreatedAt + MaxTTL` when MaxTTL is set

**New API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/auth/token/lookup-self` | Return metadata of the authenticated caller's token |
| `POST` | `/v1/auth/token/renew-self` | Renew the authenticated caller's own token |

**`POST /v1/auth/token` request additions:**
```json
{
  "display_name": "ci-runner",
  "policies": ["ci"],
  "ttl": "1h",
  "renewable": true,
  "max_ttl": "24h"
}
```

---

## [0.27.0] — 2026-06-12

### Added

#### Policy Deny Rules (`internal/policy`)

The policy engine now supports an explicit **deny** capability that overrides
any allow rules, regardless of policy order.

**How it works:**

1. Before checking allow rules, `Allowed` scans every rule in every attached
   policy. If any matching rule carries `CapDeny`, the request is rejected
   immediately — no allow rule can override it.
2. Only if no deny rule fires does the usual allow-matching proceed.

**Changes:**
- `CapDeny Capability = 1 << 4` added to the capability bitmask
- `Allowed` performs a deny-first two-pass evaluation
- `parseCaps` in the API layer accepts `"deny"` as a capability string
- `capsToStrings` emits `"deny"` when `CapDeny` is set
- 7 new unit tests in `internal/policy/policy_test.go`

**Example policy with a deny rule:**

```json
{
  "rules": [
    { "path": "secret/**",      "capabilities": ["read", "write"] },
    { "path": "secret/prod/*",  "capabilities": ["deny"] }
  ]
}
```

Tokens carrying this policy can read/write `secret/dev/key` but are
unconditionally blocked from anything under `secret/prod/`.

---

## [0.26.0] — 2026-06-12

### Added

#### Token Accessor (`internal/token`)

Every token now carries an **accessor** — a separate, cryptographically random
identifier (`tuck_acc_` + base64url(16 bytes)) stored as an index in the barrier
(`auth/accessor/<accessor>` → token ID).

The accessor lets operators look up or revoke tokens by a value that is safe
to log, pass between services, or store in external systems — without ever
exposing the raw bearer token.

**Changes:**
- `Token.Accessor` field added; populated by `Generate` on every new token
- `Store.Put` writes an accessor index entry alongside the token record
- `Store.Delete` cleans up the accessor index atomically
- `Store.GetByAccessor` / `Store.DeleteByAccessor` for accessor-based operations

**New API endpoints**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/auth/token/lookup-accessor` | Fetch token metadata by accessor |
| `DELETE` | `/v1/auth/token/revoke-accessor` | Revoke a token by accessor |

Both require a token with `write` / `delete` capability on `auth/token`.

**Example**

```sh
# Create a token — response now includes "accessor"
TOKEN_DATA=$(curl -s -XPOST https://tuck:8200/v1/auth/token \
  -H "X-Tuck-Token: $ROOT" \
  -d '{"display_name":"ci","policies":["ci-policy"],"ttl":"1h"}')
ACCESSOR=$(echo $TOKEN_DATA | jq -r .accessor)
# → "tuck_acc_xxxxxxxxxxxxxxxxxxxxxxxxxxx"

# Look up token metadata by accessor (safe to store in a log)
curl -s -XPOST https://tuck:8200/v1/auth/token/lookup-accessor \
  -H "X-Tuck-Token: $ROOT" \
  -d "{\"accessor\":\"$ACCESSOR\"}"
# → {"id":"tuck_...","accessor":"tuck_acc_...","display_name":"ci","policies":["ci-policy"],...}

# Revoke by accessor — no need to know the raw token value
curl -XDELETE https://tuck:8200/v1/auth/token/revoke-accessor \
  -H "X-Tuck-Token: $ROOT" \
  -d "{\"accessor\":\"$ACCESSOR\"}"
```

---

## [0.25.0] — 2026-06-12

### Added

#### Cubbyhole Engine (`internal/cubbyhole`)

Per-token private storage. Every Tuck token has its own isolated cubbyhole
namespace — no other token can read or write another token's data. All entries
are automatically purged when the owner token is revoked (explicit or TTL expiry).

Common use cases:
- Store bootstrap credentials visible only to the process that received them
- Pair with response wrapping: unwrap a secret into the cubbyhole, read it from there
- Temporary scratch space tied to a session token lifetime

**Storage:** `cubbyhole/<token_id>/<path>` (AES-256-GCM in barrier)

**API endpoints** (all authenticated; data scoped to the caller's own token)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/cubbyhole/{path}` | Read a JSON object |
| `PUT` | `/v1/cubbyhole/{path}` | Write a JSON object |
| `DELETE` | `/v1/cubbyhole/{path}` | Delete an entry |
| `LIST` | `/v1/cubbyhole/{path}` | List keys under a prefix |

Cubbyhole data is purged:
- Automatically when the token expires (GC runs every 15 minutes)
- Immediately when `DELETE /v1/auth/token/{id}` is called

**Example**

```sh
# Write into your own cubbyhole
curl -XPUT https://tuck:8200/v1/cubbyhole/bootstrap \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"db_url":"postgres://...", "api_key":"sk-..."}'

# Read back
curl https://tuck:8200/v1/cubbyhole/bootstrap \
  -H "X-Tuck-Token: $TOKEN"
# → {"data": {"db_url": "postgres://...", "api_key": "sk-..."}}

# List keys
curl -XLIST https://tuck:8200/v1/cubbyhole/ \
  -H "X-Tuck-Token: $TOKEN"
# → {"keys": ["bootstrap"]}

# Another token cannot access this data — it has its own empty cubbyhole
```

---

## [0.24.0] — 2026-06-12

### Added

#### Response Wrapping (`internal/wrapping`)

Any JSON payload can be sealed inside a single-use wrapping token with a
configurable TTL. The caller hands the token to a consumer; the consumer
unwraps it once to retrieve the payload. Because the token is deleted
atomically on the first unwrap attempt, a failed unwrap proves that someone
else has already read the secret — an integrity guarantee impossible with
plaintext delivery.

**Token format:** `tuck_wrap_` + base64url(32 random bytes)

**Default TTL:** 5 minutes. **Maximum TTL:** 24 hours.

**Storage:** `sys/wrapping/<id>` in the barrier (AES-256-GCM encrypted).

**API endpoints** (all authenticated)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sys/wrapping/wrap` | Seal a JSON payload; return a wrapping token |
| `POST` | `/v1/sys/wrapping/unwrap` | Consume the token and return the payload |
| `POST` | `/v1/sys/wrapping/lookup` | Inspect token metadata without consuming it |
| `DELETE` | `/v1/sys/wrapping/revoke` | Explicitly destroy a wrapping token |

Background GC removes expired wrapping tokens every 15 minutes.

**Example — CI hand-off**

```sh
# Stage 1: wrap a deployment secret
WRAP=$(curl -s -XPOST https://tuck:8200/v1/sys/wrapping/wrap \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{
    "data": {"db_password": "s3cr3t", "db_user": "app"},
    "ttl": "5m"
  }')
WRAP_TOKEN=$(echo $WRAP | jq -r .token)
# → tuck_wrap_...  (hand this token to Stage 2, not the secret)

# Stage 2: unwrap once
curl -s -XPOST https://tuck:8200/v1/sys/wrapping/unwrap \
  -H "X-Tuck-Token: $STAGE2_TOKEN" \
  -d "{\"token\":\"$WRAP_TOKEN\"}"
# → {"data": {"db_password": "s3cr3t", "db_user": "app"}}

# Any subsequent unwrap attempt returns 404 — token is consumed:
curl -s -XPOST https://tuck:8200/v1/sys/wrapping/unwrap \
  -H "X-Tuck-Token: $STAGE2_TOKEN" \
  -d "{\"token\":\"$WRAP_TOKEN\"}"
# → {"error": "wrapping token not found or already used"}
```

**Inspect without consuming**

```sh
curl -XPOST https://tuck:8200/v1/sys/wrapping/lookup \
  -H "X-Tuck-Token: $TOKEN" \
  -d "{\"token\":\"$WRAP_TOKEN\"}"
# → {"creation_time":"...","expires_at":"...","creation_ttl":300}
```

---

## [0.23.0] — 2026-06-12

### Added

#### Azure Dynamic Secrets Engine (`internal/dynamic/azure`)

Generate short-lived Azure AD client secrets for existing app registrations on
demand, using the Microsoft Graph API. Completes the cloud trio: AWS (M21),
GCP (M22), Azure (M23).

Tuck adds a password credential to a configured Azure AD application, returns
the client secret once, and on lease expiry / explicit revocation removes the
credential via Graph API `removePassword`.

Credentials resolve in order: `client_id` + `client_secret` in config →
`DefaultAzureCredential` (Managed Identity on AKS, `AZURE_*` env vars, Azure CLI).

**Config** (`PUT /v1/azure/config`)

| Field | Description |
|-------|-------------|
| `tenant_id` | Azure AD tenant ID (required) |
| `client_id` | Service principal client ID used to call Graph API (optional; empty = DefaultAzureCredential) |
| `client_secret` | SP secret (stored encrypted; never returned on GET) |

**Role** (`PUT /v1/azure/roles/{name}`)

| Field | Description |
|-------|-------------|
| `application_object_id` | Object ID of the Azure AD app registration (used for Graph API calls) |
| `application_id` | Client ID of the Azure AD app (returned as `client_id` in generated credentials) |
| `default_ttl` | Credential lifetime; default 1 h |
| `max_ttl` | Maximum TTL; default 12 h |

**Generate result** (`POST /v1/azure/creds/{role}`)

| Field | Description |
|-------|-------------|
| `lease_id` | ID for lease inspection / early revocation |
| `tenant_id` | Azure AD tenant ID |
| `client_id` | Application (client) ID |
| `client_secret` | Generated secret value (shown once) |
| `expires_at` | RFC 3339 expiry timestamp |

**API endpoints** (11 total, all authenticated)

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/v1/azure/config` | Write config |
| `GET` | `/v1/azure/config` | Read config (`client_secret` redacted) |
| `DELETE` | `/v1/azure/config` | Delete config |
| `PUT` | `/v1/azure/roles/{name}` | Create / update role |
| `GET` | `/v1/azure/roles/{name}` | Read role |
| `DELETE` | `/v1/azure/roles/{name}` | Delete role |
| `LIST` | `/v1/azure/roles/` | List role names |
| `POST` | `/v1/azure/creds/{role}` | Generate client secret |
| `GET` | `/v1/azure/lease/{id}` | Inspect lease |
| `DELETE` | `/v1/azure/lease/{id}` | Revoke lease early |
| `LIST` | `/v1/azure/lease/` | List lease IDs |

Background GC revokes expired leases every 15 minutes and removes Azure AD
password credentials.

**Example — AKS with Managed Identity**

```sh
# Configure (empty client_id/secret = use Managed Identity automatically)
curl -XPUT https://tuck:8200/v1/azure/config \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"tenant_id":"00000000-0000-0000-0000-000000000000"}'

# Create a role (find object_id in Azure Portal → App registrations → Overview)
curl -XPUT https://tuck:8200/v1/azure/roles/api-app \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{
    "application_object_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
    "application_id":        "11111111-2222-3333-4444-555555555555",
    "default_ttl": "1h",
    "max_ttl": "8h"
  }'

# Generate a client secret
curl -XPOST https://tuck:8200/v1/azure/creds/api-app \
  -H "X-Tuck-Token: $TOKEN"
# → {
#     "lease_id": "...",
#     "tenant_id": "00000000-...",
#     "client_id": "11111111-...",
#     "client_secret": "xxx~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
#     "expires_at": "2026-06-12T15:00:00Z"
#   }

# Revoke early
curl -XDELETE https://tuck:8200/v1/azure/lease/<lease_id> \
  -H "X-Tuck-Token: $TOKEN"
```

**Required Azure AD permissions for the Tuck service principal**

The service principal or managed identity used by Tuck must have the Microsoft
Graph application permission `Application.ReadWrite.OwnedBy` (or
`Application.ReadWrite.All`) granted and admin-consented in the Azure AD tenant.

---

## [0.22.0] — 2026-06-12

### Added

#### GCP Dynamic Secrets Engine (`internal/dynamic/gcp`)

Generate short-lived GCP credentials on demand. Completes the cloud-trio
alongside the AWS engine (M21) and GCP KMS seal (M19).

Two credential types:

**`service_account_key`** — calls the GCP IAM Admin API to create a new JSON
key for an existing service account. Returns the key file once. On lease expiry
/ explicit revocation, the key is deleted from GCP IAM.

**`access_token`** — calls the IAM Credentials API (`generateAccessToken`) to
produce a short-lived OAuth2 Bearer token for a service account via
impersonation. The token expires naturally; no cleanup is needed.

Credentials are resolved in order: inline `credentials_json` in the config →
Application Default Credentials (Workload Identity on GKE, env var
`GOOGLE_APPLICATION_CREDENTIALS`, `gcloud auth application-default login`).

**Config** (`PUT /v1/gcp/config`)

| Field | Description |
|-------|-------------|
| `credentials_json` | Inline service account JSON (stored encrypted; never returned on GET). Empty = ADC. |

**Role** (`PUT /v1/gcp/roles/{name}`)

| Field | Description |
|-------|-------------|
| `credential_type` | `service_account_key` or `access_token` |
| `service_account_email` | SA to use (e.g. `deploy@project.iam.gserviceaccount.com`) |
| `key_algorithm` | `KEY_ALG_RSA_2048` (default) or `KEY_ALG_RSA_4096` |
| `scopes` | OAuth2 scopes for `access_token` (default: `cloud-platform`) |
| `default_ttl` | Credential lifetime; default 1 h |
| `max_ttl` | Maximum TTL; default 12 h |

**API endpoints** (11 total, all authenticated)

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/v1/gcp/config` | Write config |
| `GET` | `/v1/gcp/config` | Read config (`credentials_json` redacted) |
| `DELETE` | `/v1/gcp/config` | Delete config |
| `PUT` | `/v1/gcp/roles/{name}` | Create / update role |
| `GET` | `/v1/gcp/roles/{name}` | Read role |
| `DELETE` | `/v1/gcp/roles/{name}` | Delete role |
| `LIST` | `/v1/gcp/roles/` | List role names |
| `POST` | `/v1/gcp/creds/{role}` | Generate credentials |
| `GET` | `/v1/gcp/lease/{id}` | Inspect lease |
| `DELETE` | `/v1/gcp/lease/{id}` | Revoke lease early |
| `LIST` | `/v1/gcp/lease/` | List lease IDs |

Background GC revokes expired leases every 15 minutes and deletes SA keys.

**Example — GKE with Workload Identity (access_token)**

```sh
# Configure (empty = use Workload Identity automatically)
curl -XPUT https://tuck:8200/v1/gcp/config \
  -H "X-Tuck-Token: $TOKEN" -d '{}'

# Create a role
curl -XPUT https://tuck:8200/v1/gcp/roles/bigquery-reader \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{
    "credential_type": "access_token",
    "service_account_email": "bq-reader@my-project.iam.gserviceaccount.com",
    "scopes": ["https://www.googleapis.com/auth/bigquery.readonly"],
    "default_ttl": "1h"
  }'

# Generate token
curl -XPOST https://tuck:8200/v1/gcp/creds/bigquery-reader \
  -H "X-Tuck-Token: $TOKEN"
# → {"lease_id":"...","access_token":"ya29...","token_type":"Bearer","expires_at":"..."}
```

**Example — service account JSON key**

```sh
curl -XPUT https://tuck:8200/v1/gcp/roles/ci-deploy \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"credential_type":"service_account_key","service_account_email":"ci@project.iam.gserviceaccount.com"}'

curl -XPOST https://tuck:8200/v1/gcp/creds/ci-deploy \
  -H "X-Tuck-Token: $TOKEN"
# → {"lease_id":"...","private_key":"{\"type\":\"service_account\",...}","expires_at":"..."}
```

**Required GCP permissions for the Tuck service account**

- `service_account_key`: `roles/iam.serviceAccountKeyAdmin` on the target SA
- `access_token`: `roles/iam.serviceAccountTokenCreator` on the target SA

---

## [0.21.0] — 2026-06-12

### Added

#### AWS Dynamic Secrets Engine (`internal/dynamic/aws`)

Generate short-lived AWS credentials on demand. Tuck creates them, tracks
them as leases, and revokes them automatically when the lease expires.

Two credential types:

**`assumed_role`** — calls STS `AssumeRole`, returns temporary credentials
(access key + secret key + session token). Expire naturally; no cleanup.

**`iam_user`** — creates an IAM user, attaches policies, creates an access key.
On expiry / revocation: access key deleted, user and policies removed from AWS.

**Config** (`PUT /v1/aws/config`)

| Field | Default | Description |
|-------|---------|-------------|
| `region` | _(required)_ | AWS region (e.g. `us-east-1`) |
| `access_key_id` | _(empty)_ | Empty = use default credential chain (IRSA / instance role / env) |
| `secret_access_key` | _(empty)_ | Stored encrypted; never returned on GET |
| `iam_endpoint` | _(empty)_ | Custom IAM endpoint (Localstack, VPC endpoint) |
| `sts_endpoint` | _(empty)_ | Custom STS endpoint |

**Role** (`PUT /v1/aws/roles/{name}`)

| Field | Description |
|-------|-------------|
| `credential_type` | `assumed_role` or `iam_user` |
| `role_arns` | IAM role ARNs to assume (`assumed_role`; first ARN used) |
| `policy_arns` | Managed policy ARNs to attach (`iam_user`) |
| `policy_document` | Inline JSON policy |
| `default_ttl` | Credential lifetime; default 1 h |
| `max_ttl` | Maximum TTL; default 12 h |

**API endpoints** (11 total, all authenticated)

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/v1/aws/config` | Write config |
| `GET` | `/v1/aws/config` | Read config (`secret_access_key` redacted) |
| `DELETE` | `/v1/aws/config` | Delete config |
| `PUT` | `/v1/aws/roles/{name}` | Create / update role |
| `GET` | `/v1/aws/roles/{name}` | Read role |
| `DELETE` | `/v1/aws/roles/{name}` | Delete role |
| `LIST` | `/v1/aws/roles/` | List role names |
| `POST` | `/v1/aws/creds/{role}` | Generate credentials |
| `GET` | `/v1/aws/lease/{id}` | Inspect lease |
| `DELETE` | `/v1/aws/lease/{id}` | Revoke lease early |
| `LIST` | `/v1/aws/lease/` | List lease IDs |

Background GC revokes expired leases every 15 minutes and deletes the IAM
users from AWS.

**Example — EKS with IRSA (assumed_role)**

```sh
curl -XPUT https://tuck:8200/v1/aws/config \
  -H "X-Tuck-Token: $TOKEN" -d '{"region":"us-east-1"}'

curl -XPUT https://tuck:8200/v1/aws/roles/deploy \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"credential_type":"assumed_role","role_arns":["arn:aws:iam::123:role/deploy"],"default_ttl":"1h"}'

curl -XPOST https://tuck:8200/v1/aws/creds/deploy \
  -H "X-Tuck-Token: $TOKEN"
# → {"lease_id":"...","access_key_id":"ASIA...","secret_access_key":"...","session_token":"...","expires_at":"..."}
```

**Required IAM permissions for the Tuck server**

- `assumed_role`: `sts:AssumeRole` on the target role ARN
- `iam_user`: `iam:CreateUser`, `iam:AttachUserPolicy`, `iam:PutUserPolicy`,
  `iam:CreateAccessKey`, `iam:DeleteAccessKey`, `iam:DetachUserPolicy`,
  `iam:DeleteUserPolicy`, `iam:DeleteUser`

---

## [0.20.0] — 2026-06-12

### Added

#### LDAP / Active Directory Auth (`internal/auth/ldap`)

Authenticate users from any LDAP-compatible directory service — OpenLDAP,
Active Directory, FreeIPA, 389 Directory Server. Group membership is mapped
to Tuck policies via configurable Roles.

**Login flow**

1. Connect to LDAP server (`ldap://` or `ldaps://`, optional STARTTLS).
2. Bind with the service account to search for the user entry.
3. Bind as the authenticated user to verify the password.
4. Collect group membership from `memberOf` attribute or via a dedicated
   group subtree search (`group_dn` + `group_filter`).
5. Match groups/usernames against configured Roles → union of policies.
6. Return a scoped Tuck token.

**Config fields** (PUT /v1/auth/ldap/config)

| Field | Default | Description |
|-------|---------|-------------|
| `urls` | _(required)_ | LDAP server URLs (ldap:// or ldaps://) |
| `bind_dn` | _(required)_ | Service account DN |
| `bind_password` | _(required)_ | Service account password (stored encrypted; never returned) |
| `user_dn` | _(required)_ | Base DN for user searches |
| `user_attr` | `uid` | Username attribute (use `sAMAccountName` for AD) |
| `group_dn` | `""` | Base DN for group searches; empty = read `memberOf` from user |
| `group_attr` | `memberOf` | User attribute holding group DNs (when `group_dn` is empty) |
| `group_filter` | `(member={{.UserDN}})` | LDAP filter for group searches |
| `tls_insecure` | `false` | Skip TLS verification (dev only) |
| `starttls` | `false` | Upgrade ldap:// to TLS via STARTTLS |

**Role fields** (PUT /v1/auth/ldap/role/{name})

| Field | Description |
|-------|-------------|
| `groups` | Group CNs or full DNs that grant this role (case-insensitive CN matching) |
| `users` | Specific usernames or user DNs (optional, bypasses group check) |
| `policies` | Tuck policies granted on login |
| `ttl` | Token lifetime (e.g. `"8h"`); empty = server default |

**API endpoints** (7 total)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/v1/auth/ldap/login` | public | Login with username + password |
| `GET` | `/v1/auth/ldap/config` | token | Read config (bind_password redacted) |
| `PUT` | `/v1/auth/ldap/config` | token | Write config |
| `PUT` | `/v1/auth/ldap/role/{name}` | token | Create / update role |
| `GET` | `/v1/auth/ldap/role/{name}` | token | Read role |
| `DELETE` | `/v1/auth/ldap/role/{name}` | token | Delete role |
| `LIST` | `/v1/auth/ldap/role/` | token | List role names |

**Example — Active Directory**

```sh
# Configure
curl -XPUT https://tuck:8200/v1/auth/ldap/config \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{
    "urls": ["ldaps://ad.corp.example.com:636"],
    "bind_dn": "CN=tuck-svc,OU=ServiceAccounts,DC=corp,DC=example,DC=com",
    "bind_password": "svc-password",
    "user_dn": "OU=Users,DC=corp,DC=example,DC=com",
    "user_attr": "sAMAccountName"
  }'

# Create a role
curl -XPUT https://tuck:8200/v1/auth/ldap/role/ops \
  -H "X-Tuck-Token: $TOKEN" \
  -d '{"groups":["Ops-Team"],"policies":["ops-policy"],"ttl":"8h"}'

# Login
curl -XPOST https://tuck:8200/v1/auth/ldap/login \
  -d '{"username":"alice","password":"alicepass"}'
```

---

#### Azure Key Vault Seal (`internal/seal/azurekv.go`)

Completes the cloud KMS trilogy: AWS KMS (M19), GCP Cloud KMS (M19), and
now Azure Key Vault. Auto-unseal using an RSA key stored in Azure Key Vault.

Credentials are resolved from DefaultAzureCredential:
- `AZURE_CLIENT_ID` / `AZURE_CLIENT_SECRET` / `AZURE_TENANT_ID` env vars
- Managed Identity (AKS Workload Identity, VM Managed Identity)
- Azure CLI credentials (local development)

**Flags**

| Flag | Default | Description |
|------|---------|-------------|
| `--seal-azurekv-vault-url` | _(required)_ | Vault URL (e.g. `https://my-vault.vault.azure.net`) |
| `--seal-azurekv-key-name` | _(required)_ | RSA key name in the vault |
| `--seal-azurekv-algorithm` | `RSA-OAEP-256` | Encryption algorithm |
| `--seal-azurekv-key-file` | `tuck-azurekv.enc` | File to store the ciphertext |

**Example — AKS with Workload Identity**

```sh
tuck \
  --seal-type=azurekv \
  --seal-azurekv-vault-url=https://my-vault.vault.azure.net \
  --seal-azurekv-key-name=tuck-seal \
  --tls-auto
```

The pod must have the `Keys Encrypt` and `Keys Decrypt` permissions on the
key, typically assigned via an Azure RBAC role (`Key Vault Crypto User`).

---

## [0.19.0] — 2026-06-12

### Added

#### AWS KMS Seal (`internal/seal/awskms.go`)

Native auto-unseal via AWS Key Management Service. On first init, Tuck
generates a 32-byte root key in memory, encrypts it with the specified
Customer Managed Key (CMK), and stores the ciphertext locally. On every
restart the ciphertext is decrypted by KMS — the plaintext never touches
disk.

Credentials are resolved from the standard AWS credential chain: IAM
instance/pod role (EC2 / EKS IRSA / ECS TaskRole) → environment variables
(`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`) → shared credentials file
(`~/.aws/credentials`).

**Flags**

| Flag | Default | Description |
|------|---------|-------------|
| `--seal-awskms-key-id` | _(required)_ | CMK ARN or alias (`alias/tuck-seal`, `arn:aws:kms:...`) |
| `--seal-awskms-region` | `""` | AWS region; empty = from `AWS_DEFAULT_REGION` or profile |
| `--seal-awskms-key-file` | `tuck-awskms.enc` | File to store the encrypted root key ciphertext |

**Example**

```sh
tuck \
  --seal-type=awskms \
  --seal-awskms-key-id=alias/tuck-seal \
  --seal-awskms-region=us-east-1 \
  --tls-auto
```

On EKS with IRSA, no credentials flags are needed — the pod's IAM role is
picked up automatically from the EC2 instance metadata service.

---

#### GCP Cloud KMS Seal (`internal/seal/gcpkms.go`)

Native auto-unseal via Google Cloud KMS. The flow mirrors the AWS KMS seal:
generate root key in memory → encrypt with the Cloud KMS CryptoKey → store
ciphertext locally → decrypt on every restart.

Credentials are resolved from Application Default Credentials (ADC):
`GOOGLE_APPLICATION_CREDENTIALS` env var pointing to a service account JSON
file, or the GCE/GKE metadata server (Workload Identity — the recommended
production configuration).

**Flags**

| Flag | Default | Description |
|------|---------|-------------|
| `--seal-gcpkms-key-name` | _(required)_ | Full CryptoKey resource name |
| `--seal-gcpkms-key-file` | `tuck-gcpkms.enc` | File to store the encrypted root key ciphertext |

The key name format:
```
projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}
```

**Example**

```sh
tuck \
  --seal-type=gcpkms \
  --seal-gcpkms-key-name=projects/my-project/locations/global/keyRings/tuck/cryptoKeys/seal \
  --tls-auto
```

On GKE with Workload Identity, no credentials flags are needed — the pod's
service account is picked up from the GKE metadata server automatically.

---

## [0.18.0] — 2026-06-11

### Added

#### TOTP Secrets Engine (`internal/dynamic/totp`)

Tuck now stores and manages TOTP (Time-based One-Time Password) secrets inside
the encrypted barrier. Applications can validate OTP codes server-side or have
Tuck generate the current code — useful for application 2FA flows and
service-to-service authentication via short-lived numeric codes.

**How it works**

TOTP is defined in RFC 6238 (built on HMAC-OTP, RFC 4226). A random secret is
stored in Tuck's encrypted barrier; at each 30-second window the engine
computes `HOTP(secret, floor(unix_time / period))` using dynamic truncation and
returns a numeric code. Tuck validates codes with a configurable skew window
(default ±1 period) to accommodate clock drift.

**Key options**

| Field | Default | Description |
|-------|---------|-------------|
| `algorithm` | `sha1` | Hash algorithm: `sha1`, `sha256`, `sha512` |
| `digits` | `6` | Code length: `6` or `8` |
| `period` | `30` | Code rotation period in seconds |
| `skew` | `1` | Allowed window drift in periods (checked on both sides) |
| `issuer` | `"Tuck"` | Label in the otpauth:// URL |
| `account` | key name | Account identifier in the otpauth:// URL |
| `secret` | auto-generated | Optional base32 TOTP secret to import |

**Workflow**

1. `POST /v1/totp/keys/{name}` — create a key; response includes the base32
   secret and an `otpauth://` URI ready for a QR code generator
2. Import the `url` into any standard authenticator app (Google Authenticator, Authy, etc.)
3. `POST /v1/totp/code/{name}` with `{"code":"123456"}` to validate a user code
4. `GET /v1/totp/code/{name}` to have Tuck generate the current code (for server-to-server flows)

**API endpoints** (6 total)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/totp/keys/{name}` | Create or overwrite a TOTP key |
| `GET` | `/v1/totp/keys/{name}` | Read key metadata (no secret) |
| `DELETE` | `/v1/totp/keys/{name}` | Delete a key |
| `LIST` | `/v1/totp/keys/` | List key names |
| `GET` | `/v1/totp/code/{name}` | Generate the current code (+ `valid_until`) |
| `POST` | `/v1/totp/code/{name}` | Validate a code → `{"valid":true}` |

**Quick start**
```sh
# Create a TOTP key — use the returned "url" to generate a QR code
TOKEN=$(curl -s https://tuck:8200/v1/totp/keys/myapp \
  -X POST -H "X-Tuck-Token: $ROOT" \
  -d '{"issuer":"ACME Corp","account":"user@example.com"}' \
  | jq -r .secret)

# Validate a code entered by the user
curl -XPOST https://tuck:8200/v1/totp/code/myapp \
  -H "X-Tuck-Token: $APP_TOKEN" \
  -d '{"code":"123456"}'
# → {"valid":true}

# Server-side code generation (e.g. for rotation scripts)
curl https://tuck:8200/v1/totp/code/myapp \
  -H "X-Tuck-Token: $APP_TOKEN"
# → {"code":"123456","valid_until":"2026-06-11T12:00:30Z"}
```

**Tests**: 13 unit tests including RFC 6238 Appendix B known test vectors
(SHA1 at T=59, T=1111111109, T=1234567890, T=2000000000), skew window
boundary tests, algorithm variants (SHA256), 8-digit codes, import/export, and
invalid secret rejection.

**No new dependencies** — implemented with Go stdlib only (`crypto/hmac`,
`crypto/sha1`, `crypto/sha256`, `crypto/sha512`, `encoding/base32`).

---

## [0.17.0] — 2026-06-11

### Added

#### SSH Secrets Engine (`internal/dynamic/ssh`)

CA-mode SSH certificate authority. Tuck holds an SSH CA key pair and signs
short-lived SSH certificates for users or hosts. Target hosts need only a
one-time `TrustedUserCAKeys` configuration; thereafter any certificate signed
by Tuck is automatically trusted — no per-user `authorized_keys` management.

**CA key types**

| Type | Details |
|------|---------|
| `ed25519` (default) | Fast, small certs |
| `rsa` | 4096-bit RSA CA |

**Workflow**

1. `POST /v1/ssh/generate/ca` — generate CA (or `POST /v1/ssh/import/ca` to bring your own)
2. `GET /v1/ssh/ca/public-key` (unauthenticated) — fetch CA pubkey → put in `TrustedUserCAKeys`
3. `PUT /v1/ssh/roles/{name}` — create a role constraining principals, TTL, cert type
4. `POST /v1/ssh/sign/{role}` — sign a user's SSH public key → returns `signed_key` for `~/.ssh/id_*-cert.pub`

**Role options**

- `allowed_users` — whitelist of SSH usernames; empty = any principal
- `cert_type` — `user` (default) or `host`
- `default_ttl` / `max_ttl` — certificate lifetime (requested TTL capped at max_ttl)
- `default_extensions` — per-role extensions (default: the five standard `permit-*` extensions)

**API endpoints** (8 total)

| Method | Path | Auth |
|--------|------|------|
| `POST` | `/v1/ssh/generate/ca` | required |
| `POST` | `/v1/ssh/import/ca` | required |
| `GET` | `/v1/ssh/ca/public-key` | none |
| `PUT` | `/v1/ssh/roles/{name}` | required |
| `GET` | `/v1/ssh/roles/{name}` | required |
| `DELETE` | `/v1/ssh/roles/{name}` | required |
| `LIST` | `/v1/ssh/roles/` | required |
| `POST` | `/v1/ssh/sign/{role}` | required |

**Tests**: 12 unit tests covering CA generation (Ed25519 + RSA), CA import,
role CRUD, user cert signing + chain verification via `gossh.CertChecker`, TTL
capping, principal enforcement, host certs, and RSA CA signing.

**Dependency**: `golang.org/x/crypto v0.53.0` (added)

---

## [0.16.0] — 2026-06-11

### Added

#### Transit Secrets Engine (`internal/dynamic/transit`)

Encryption-as-a-service. Applications submit data for cryptographic operations without ever handling raw key material. Keys are versioned; old ciphertext can be re-encrypted after rotation without re-querying the source.

**Key types**

| Type | Operations |
|------|-----------|
| `aes256-gcm96` (default) | encrypt, decrypt, rewrap, hmac |
| `ecdsa-p256` | sign, verify, hmac |
| `ed25519` | sign, verify, hmac |
| `rsa-2048` | sign (PSS), verify, hmac |
| `rsa-4096` | sign (PSS), verify, hmac |

**Key features**
- **Versioned keys** — `Rotate` creates a new key version; all previous versions remain for decryption/verification down to `min_decryption_version`.
- **Rewrap** — re-encrypt old ciphertext with the current key version; migrate at your own pace after rotation.
- **Ciphertext format** — `vault:v{N}:{base64url}` — the version prefix is embedded in every ciphertext/signature for unambiguous routing.
- **HMAC** — deterministic MAC using the key's raw material; all key types supported.
- **Ed25519** — uses its own internal hash; `hash_algorithm` is ignored (sign/verify take the raw message).
- **RSA-PSS** — uses PSS padding with SHA-256/384/512 depending on `hash_algorithm`.
- **Deletion protection** — keys are not deletable by default; set `deletable=true` via the config endpoint first.
- **Idempotent create** — calling `CreateKey` on an existing key is a no-op.
- 16 tests covering all operations, key rotation, rewrap, min_version enforcement, deletion guard, type mismatch errors, and invalid ciphertext handling.

**HTTP API**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/transit/keys/{name}` | Create a key (`{"type":"aes256-gcm96"}`) |
| GET | `/v1/transit/keys/{name}` | Get key metadata (no key material) |
| DELETE | `/v1/transit/keys/{name}` | Delete key (must be marked deletable) |
| LIST | `/v1/transit/keys/` | List key names |
| POST | `/v1/transit/keys/{name}/rotate` | Rotate — add a new key version |
| POST | `/v1/transit/keys/{name}/config` | Update `min_decryption_version`, `deletable` |
| POST | `/v1/transit/encrypt/{name}` | Encrypt plaintext (`{"plaintext":"<b64url>"}`) |
| POST | `/v1/transit/decrypt/{name}` | Decrypt ciphertext → `{"plaintext":"<b64url>"}` |
| POST | `/v1/transit/rewrap/{name}` | Re-encrypt with latest key version |
| POST | `/v1/transit/sign/{name}` | Sign input → `{"signature":"vault:v{N}:..."}` |
| POST | `/v1/transit/verify/{name}` | Verify signature → `{"valid":true}` |
| POST | `/v1/transit/hmac/{name}` | Compute HMAC → `{"hmac":"vault:v{N}:..."}` |

**Quick start**
```sh
# Create an AES key and encrypt
curl -XPOST https://tuck:8200/v1/transit/keys/payments \
  -H "X-Tuck-Token: $TOKEN" -d '{"type":"aes256-gcm96"}'

CIPHER=$(curl -s -XPOST https://tuck:8200/v1/transit/encrypt/payments \
  -H "X-Tuck-Token: $TOKEN" \
  -d "{\"plaintext\":\"$(echo -n 'card:4242' | base64 -w0)\"}" \
  | jq -r .ciphertext)

# Rotate the key and rewrap all stored ciphertext
curl -XPOST https://tuck:8200/v1/transit/keys/payments/rotate \
  -H "X-Tuck-Token: $TOKEN"

curl -XPOST https://tuck:8200/v1/transit/rewrap/payments \
  -H "X-Tuck-Token: $TOKEN" -d "{\"ciphertext\":\"$CIPHER\"}"
```

---

## [0.15.0] — 2026-06-11

### Added

#### PKI Secrets Engine (`internal/dynamic/pki`)

Tuck now acts as an internal Certificate Authority. Services can request short-lived X.509 certificates on demand — no more static cert files or manual CA workflows.

- **`Manager.GenerateCA`** — creates a self-signed root CA (ECDSA P-256 default, or RSA); persists key inside the encrypted barrier.
- **`Manager.ImportCA`** — imports an existing CA cert + private key; validates both before persisting.
- **`Manager.GetCRL`** — generates a signed CRL from all revoked certificate records (updates on every call).
- **`Role`** — controls what certs a role may issue: `allowed_domains`, `allow_subdomains`, `allow_ip_sans`, `allow_localhost`, `key_type` (ec/rsa), `key_bits`, `default_ttl`, `max_ttl`, `server_flag`, `client_flag`.
- **`Manager.IssueCert`** — validates CN + SANs against role, generates a new key pair, signs the leaf cert with the CA, persists a `CertRecord` (no private key stored), returns the cert + private key to the caller once.
- **`Manager.RevokeCert`** — marks a cert as revoked; it appears in the next CRL.
- Domain validation: exact match or subdomain match (when `allow_subdomains=true`); IP SANs gated by `allow_ip_sans`; loopback gated by `allow_localhost`.
- TTL enforcement: `max_ttl` caps requested TTL; falls back to `default_ttl`.
- 12 tests covering: CA generation, CA import, role CRUD, cert issuance + x509 chain verification, RSA keys, domain policy enforcement, subdomain allow, IP SAN allow/deny, revocation + CRL parsing, cert listing, TTL capping.

**HTTP API**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/pki/generate/root` | token | Generate a new self-signed root CA |
| POST | `/v1/pki/import/ca` | token | Import an existing CA cert + key |
| GET | `/v1/pki/ca/pem` | none | Fetch the CA certificate (for client trust stores) |
| GET | `/v1/pki/crl/pem` | none | Fetch the current CRL |
| PUT | `/v1/pki/roles/{name}` | token | Create or update a role |
| GET | `/v1/pki/roles/{name}` | token | Read a role |
| DELETE | `/v1/pki/roles/{name}` | token | Delete a role |
| LIST | `/v1/pki/roles/` | token | List role names |
| POST | `/v1/pki/issue/{role}` | token | Issue a TLS certificate |
| POST | `/v1/pki/revoke/{serial}` | token | Revoke a certificate |
| GET | `/v1/pki/certs/{serial}` | token | Inspect a cert record (metadata only) |
| LIST | `/v1/pki/certs/` | token | List issued cert serials |

**Quick start**
```sh
# 1. Generate root CA
curl -XPOST https://tuck:8200/v1/pki/generate/root \
  -H "X-Tuck-Token: $ROOT" \
  -d '{"common_name":"Tuck Internal CA","ttl":"87600h"}'

# 2. Create a role
curl -XPUT https://tuck:8200/v1/pki/roles/web \
  -H "X-Tuck-Token: $ROOT" \
  -d '{"allowed_domains":["svc.cluster.local"],"allow_subdomains":true,"server_flag":true,"default_ttl":"72h"}'

# 3. Issue a certificate
curl -XPOST https://tuck:8200/v1/pki/issue/web \
  -H "X-Tuck-Token: $APP_TOKEN" \
  -d '{"common_name":"api.svc.cluster.local"}'

# 4. Distribute CA cert to clients
curl https://tuck:8200/v1/pki/ca/pem
```

---

## [0.14.0] — 2026-06-11

### Added

#### AppRole Auth (`internal/auth/approle`)

Machine-to-machine authentication using role-id + secret-id pairs — no OIDC provider or Kubernetes dependency required.

- **`Role`** — named role with auto-generated `role_id`; configurable `token_ttl`, `secret_id_ttl`, `secret_id_num_uses`, and `policies`.
- **`SecretID`** — short-lived credential generated per role; supports unlimited, limited-use, and TTL-bound modes.
- **`Store.Login`** — validates role-id + secret-id, decrements use-count, auto-deletes exhausted or expired secret-IDs, and returns a `LoginResult`.

**HTTP API**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/auth/approle/login` | none | Exchange role-id + secret-id for a Tuck token |
| PUT | `/v1/auth/approle/role/{name}` | token | Create or update a role |
| GET | `/v1/auth/approle/role/{name}` | token | Read role definition |
| DELETE | `/v1/auth/approle/role/{name}` | token | Delete a role |
| LIST | `/v1/auth/approle/role/` | token | List role names |
| POST | `/v1/auth/approle/role/{name}/secret-id` | token | Generate a new secret-id |
| GET | `/v1/auth/approle/role/{name}/secret-id/{id}` | token | Inspect a secret-id |
| DELETE | `/v1/auth/approle/role/{name}/secret-id/{id}` | token | Destroy a specific secret-id |

#### Dynamic Secrets — Database Engine (`internal/dynamic/database`)

On-demand short-lived database credentials for PostgreSQL and MySQL; no static credentials needed in application code.

- **`Config`** — named database connection (plugin_name: `postgresql` or `mysql`, DSN, max_open_conns); connection pool with ping-based health check.
- **`Role`** — maps a role name to a database config; `creation_statements` and `revocation_statements` support `{{username}}`, `{{password}}`, `{{expiry}}`, `{{database}}` templates; auto-populated with safe defaults per plugin type.
- **`Lease`** — tracks each generated credential; expired leases are revoked by the background GC via `RevokeExpired()`.
- GC integration: `dbManager.RevokeExpired(ctx)` is called every GC tick alongside token expiry.
- Connection URL masked on GET config responses to avoid credential leakage.

**HTTP API**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| PUT | `/v1/database/config/{name}` | token | Register a database connection |
| GET | `/v1/database/config/{name}` | token | Read config (connection_url redacted) |
| DELETE | `/v1/database/config/{name}` | token | Remove config and close pooled connection |
| LIST | `/v1/database/config/` | token | List config names |
| PUT | `/v1/database/role/{name}` | token | Create or update a database role |
| GET | `/v1/database/role/{name}` | token | Read role definition |
| DELETE | `/v1/database/role/{name}` | token | Delete a role |
| LIST | `/v1/database/role/` | token | List role names |
| POST | `/v1/database/creds/{role}` | token | Generate ephemeral credentials |
| GET | `/v1/database/lease/{id}` | token | Inspect a lease |
| DELETE | `/v1/database/lease/{id}` | token | Immediately revoke a lease |
| LIST | `/v1/database/lease/` | token | List active lease IDs |

---

## [0.13.0] — 2026-06-11

### Added

#### JWT/OIDC Auth (`internal/auth/jwt`)

Any OIDC-compatible identity provider (Keycloak, Auth0, Dex, GitHub Actions, Google, …) can now exchange a signed JWT for a short-lived Tuck token.

- **`Provider`** — validates JWTs against a JWKS endpoint; enforces issuer, audience, expiry, and `kid` header.
- **`JWKS`** — caching JWKS fetcher with configurable TTL (default 10 min); refreshes automatically on cache miss or stale keys.
- **`Store`** — persists provider config and roles inside the encrypted barrier.
- **`Role`** — binds `bound_subject`, `bound_claims` (arbitrary JWT claims), `bound_audiences`, and `policies`; TTL per role.
- Idempotent match: first matching role wins.

**HTTP API**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/auth/jwt/login` | Exchange JWT → Tuck token (unauthenticated) |
| GET/PUT | `/v1/auth/jwt/config` | Read/write JWKS config (`jwks_uri`, `issuer`, `audience`, `default_ttl`) |
| GET/PUT/DELETE | `/v1/auth/jwt/role/{name}` | Manage roles |
| LIST | `/v1/auth/jwt/role/` | List all role names |

**Quick start**
```sh
# 1. Configure JWKS
curl -XPUT https://tuck:8200/v1/auth/jwt/config \
  -H "X-Tuck-Token: $ROOT" \
  -d '{"jwks_uri":"https://accounts.google.com/.well-known/jwks","issuer":"https://accounts.google.com"}'

# 2. Create a role
curl -XPUT https://tuck:8200/v1/auth/jwt/role/ci \
  -H "X-Tuck-Token: $ROOT" \
  -d '{"bound_claims":{"repository":"myorg/myrepo"},"policies":["ci-reader"],"ttl":"15m"}'

# 3. Login
curl -XPOST https://tuck:8200/v1/auth/jwt/login \
  -d "{\"jwt\":\"$ACTIONS_ID_TOKEN_REQUEST_TOKEN\"}"
```

#### Helm Chart (`deploy/helm/tuck`)

Single `helm install` deploys the full Tuck stack into Kubernetes.

**Components** (each independently toggleable):
- **Server** (`server.enabled=true`) — StatefulSet with PVC, configurable seal type, optional TLS, optional Raft HA, OTel endpoint.
- **Operator** (`operator.enabled=true`) — Deployment (2 replicas, leader election), watches TuckSecrets cluster-wide.
- **Webhook Injector** (`injector.enabled=false` by default) — opt-in; creates cert-manager Certificate + MutatingWebhookConfiguration.
- **CRD** (`crds.install=true`) — TuckSecret CRD with `helm.sh/resource-policy: keep`.

**Key values**
```yaml
server.sealType: dev | shamir | transit
server.persistence.enabled: true  # PVC-backed bbolt
server.cluster.enabled: false     # Raft HA
injector.enabled: false           # webhook injector (requires cert-manager)
crds.install: true
```

**Install**
```sh
helm install tuck deploy/helm/tuck \
  --namespace tuck-system --create-namespace \
  --set server.sealType=shamir \
  --set server.shamirSeal.n=5,server.shamirSeal.k=3
```

---

## [0.12.0] — 2026-06-11

### Added

#### OP-4 — Webhook Agent Injector

Secrets are now deliverable as files on a tmpfs volume inside Pods, bypassing Kubernetes etcd entirely. No secret value ever touches the K8s Secret API.

**`internal/injector/`**
- `Handler` — HTTP mutating admission webhook; handles `POST /mutate`.
- `BuildPatch` — produces a RFC 6902 JSON Patch that adds:
  - A `tuck-secrets` `emptyDir{medium: Memory}` (tmpfs) volume.
  - A `tuck-agent` init container that fetches secrets before app containers start.
  - A read-only `/tuck/secrets` volume mount in every app container.
- Idempotent: repeated calls on already-injected Pods produce no patch.
- `ParseAnnotations` / `ParseSecretsList` — extract config from Pod annotations.

**`cmd/tuck-agent/`** — init container binary
- Reads `TUCK_ADDR`, `TUCK_TOKEN_FILE` (or `TUCK_TOKEN`), `TUCK_SECRETS`, `TUCK_OUTPUT_DIR`.
- Fetches each secret via `pkg/client`, writes files atomically (`.tmp` → rename) with mode `0400`.
- Fails fast if any secret is missing — Pod creation is blocked until all secrets are available.

**`cmd/tuck-injector/`** — webhook server binary
- HTTPS server (`--tls-cert` / `--tls-key` from cert-manager or custom CA).
- `--agent-image` flag to pin the tuck-agent image version.
- `/healthz` and `/readyz` probes, graceful shutdown.

**`deploy/webhook/`** — Kubernetes manifests
- `rbac.yaml` — ServiceAccount + ClusterRole for the injector.
- `deployment.yaml` — 2-replica Deployment + Service (port 443→8443).
- `cert.yaml` — cert-manager `Certificate` + self-signed `Issuer` for webhook TLS.
- `webhook.yaml` — `MutatingWebhookConfiguration` with `failurePolicy: Ignore` (never blocks pods on injector outage), namespace selector `tuck.io/inject=enabled`, object selector `tuck.io/inject=true`.
- `example-pod.yaml` — annotated Pod showing all supported annotations.

**Pod annotations**

| Annotation | Required | Default | Description |
|---|---|---|---|
| `tuck.io/inject` | yes | — | Set to `"true"` to enable injection |
| `tuck.io/addr` | yes | — | Tuck server URL |
| `tuck.io/secrets` | yes | — | `"path:filename,..."`  pairs |
| `tuck.io/token-secret` | no | `tuck-token` | K8s Secret with `token` key |
| `tuck.io/output-dir` | no | `/tuck/secrets` | Secrets directory in Pod |
| `tuck.io/agent-image` | no | `ghcr.io/nagenaev/tuck-agent:latest` | Override agent image |
| `tuck.io/insecure` | no | `false` | Skip TLS verification |

**Release pipeline updates**
- goreleaser builds `tuck-injector` and `tuck-agent` for `linux/{amd64,arm64}`.
- Docker images: `ghcr.io/nagenaev/tuck-injector` and `ghcr.io/nagenaev/tuck-agent`.
- `build/Dockerfile.injector` and `build/Dockerfile.agent` (distroless, uid 65532).

---

## [0.11.0] — 2026-06-11

### Added

#### HA-1 — Raft-replicated backend (`internal/physical/raft`)
- New `physraft.Backend` implementing `physical.Backend` via Hashicorp Raft consensus.
- **All writes replicated** through the Raft log — AES-256-GCM ciphertext is still the only thing that ever hits storage; Raft adds consensus on top, not cleartext.
- **FSM** backed by bbolt (`fsm.db`): applies `put`/`delete` commands committed by the cluster leader. Snapshot/restore support for log compaction.
- **Persistent stores**: Raft log + stable store in `raft.db` (raft-boltdb/v2), file-based snapshot store.
- **TCP transport** with configurable `BindAddr` and `AdvertiseAddr` for multi-node setups.
- `ErrNotLeader` — write operations on followers return a typed error; the HTTP layer maps it to `503 not leader`.
- `Backend.Status()` — real-time cluster topology (leader ID, leader addr, all servers, suffrage).
- `Backend.AddVoter` / `Backend.RemoveServer` — online membership changes from the leader.

#### Cluster HTTP API (`/v1/sys/cluster`)
- `GET /v1/sys/cluster` — returns cluster topology (is_leader, leader, servers).
- `POST /v1/sys/cluster/join` — adds a voter to a running cluster (`{"node_id","raft_addr"}`); must be called against the leader.
- `DELETE /v1/sys/cluster/node/{id}` — removes a voter from the cluster.

#### Server flags (`tuck --cluster ...`)
- `--cluster` — enable Raft HA backend (replaces bbolt).
- `--cluster-node-id` — stable node identity (defaults to hostname).
- `--cluster-bind-addr` — Raft RPC listen address (default `0.0.0.0:8201`).
- `--cluster-advertise` — advertised Raft address for peer discovery.
- `--cluster-dir` — data directory for Raft logs + FSM state (default `tuck-raft/`).
- `--cluster-bootstrap` — bootstrap a fresh cluster (first node only; idempotent on restart).
- `--cluster-peers` — comma-separated `id=raftAddr` list for multi-node bootstrap.
- `--cluster-join` — auto-join an existing cluster by POSTing to the leader's HTTP API.

---

## [0.10.0] — 2026-06-11

### Added
- **Go SDK** (`pkg/client`) — typed Go client for the full Tuck API: seal management, KV v1/v2, tokens, policies. Supports `WithInsecure()` and `WithHTTPClient()` options.
- **goreleaser** (`.goreleaser.yaml`) — automated release pipeline: linux/darwin/windows × amd64/arm64 binaries, Docker images (`ghcr.io/nagenaev/tuck`, `ghcr.io/nagenaev/tuck-operator`), SHA-256 checksums, cosign keyless signing, syft SBOM.
- **GitHub Actions release workflow** (`.github/workflows/release.yml`) — triggered on `v*` tags; runs goreleaser, signs artifacts with cosign, publishes to GHCR.

---

## [0.10.0-beta.1] — 2026-06-11

Pre-release for M10 testing.

---

## [0.9.0] — 2026-06-11 (M8 + M9)

### Added

#### KV v2 — Versioned secrets (`/v2/secret/*`)
- Every write creates a new immutable version (auto-incremented version number).
- **CAS** (check-and-set) via `?cas=N` — atomic conditional write.
- **Soft-delete** (`DELETE ?versions=1,2`) and **undelete** (`POST /v2/secret/undelete/`).
- **Destroy** (`POST /v2/secret/destroy/`) — permanent, unrecoverable data removal.
- **max_versions** — configurable retention limit; oldest versions auto-destroyed.
- Version metadata (`GET/PUT/DELETE /v2/secret/metadata/`).

#### Operator — HA & reliability
- **Leader election** (`--leader-elect`) — `coordination.k8s.io/v1` Lease-based; only the leader pod reconciles.
- **Status conditions** — `Synced` and `Ready` conditions on `TuckSecret.status`.
- **Deletion policy** — `spec.deletionPolicy: Retain | Delete`; finalizer `tuck.io/cleanup` ensures cleanup runs before garbage collection.
- Exponential backoff in the watch-reconcile loop.

#### Observability & API
- **OpenTelemetry tracing** (`--otel-endpoint`) — OTLP HTTP exporter; noop when empty.
- **OpenAPI 3.0 spec** — embedded in binary, served at `GET /openapi.json`.
- **Embedded web dashboard** at `/ui/` — login, secrets browser, token & policy management.
- **Prometheus metrics** at `/metrics`.

#### Security & operations
- **Threat model** (`docs/THREAT_MODEL.md`).
- **Tamper-evident audit log** — SHA-256 hash chain; values never logged.
- **Backup/restore** — `GET /v1/sys/snapshot` (bbolt `Tx.WriteTo`).
- **Key rotation** — `POST /v1/sys/rotate` re-wraps DEK; no data re-encryption.
- **Per-IP rate limiting** — token bucket.
- **TLS** — ECDSA P-256 self-signed (`--tls-auto`) or external cert.
- **Graceful shutdown** — 30-second drain + seal on exit.

#### CLI (`tuckcli`)
- Full KV, token, and policy management.
- `TUCK_ADDR` / `TUCK_TOKEN` env vars.

#### Community
- `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `CODEOWNERS`.
- Issue templates (bug report, feature request).
- Operations runbook (`docs/RUNBOOK.md`).
- k6 load test script (`test/load/k6_soak.js`).

---

## [0.4.0] — M0–M4 (foundation)

### Added
- **M0** — AES-256-GCM envelope encryption (barrier), bbolt backend, dev seal, KV HTTP API.
- **M1** — Token authentication, path-based ACL policies (glob-matching).
- **M2** — Kubernetes ServiceAccount auth via `TokenReview` API.
- **M3** — `TuckSecret` CRD + operator controller; `deploy/` manifests.
- **M4** — Production seals: Shamir secret sharing (n-of-k), Transit (Vault-compatible KMS).

---

[Unreleased]: https://github.com/NAGenaev/tuck/compare/v0.13.0...HEAD
[0.13.0]: https://github.com/NAGenaev/tuck/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/NAGenaev/tuck/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/NAGenaev/tuck/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/NAGenaev/tuck/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/NAGenaev/tuck/compare/v0.4.0...v0.9.0
[0.4.0]: https://github.com/NAGenaev/tuck/releases/tag/v0.4.0
