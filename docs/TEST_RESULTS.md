# Tuck - test results

**Run date:** 2026-06-11 15:33
**Environment:** Windows, go1.23.4, minikube v1.38.1

| ID | Test | Result | Details |
|----|------|--------|---------|
| UNIT | go test ./... | PASS |  |
| SHAMIR | Shamir seal unit tests | PASS |  |
| SHAMIR-HTTP | TestSysShamirUnseal integration | PASS |  |
| BUILD | go build tuck.exe | PASS |  |
| SETUP | Extract root token | PASS | tuck_OFYg_YwJ0zgndAcZlm3ZSnDwi5N1V7EupONiIERk7c8 |
| SETUP2 | Server ready | PASS |  |
| 1 | Health check | PASS | {"sealed":false} |
| 2 | Seal status | PASS | {"sealed":false,"type":"dev"} |
| 3a | Write secret | PASS | status 204 |
| 3b | Read secret | PASS | {"path":"db/password","value":"supersecret123"} |
| 3c | 404 nonexistent | PASS | status 404 |
| 3d | Delete secret | PASS | status 204 |
| 3e | Deleted returns 404 | PASS | status 404 |
| 4a | No token 401 | PASS | status 401 |
| 4b | Bad token 401 | PASS | status 401 |
| 5a | Create policy | PASS | status 204 |
| 5b | Read policy | PASS | {"name":"prod-readonly","rules":[{"path":"secret/prod/*","capabilities":["rea... |
| 5c | Create limited token | PASS | tuck_BA8gkia3DJq_c8g-nLhY9KZ57pZifxXvGDgHf1nP9s4 |
| 5d | Limited can read prod | PASS | {"path":"prod/api-key","value":"prod-api-key-value"} |
| 5e | Limited cannot write | PASS | status 403 |
| 5f | Limited cannot read staging | PASS | status 403 |
| 6a | Create token with TTL | PASS | tuck_wluynEvo7aVL6QypWWU5eviKgKba4EP5JEU0oouZaR4 |
| 6b | Get token | PASS | status 200 |
| 6c | Revoke token | PASS | status 204 |
| 6d | Revoked token 401 | PASS | status 401 |
| 7a | Manual seal | PASS | status 200 |
| 7b | Sealed status | PASS | {"sealed":true,"type":"dev"} |
| 7c | Sealed 503 | PASS | status 503 |
| 7d | Dev unseal API 400 | PASS | status 400 |
| 7e | Restart auto-unseal | PASS | {"sealed":false} |
| 10a | No new root token on restart | PASS |  |
| 10b | Persist after restart | PASS | {"path":"persist-test","value":"should-survive-restart"} |
| K8S-TOKEN | K8s root token available | PASS | tuck_XSajRCW-p9pObKo... |
| 8 | K8s SA auth | PASS | {"path":"app/config","value":"app-config-value"} |
| 9a | TuckSecret sync | PASS | value=new-postgres-secret-abc |
| 9b | TuckSecret CRD | PASS |  |
| 10k | K8s pod restart persist | PASS | {"path":"k8s-persist-test","value":"survives-pod-restart"} |

## Summary

- **Passed: 37 / 37**
- **Failed: 0**
- **Date:** 2026-06-11 (Windows 10, Go 1.23.4, minikube v1.38.1, k8s 1.35.1)

### Coverage map

| Block | Tests | Status |
|-------|-------|--------|
| Unit (`go test ./...`) | UNIT, SHAMIR, SHAMIR-HTTP | PASS |
| Local API (tests 1-7) | Health, KV, auth, ACL, tokens, seal | PASS |
| Local persist (test 10) | 10a, 10b | PASS |
| Minikube (tests 8-9) | SA auth, TuckSecret operator | PASS |
| Minikube persist (10k) | Pod restart + PVC | PASS |

Scenarios: [TESTING.md](TESTING.md) | Rerun: `.\scripts\run-all-tests.ps1`
