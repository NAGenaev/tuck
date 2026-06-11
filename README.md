# Tuck

> The simplest k8s-native secrets manager. Tuck a secret away — no ceremony.

Tuck is an open-source secrets manager. The pitch is **anti-Vault**: a single
static binary, no external database, and **auto-unseal by default** — so there
is no manual unseal ritual to perform on every restart. First-class target is
Kubernetes.

**Status:** Milestone 0 — the cryptographic core works end to end. Not for
production yet.

## Why another one?

[OpenBao](https://openbao.org) (the MPL fork of Vault) and
[Infisical](https://infisical.com) already exist. Tuck's wedge is **operational
simplicity**: one binary, zero external deps (no Postgres, no Redis), sealed by
design, GitOps-friendly. `kubectl apply` and it runs.

## Architecture

```
HTTP API (net/http, no framework)
  └─ core      orchestration + logical KV
       └─ barrier   CRYPTO CORE: envelope encryption (root key → DEK → data)
            └─ physical   Backend interface → bbolt (single file) | in-memory
  seal   unseal strategy: dev (M0) → kms / shamir (later)
```

Key hierarchy (envelope encryption):

```
root key  --AES-GCM-->  barrier key (DEK)  --AES-GCM-->  secret data
```

The root key lives only in memory and comes from the seal. The barrier key is
generated once, wrapped with the root key, and stored as the keyring. Rotating
the root key later means re-wrapping the keyring — never re-encrypting data.

## Quickstart

```sh
go run ./cmd/tuck
# tuck: unsealed (dev seal), serving on http://127.0.0.1:8200

curl -X PUT http://127.0.0.1:8200/v1/secret/db/password -d 'hunter2'
curl     http://127.0.0.1:8200/v1/secret/db/password
# {"path":"db/password","value":"hunter2"}

curl http://127.0.0.1:8200/v1/health
# {"sealed":false}
```

> The **dev seal** stores the root key in plaintext on disk (`tuck-dev-rootkey`).
> It is insecure by design, for local development only.

## Test

```sh
go test ./...
```

### Local Kubernetes (minikube)

See [docs/MINIKUBE.md](docs/MINIKUBE.md) for a step-by-step guide (Windows +
Docker Desktop). Full test scenarios: [docs/TESTING.md](docs/TESTING.md).
Latest results: [docs/TEST_RESULTS.md](docs/TEST_RESULTS.md) (37/37 PASS).
E2E demo: [docs/E2E-DEMO.md](docs/E2E-DEMO.md). Deploy layout: [deploy/README.md](deploy/README.md).

## Roadmap

- **M0 — crypto core (done):** barrier (envelope encryption), bbolt backend,
  dev seal, KV HTTP API, integration tests.
- **M1 — identity (done):** token auth, path-based ACL policies.
- **M2 — k8s-native (done):** Kubernetes auth via the TokenReview API; short-lived
  tokens mapped from `namespace/serviceaccount` to policy.
- **M3 — operator (done):** `TuckSecret` CRD + controller that syncs secrets into
  native K8s Secrets; manifests in `deploy/` for one-command install
  installation.
- **M4 — production seals:** KMS auto-unseal, Shamir fallback.
- **M5 — HA:** Raft-replicated storage backend.

## License

TBD (target: a genuinely open license — MPL-2.0 or Apache-2.0).
