# Tuck v1.0-rc Security Audit Report

**Date:** 2026-06-12  
**Scope:** v0.34.0 codebase (commit 4d60193) → v1.0-rc  
**Tools:** `govulncheck`, `gosec`, `go vet`, manual review  
**Result: PASS — 0 open findings**

> **Refreshed 2026-07-26** (v1.37.0 + same-day work, ~1.5 months and ~90 commits after the pass above) — see [§7](#7-refresh-pass-2026-07-26) for what changed. Still PASS, 0 open findings, but two real issues (one dependency CVE pair, one missing HTTP timeout) had crept in since June and are fixed as of this refresh. This is a self-review refresh to keep the codebase audit-ready, not a substitute for the external audit still recommended in §6.

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
| INF-1 | ~~No rate limiting on KV/token endpoints (only auth/unseal)~~ — **Fixed v1.35**: per-IP and per-token limiters wired to all endpoints via `PUT /v1/sys/config`; limits survive restart (auto-unseal) and Shamir unseal | Low | ✅ |

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

---

## 7. Refresh pass (2026-07-26)

**Scope:** v1.37.0 + same-day commits (`1d44570`, `b98f769`, `28e9415`) — CSI live-refresh, Terraform provider AWS/GCP/Azure resources, and 3 integration-test side-finding fixes from earlier the same day. Re-ran the same tool set as §1–3 against current `main`, ahead of the still-open recommendation in §6 to get an external audit. Both new issues found are fixed as of this entry.

### 7.1 Vulnerability scan (govulncheck)

```
govulncheck ./...
```

**Before:** 2 reachable vulnerabilities (both accumulated since the June pass, not introduced by this session's own changes):

| ID | Package | Fixed in |
|----|---------|---------|
| GO-2026-5970 | golang.org/x/text (infinite loop on invalid input, reachable via `internal/dynamic/azure`) | v0.39.0 |
| GO-2026-5856 | crypto/tls (Encrypted Client Hello privacy leak) | go1.25.12 |

**Fix:** `go get golang.org/x/text@v0.39.0`; toolchain bumped `go1.25.11` → `go1.25.12` in `go.mod`.

**After:** `No vulnerabilities found.` (2 further vulnerabilities remain in the dependency tree — `go-ntlmssp`, `x/net/dns/dnsmessage`, `x/crypto/openpgp` — but govulncheck's call-graph analysis confirms none are reachable from Tuck's own code; monitored, not fixed.)

The separate `contrib/terraform-provider-tuck` Go module (not part of the main `govulncheck ./...` scope above — it's its own `go.mod`) had drifted much further: **31 reachable vulnerabilities**, `go 1.22.0` / `toolchain go1.23.4`, `terraform-plugin-framework v1.13.0`. Fixed by bumping to `go 1.25.12` and `go get -u ./...` (`terraform-plugin-framework` → v1.19.0 and its whole dependency tree). `govulncheck ./...` in that module now also reports `No vulnerabilities found.`

### 7.2 Static analysis (gosec)

```
gosec -severity medium -exclude-generated ./...
```

**Before:** 2 findings.

| Rule | Location | Disposition | Rationale |
|------|----------|-------------|-----------|
| G704 (SSRF) | `cmd/tuckcli/main.go:864` (`vaultClient.do`, the `tuckcli migrate` Vault-import helper) | Accepted, re-annotated | Same accepted pattern as the `tuckcli`/`pkg/client` entries in §2 — CLI tool, user supplies the Vault address via `--vault-addr`. gosec's taint-sink detection now lands one line later (on `v.http.Do`, not `http.NewRequest`) than when §2 was written; the existing `#nosec G704` comment didn't cover the new sink line. Added a second annotation on the actual sink. |
| G112 (Slowloris) | `cmd/tuck-operator/main.go:107` (`startHealthServer`) | **Fixed** | The operator's `/healthz` liveness listener had no `ReadHeaderTimeout`, leaving it open to slow-header resource exhaustion. Added `ReadHeaderTimeout: 5 * time.Second}`. |

**After:** `Issues: 0` (verified both with the audit's own `-severity medium -exclude-generated` invocation and CI's actual `-exclude=G104,G704,G706` invocation).

### 7.3 Go vet / test suite

`go vet ./...` and `go test ./... -short` (whole repo): clean, all green, both before and after the fixes above (the fixes touched only a dependency version and one `http.Server` field — no behavioral change to verify beyond the existing suite passing).

### 7.4 CI fallout from the toolchain bump

Bumping `go.mod`'s `go` directive to `1.25.12` (§7.1) broke the pinned `golangci-lint@v2.1.6` in CI's `lint` job outright — that binary was built with `go1.23.4`, and golangci-lint refuses to run at all against a module targeting a newer Go language version than it was itself built with (`can't load config: the Go language version ... is lower than the targeted Go version`). Confirmed locally, then fixed by re-pinning to `v2.12.2` (built with `go1.25.12`). Also ran the newly-working linter against the full repo for the first time in this pass and fixed the 5 issues it found — 4 pre-existing (`errcheck`/`ineffassign` in `internal/api/integration_e2e_test.go` and `internal/api/ratelimit_test.go`), 1 introduced earlier the same session (`staticcheck` De Morgan's-law suggestion in the Postgres revocation-statement test added in commit `79eef53`).

Separately: `contrib/terraform-provider-tuck` — the module that had drifted to 31 vulnerabilities (§7.1) — turned out to have **no CI coverage at all**; `.github/workflows/ci.yml`'s `test`/`lint`/`security` jobs only ever touch the root module. That's very likely *why* it drifted that far unnoticed. Added a `terraform-provider` CI job (build + vet + `govulncheck`, scoped to `contrib/terraform-provider-tuck`) so this doesn't silently recur. Also added a weekly `schedule: cron` trigger to the whole workflow (Mondays 06:00 UTC), so CVE-only drift — new vulnerabilities published against code that hasn't changed — gets caught even in weeks with no commits, rather than only at the next push.

Checking the actual CI run for this refresh (rather than trusting the local pass) surfaced one more, unrelated, pre-existing break: the `lint` and `security` jobs had been failing on every run — `internal/ui` embeds the web dashboard build directly via `//go:embed assets` (`internal/ui/embed.go`), and `internal/ui/assets/` is gitignored (only a `.gitkeep` placeholder is tracked; `web/vite.config.ts`'s `outDir` populates the real thing). Only the `test` job ran `npm run build` first; `lint` and `security` never did, so on every fresh checkout `go vet`/`golangci-lint`/`gosec` all failed immediately with `pattern assets: contains no embeddable files` before reaching any of their own checks. Verified by reproducing it locally (moved the built assets aside, confirmed `go build ./...` fails the same way, restored them) rather than trusting the theory alone. Fixed by adding the same `npm ci && npm run build` step to both jobs; also fixed `CONTRIBUTING.md`, whose local-build walkthrough had `go build ./...` before the web build instead of after, so a contributor following it top-to-bottom would hit this exact failure on a fresh clone.

### 7.5 Takeaway

Nothing found this pass was introduced by this session's own feature work (CSI refresh, Terraform resources) — the security-relevant issues were pre-existing drift: dependency CVEs published after the June scan, and a pre-existing missing timeout that gosec's rule set already covered in June but evidently didn't fire then (possibly a gosec version difference between passes; not reinvestigated further since the fix is trivial and unconditionally correct regardless of why it wasn't flagged in June). The real signal here is operational, not code-quality: **dependency drift accumulates within about six weeks even on a project this size**, and accumulates *fastest* in whatever isn't wired into CI — `contrib/terraform-provider-tuck` had zero CI coverage and drifted to 31 vulnerabilities before anyone looked; the root module, which CI does check, only drifted 2. Fixed the proximate issues (§7.1–7.3) and, more importantly, the structural gaps that let them accumulate unnoticed (§7.4): CI now covers the Terraform provider module too, runs on a weekly schedule regardless of commit activity, and uses a `golangci-lint` build that won't silently stop working the next time `go.mod`'s Go version moves. None of this replaces the external audit still recommended in §6 — it just means the codebase that audit eventually looks at will have fewer of these self-inflicted, easily-avoidable gaps in it.
