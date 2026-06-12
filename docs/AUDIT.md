# Tuck v1.0-rc Security Audit Report

**Date:** 2026-06-12  
**Scope:** v0.34.0 codebase (commit 4d60193) → v1.0-rc  
**Tools:** `govulncheck`, `gosec`, `go vet`, manual review  
**Result: PASS — 0 open findings**

---

## 1. Vulnerability scan (govulncheck)

```
govulncheck ./...
```

**Before:** 10 vulnerabilities in Go standard library (all in `go1.25.8`):

| ID | Package | Fixed in |
|----|---------|---------|
| GO-2026-5039 | net/textproto | go1.25.11 |
| GO-2026-5037 | crypto/x509 | go1.25.11 |
| GO-2026-4982 | html/template | go1.25.10 |
| GO-2026-4980 | html/template | go1.25.10 |
| GO-2026-4971 | net | go1.25.10 |
| GO-2026-4947 | crypto/x509 | go1.25.9 |
| GO-2026-4946 | crypto/x509 | go1.25.9 |
| GO-2026-4918 | net/http | go1.25.10 |
| GO-2026-4870 | crypto/tls | go1.25.9 |
| GO-2026-4865 | html/template | go1.25.9 |

**Fix:** Upgraded toolchain to `go 1.25.11` in `go.mod`.

**After:** `No vulnerabilities found.`

---

## 2. Static analysis (gosec)

```
gosec -severity medium -exclude-generated ./...
```

**Before:** 27 findings across 81 files.

**After:** `Issues: 0`

### Finding disposition

| Rule | Location | Disposition | Rationale |
|------|----------|-------------|-----------|
| G101 (hardcoded credentials) | `token/store.go`, `approle/approle.go`, `injector/patch.go` | False positive | String constants are bbolt path prefixes / K8s annotation names, not credential values |
| G402 (InsecureSkipVerify) | `tuckcli/main.go`, `pkg/client/client.go` | Accepted, annotated | Controlled by explicit `--insecure` CLI flag; default is false |
| G402 (InsecureSkipVerify) | `auth/ldap/ldap.go` | Accepted, annotated | Controlled by operator config field `TLSInsecure`; default is false |
| G505 (crypto/sha1) | `dynamic/totp/totp.go` | Accepted, annotated | RFC 6238 (TOTP) mandates SHA-1 as the base algorithm; we also support SHA-256/512 |
| G115 (int overflow) | `dynamic/ssh/ssh.go`, `dynamic/totp/totp.go` | Accepted, annotated | `time.Unix()` is always non-negative for any post-1970 date; SSH cert validity and TOTP counters are safe |
| G304/G703 (path traversal) | Multiple (config, audit, seal, k8s, agent) | False positive, annotated | All file paths are operator-supplied via CLI flags or env vars in a single-tenant daemon context |
| G704 (SSRF) | `tuckcli/main.go` | False positive, annotated | CLI tool — user intentionally supplies the server address |

---

## 3. Go vet

```
go vet ./...
```

**Result:** No issues.

---

## 4. Manual review — critical paths

### 4.1 Barrier (AES-256-GCM envelope encryption)

File: `internal/barrier/barrier.go`

- Root key generated with `crypto/rand` ✅
- Per-entry DEK derived via `crypto/rand` ✅  
- AES-256-GCM: 32-byte key, 12-byte nonce from `crypto/rand` ✅
- Nonce uniqueness: fresh `crypto/rand` nonce per encrypt call ✅
- Sealed state: `Get`/`Put` refuse all operations when barrier is sealed ✅
- Key rotation: new root key wraps existing DEK; no data re-encryption needed ✅

### 4.2 Shamir Secret Sharing

File: `internal/shamir/shamir.go`

- GF(256) implementation over AES S-box polynomial ✅
- Shares generated with `crypto/rand` ✅
- Minimum k=2 enforced ✅
- No timing side-channel: reconstruction is constant-time xor in GF(256) ✅

### 4.3 Token authentication

File: `internal/token/store.go`, `internal/api/tokens.go`

- Token IDs are 256-bit random (32 bytes from `crypto/rand`) ✅
- Token IDs stored under SHA-256(id) — raw bearer never in storage keys (SEC-1 fix) ✅
- Accessor is a separate random identifier; decoupled from bearer token ✅
- Expired tokens rejected before handler dispatch ✅
- MaxUses enforced atomically in core before each request ✅

### 4.4 ACL policy enforcement

File: `internal/policy/policy.go`

- Deny rules evaluated before allow rules (deny-wins) ✅
- Glob matching uses standard `path.Match` ✅
- Root token bypasses ACL (by design; documented in threat model) ✅
- Policy stored encrypted through barrier ✅

### 4.5 Audit log

File: `internal/audit/audit.go`

- Every authenticated request logged before handler executes (fail-closed) ✅
- Secret values never included in log entries ✅
- SHA-256 hash chain: each entry includes `prev_hash` ✅
- Log file permissions: 0600 ✅

### 4.6 TLS

- Dev mode: ECDSA P-256 self-signed, generated fresh on each start ✅
- Production: operator supplies cert/key files ✅
- Default cipher suite selection delegated to Go TLS stack (TLS 1.2+) ✅
- `InsecureSkipVerify` only when operator explicitly passes `--insecure` ✅

### 4.7 Transit engine (encryption-as-a-service)

File: `internal/dynamic/transit/transit.go`

- Key material stored encrypted through barrier ✅
- Key rotation: new version appended; old versions retained for decryption ✅
- Decrypt rejects ciphertext version beyond current key version ✅
- Rewrap re-encrypts under the latest key version ✅

---

## 5. Known limitations (out of scope for v1.0)

| ID | Description | Severity | Plan |
|----|-------------|----------|------|
| SEC-6 | No `mlockall` — root key may be swapped to disk under memory pressure | Low | v1.x |
| OPS-7 | Audit log rotation not implemented — file grows unbounded | Low | v1.x |
| INF-1 | No rate limiting on KV/token endpoints (only auth/unseal) | Low | v1.x |

---

## 6. Conclusion

All automated tool findings resolved. No High/Critical open issues. The codebase
is ready for **v1.0-rc** tagging and external security review.

**Recommended external audit focus areas:**
1. Barrier AES-256-GCM implementation — key derivation and nonce uniqueness
2. Shamir GF(256) arithmetic — correctness and side-channel resistance
3. Token ID hashing (SEC-1 fix) — forward-secrecy properties
4. ACL policy engine — bypass edge cases in glob matching
5. Rate limiter — bypass via IPv6 or proxy headers
