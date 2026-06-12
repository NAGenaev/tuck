# Getting Started with Tuck

This guide takes you from zero to a working Tuck installation in under 10 minutes.

---

## Prerequisites

- Go 1.21+ (to build from source) **or** a pre-built binary from [Releases](https://github.com/NAGenaev/tuck/releases)
- For Kubernetes deployment: `kubectl` + a running cluster, `helm` 3.x

---

## 1. Install

### Option A — Pre-built binary

```sh
# Linux / macOS (amd64)
curl -Lo tuck https://github.com/NAGenaev/tuck/releases/latest/download/tuck_linux_amd64
chmod +x tuck && mv tuck /usr/local/bin/

# CLI
curl -Lo tuckcli https://github.com/NAGenaev/tuck/releases/latest/download/tuckcli_linux_amd64
chmod +x tuckcli && mv tuckcli /usr/local/bin/
```

### Option B — Build from source

```sh
git clone https://github.com/NAGenaev/tuck
cd tuck
go install ./cmd/tuck ./cmd/tuckcli
```

---

## 2. Run locally (dev mode)

Dev mode auto-unseals on every start — perfect for local development and testing.

```sh
tuck --seal-type=dev --tls-auto
```

Expected output:

```
tuck: TLS enabled (auto-generated self-signed — dev only)
==========================================================
ROOT TOKEN (shown once — store it securely):
  tuck_XXXXXXXXXXXXXXXXXXXX
==========================================================
tuck: unsealed (dev seal) — https://127.0.0.1:8200
```

Copy the root token — you'll need it for every request.

---

## 3. Store and retrieve your first secret

```sh
export TUCK_ADDR=https://127.0.0.1:8200
export TUCK_TOKEN=tuck_XXXXXXXXXXXXXXXXXXXX   # paste your root token

# Store a secret
tuckcli kv put myapp/db-password "s3cr3t"

# Read it back
tuckcli kv get myapp/db-password
# {"path":"myapp/db-password","value":"s3cr3t"}

# List secrets under a prefix
tuckcli kv list myapp/
# {"keys":["db-password"]}
```

Or with `curl` (skip TLS verification for the self-signed cert):

```sh
curl -sk -X PUT https://127.0.0.1:8200/v1/secret/myapp/db-password \
  -H "X-Tuck-Token: $TUCK_TOKEN" -d 's3cr3t'

curl -sk https://127.0.0.1:8200/v1/secret/myapp/db-password \
  -H "X-Tuck-Token: $TUCK_TOKEN"
```

Open the dashboard at **https://127.0.0.1:8200/ui/** (accept the self-signed cert warning).

---

## 4. Create a scoped token and policy

Give an application read-only access to `myapp/*`:

```sh
# Create a policy
tuckcli policy put myapp-ro '{"paths":{"myapp/*":{"capabilities":["read","list"]}}}'

# Create a short-lived token with that policy
tuckcli token create --name=myapp --policy=myapp-ro --ttl=24h
# {"id":"tuck_YYYY...","ttl":"24h","policies":["myapp-ro"]}

# Use the new token
TUCK_TOKEN=tuck_YYYY... tuckcli kv get myapp/db-password   # OK
TUCK_TOKEN=tuck_YYYY... tuckcli kv put myapp/x y           # 403 Forbidden
```

---

## 5. Use a config file (recommended for production)

Instead of long flag lists, create `tuck.yaml`:

```yaml
addr: "0.0.0.0:8200"
data: "/var/lib/tuck/tuck.db"

tls:
  cert: "/etc/tuck/tls.crt"
  key:  "/etc/tuck/tls.key"

seal:
  type: "shamir"
  shamir:
    n: 5   # total shares
    k: 3   # shares required to unseal
```

Then start with just:

```sh
tuck --config=/etc/tuck/tuck.yaml
```

CLI flags override config file values. The `TUCK_CONFIG` environment variable sets the default config path.

### Sensitive values

Never put secrets in the config file. Use environment variables instead:

```sh
# Transit seal token — never pass via --seal-transit-token (visible in ps)
export TUCK_TRANSIT_TOKEN=hvs.XXXXXX
tuck --config=/etc/tuck/tuck.yaml
```

---

## 6. Production: Shamir seal

Shamir splits the root key into N shares — K are needed to unseal:

```sh
tuck --config=/etc/tuck/tuck.yaml   # first start generates shares
```

```
ROOT TOKEN: tuck_...
SHAMIR SHARES (distribute to operators):
  [1] a1b2c3...
  [2] d4e5f6...
  [3] ...
  [4] ...
  [5] ...
```

Distribute shares to separate operators. After a restart:

```sh
# Each operator submits their shard via CLI or dashboard
tuckcli unseal <shard-1>
tuckcli unseal <shard-2>
tuckcli unseal <shard-3>
# tuck: unsealed
```

---

## 7. Deploy on Kubernetes

### Helm (recommended)

```sh
helm repo add tuck https://nagenaev.github.io/tuck
helm install tuck tuck/tuck \
  --namespace tuck-system --create-namespace \
  --set seal.type=awskms \
  --set seal.awskms.keyId=alias/tuck-seal \
  --set seal.awskms.region=us-east-1
```

Tuck auto-unseals using IRSA (IAM Roles for Service Accounts) — no manual unseal steps.

### TuckSecret CRD

Sync a Tuck secret into a native Kubernetes Secret:

```yaml
apiVersion: tuck.io/v1alpha1
kind: TuckSecret
metadata:
  name: db-password
  namespace: myapp
spec:
  path: myapp/db-password
  destination:
    name: db-password
    key: password
```

```sh
kubectl apply -f tucksecret.yaml
kubectl get secret db-password -n myapp -o jsonpath='{.data.password}' | base64 -d
# s3cr3t
```

### Agent injector (sidecar)

Annotate your Pod to inject secrets directly into a tmpfs volume (secrets never touch etcd):

```yaml
metadata:
  annotations:
    tuck.io/inject: "true"
    tuck.io/role: "myapp"
    tuck.io/secret-path: "myapp/db-password"
    tuck.io/secret-dest: "/run/secrets/db-password"
```

---

## 8. Dynamic credentials

Instead of static passwords, generate short-lived credentials on demand:

```sh
# Database (PostgreSQL)
tuckcli db creds my-pg-role
# {"username":"v-myapp-abc123","password":"A1b2C3...","lease_duration":"1h"}

# AWS
tuckcli aws creds my-s3-role
# {"access_key":"AKIA...","secret_key":"...","session_token":"...","lease_duration":"1h"}
```

Credentials expire automatically. No rotation scripts needed.

---

## 9. PKI — issue a TLS certificate

```sh
# Issue a cert for a service
tuckcli pki issue my-role --cn=api.internal --ttl=720h --alt-name=api.svc.cluster.local
# -----BEGIN CERTIFICATE-----
# ...
```

---

## 10. Next steps

| Goal | Where to look |
|---|---|
| Full CLI reference | `tuckcli --help` |
| API reference | `https://127.0.0.1:8200/openapi.json` |
| HA cluster setup | `docs/RUNBOOK.md` |
| Security model | `docs/THREAT_MODEL.md` |
| Configuration reference | `docs/config-reference.md` (all YAML fields) |
| Contributing | `CONTRIBUTING.md` |

---

## Quick reference

```sh
# Server
tuck --config=tuck.yaml
tuck --seal-type=dev --tls-auto          # dev mode

# Secrets
tuckcli kv put <path> <value>
tuckcli kv get <path>
tuckcli kv delete <path>
tuckcli kv list [prefix]

# Tokens
tuckcli token create --policy=<p> --ttl=24h
tuckcli token lookup-self
tuckcli token revoke <id>

# Dynamic credentials
tuckcli db creds <role>
tuckcli aws creds <role>

# Crypto
tuckcli transit encrypt <key> <plaintext>
tuckcli pki issue <role> --cn=<name>
tuckcli totp code <key>

# Auth
tuckcli auth approle login --role-id=... --secret-id=...
```
