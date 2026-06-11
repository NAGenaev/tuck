# Tuck — Threat Model

> Version: 0.9 · Last updated: 2026-06-11

---

## 1. Overview

Tuck is a Kubernetes-native secrets manager: single binary, bbolt backend, AES-256-GCM envelope encryption.
This document enumerates protected assets, trust boundaries, threat actors, attack scenarios, and mitigations.

---

## 2. Assets

| Asset | Sensitivity | Location |
|---|---|---|
| Root key | **Critical** | Memory only (cleared after unseal) |
| Barrier DEK (data encryption key) | **Critical** | Memory only (cleared on seal) |
| Encrypted keyring `barrier/keyring` | High | bbolt file, disk |
| Encrypted secret values | High | bbolt file, disk |
| Root token | **Critical** | Shown once at init; caller must protect |
| Service tokens | High | Encrypted in bbolt; ID exposed in headers |
| Shamir shares | **Critical** | Distributed to operators; combined reconstruct root key |
| Audit log | High | Disk / stdout |
| TLS private key | High | Disk (operator-provided) or ephemeral (self-signed) |

---

## 3. Trust Boundaries

```
┌──────────────── Cluster boundary ──────────────────────┐
│                                                         │
│   Operators / CI   ──HTTPS──►  Tuck server             │
│                               │                        │
│   K8s workloads   ──SA JWT──► │  (k8s auth)            │
│                               │                        │
│   Tuck Operator   ──HTTPS──►  │  (reads secrets)       │
│                               │                        │
│                            [bbolt]  ← encrypted at rest│
│                               │                        │
│             ┌─────────────────┘                        │
│        Transit Seal / KMS  (external, optional)        │
└─────────────────────────────────────────────────────────┘
```

**In-scope trust boundary crossings:**
- HTTP(S) API (bearer token + optional mTLS)
- Kubernetes TokenReview (SA JWT → Tuck token)
- Seal backends (dev file, Shamir memory, Transit HTTP)
- bbolt database file (disk I/O, snapshot transfer)

**Out of scope (delegated):**
- K8s etcd encryption at rest (operator must enable)
- Kubernetes RBAC on the namespace where K8s Secrets land
- Network policy between pods
- HSM / cloud KMS internal security (Transit seal)

---

## 4. Threat Actors

| Actor | Capability | Scenario |
|---|---|---|
| External attacker | Network access to exposed port | Credential brute-force, DoS |
| Compromised workload | Valid K8s SA JWT | Access to only bound paths |
| Malicious insider (operator) | Physical access to node / bbolt file | Offline decryption attempt |
| Compromised operator laptop | Stolen Shamir share | Partial key reconstruction |
| Compromised CI pipeline | Valid short-lived token | Read secrets within policy |
| Rogue Kubernetes node | Read pod memory / disk | Dump process memory, read bbolt |

---

## 5. Attack Scenarios

### 5.1 Brute-Force Token Guessing
- **Risk:** High without mitigation; tokens are 32 random bytes → 2²⁵⁶ search space
- **Token format:** `tuck_` + base64url(32 random bytes) — not guessable in practice
- **Mitigation:** Per-IP token bucket rate limiter (M6, `internal/ratelimit`); 401 returns no info on whether token exists vs. expired
- **Residual:** Tokens stored by ID in barrier; a leaked bbolt snapshot reveals encrypted token blobs but not values

### 5.2 bbolt File Exfiltration (Disk Access)
- **Risk:** Attacker with filesystem access reads `tuck.db`
- **Data exposed:** Encrypted blobs only. AES-256-GCM with random nonce per write.
- **Not exposed without root key:** any plaintext secret, token values, policy rules
- **Mitigation:** AES-256-GCM; root key never persisted (dev seal is explicitly dev-only); snapshot backup endpoint requires root token
- **Residual:** Key names (paths) are stored in plaintext as bbolt keys → reveals secret namespacing. Future: encrypt key names.

### 5.3 Memory Dump (Cold-Boot / VM Snapshot)
- **Risk:** Attacker with hypervisor or physical access dumps process memory
- **Data exposed:** Root key and DEK are in heap between unseal and seal
- **Mitigation (M6):** `clear()` called on root key slice immediately after `barrier.Unseal()`. DEK zeroed on `barrier.Seal()`.
- **Residual:** Go garbage collector may copy slices before zeroing; `sync.Pool` reuse not used for key material. Go runtime doesn't `mlockall` — keys can be swapped to disk on Linux unless `--mlock` is added (future: SEC-9).

### 5.4 Shamir Share Compromise
- **Risk:** One or more Shamir shares are stolen
- **k-of-n threshold:** Attacker needs ≥ k shares; k ≤ n/2 → majority compromise required
- **Mitigation:** Shares are only produced once at init and at `POST /v1/sys/rotate`; rotate immediately on suspected compromise
- **Residual:** Shares are base64url-encoded 33-byte values; protect like private keys

### 5.5 Transit Seal Token Compromise
- **Risk:** Vault/KMS token stolen → attacker can call decrypt endpoint
- **Mitigation:** The wrapped key ciphertext is stored locally; attacker needs both the ciphertext file AND the transit token to get the root key
- **Residual:** Rotate transit token and rekey Tuck immediately on suspected compromise

### 5.6 Audit Log Tampering
- **Risk:** Attacker deletes or edits audit entries to cover access
- **Mitigation (M6):** SHA-256 hash chain — each entry includes `prev_hash`; offline verification detects any gap or modification
- **Residual:** Attacker who controls the Tuck process can truncate the log file; stream to an immutable sink (S3, Loki) in production

### 5.7 API Replay Attack
- **Risk:** Attacker captures valid request and replays it
- **Not applicable for reads:** GET /v1/secret/* is idempotent; replay returns same data
- **Mitigation for writes:** TLS prevents capture; token revocation invalidates stolen tokens
- **Residual:** No request signing or nonce; rely on TLS + short-lived tokens

### 5.8 DoS — Slowloris / Resource Exhaustion
- **Mitigation (M5):** `ReadHeaderTimeout: 5s`, `ReadTimeout: 30s`, `WriteTimeout: 30s`, `MaxHeaderBytes: 1MiB`
- **Mitigation (M6):** Per-IP rate limiting (token bucket)
- **Residual:** No global connection concurrency limit; consider `net/http` `http.Server.MaxConnsPerHost` or a reverse proxy in production

### 5.9 Operator CRD Privilege Escalation
- **Risk:** Compromised operator pod reads arbitrary secrets if RBAC is too broad
- **Mitigation:** Operator authenticates via K8s SA JWT → bound Tuck role with minimal policies; TuckSecret spec must name exact `tuckPath`
- **Residual:** A misconfigured role binding can grant broad read; enforce least-privilege via operator RBAC review

---

## 6. In Scope / Out of Scope

| Scenario | In Scope |
|---|---|
| API endpoint authentication and authorisation | ✅ |
| Secret data encryption at rest | ✅ |
| Root key and DEK memory hygiene | ✅ |
| TLS for API transport | ✅ |
| Audit log integrity | ✅ |
| Shamir secret sharing correctness | ✅ |
| K8s SA JWT validation (TokenReview) | ✅ |
| Physical node security | ❌ (host OS concern) |
| K8s etcd encryption | ❌ (cluster operator concern) |
| Network policy (which pods can reach Tuck) | ❌ (cluster operator concern) |
| HSM / cloud KMS internal security | ❌ (vendor concern) |
| Go runtime memory safety | ❌ (upstream Go concern) |

---

## 7. Security Properties Summary

| Property | Mechanism |
|---|---|
| Confidentiality at rest | AES-256-GCM envelope encryption |
| Confidentiality in transit | TLS 1.2+ ECDHE-only ciphers |
| Authentication | Bearer token (random 32 bytes) or K8s SA JWT |
| Authorisation | Path-glob ACL policies |
| Key splitting | Shamir's Secret Sharing over GF(256) |
| Audit | Tamper-evident SHA-256 hash chain |
| Rate limiting | Per-IP token bucket |
| Memory hygiene | `clear()` on root key after use; barrier key zeroed on seal |
