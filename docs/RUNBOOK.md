# Tuck — Operations Runbook

> Audience: on-call engineers, platform team  
> Version: 0.9

---

## 1. First Boot

### Dev seal (local testing only)

```bash
tuck --seal-type=dev --addr=127.0.0.1:8200
```

On first start Tuck prints:
```
ROOT TOKEN (shown once — store it securely):
  tuck_<base64url>
```

Save the root token in your password manager. It is **never** shown again.

---

### Shamir seal (production)

```bash
tuck --seal-type=shamir --seal-shamir-n=5 --seal-shamir-k=3 \
     --addr=0.0.0.0:8200 --tls-cert=/etc/tuck/tls.crt --tls-key=/etc/tuck/tls.key
```

On first start Tuck prints the root token **and** 5 Shamir shares:
```
ROOT TOKEN (shown once — store it securely):
  tuck_…
SHAMIR SHARES (distribute to operators — never store together):
  [1] <base64url>
  [2] <base64url>
  [3] <base64url>
  [4] <base64url>
  [5] <base64url>
```

**Immediately:**
1. Record the root token in an offline secure location.
2. Distribute each share to a different operator via a secure channel.
3. No single operator should hold ≥ k shares.

---

## 2. Unsealing After Restart

After a process restart with Shamir seal, Tuck starts in **sealed** state.
You must submit k-of-n shares via the API (or CLI):

```bash
# Using tuckcli
export TUCK_ADDR=https://tuck.example.com:8200
tuckcli unseal <shard-1>
tuckcli unseal <shard-2>
tuckcli unseal <shard-3>   # "unsealed successfully"
```

```bash
# Using curl
curl -X POST https://tuck:8200/v1/sys/unseal \
  -H "Content-Type: application/json" \
  -d '{"key":"<shard-base64url>"}'
```

Check status:
```bash
tuckcli status
# or
curl https://tuck:8200/v1/sys/seal-status
```

---

## 3. Sealing Manually

```bash
tuckcli --token=$ROOT_TOKEN seal
# or
curl -X POST https://tuck:8200/v1/sys/seal \
  -H "X-Tuck-Token: $ROOT_TOKEN"
```

This drops the in-memory DEK immediately. All subsequent API calls return 503
until the server is unsealed again.

---

## 4. Backup and Restore

### Taking a snapshot

```bash
curl -H "X-Tuck-Token: $ROOT_TOKEN" \
     https://tuck:8200/v1/sys/snapshot \
     -o tuck-$(date +%Y%m%d-%H%M%S).db
```

Snapshots are binary bbolt files. The data inside is **still encrypted**.
Store snapshots in a secure location (S3, encrypted volume). Even if the snapshot
leaks, data cannot be decrypted without the root key.

### Restoring from a snapshot

1. Stop the Tuck server.
2. Replace `tuck.db` with the snapshot file.
3. Restart. The server will unseal normally (same root key required).

> ⚠️ After restoring a snapshot, any tokens/secrets written after the snapshot
> was taken are lost. Revoke all tokens and rotate secrets as a precaution.

---

## 5. Key Rotation

### Rotate root key (re-wraps DEK, no data re-encryption needed)

```bash
tuckcli --token=$ROOT_TOKEN rotate
```

For Shamir seals, new shares are printed in the response. **Distribute them
immediately** — the old shares become invalid after rotation.

### When to rotate
- Suspected compromise of the root key or Shamir shares
- Periodic rotation policy (e.g., annually)
- After removing an operator who held shares

---

## 6. Token Management

### Create a service token

```bash
tuckcli --token=$ROOT_TOKEN token create \
  --name=ci-pipeline --policy=ci-read-only --ttl=24h
```

### List all tokens

```bash
tuckcli --token=$ROOT_TOKEN token list
```

### Revoke a token

```bash
tuckcli --token=$ROOT_TOKEN token revoke tuck_<id>
```

### Renew a token

```bash
tuckcli --token=$ROOT_TOKEN token renew tuck_<id> 48h
```

### Expired tokens

Tuck GC runs every 15 minutes and removes expired tokens automatically.
To force GC: restart the server (it runs on startup).

---

## 7. Policy Management

### Create a read-only policy for a path prefix

```bash
tuckcli --token=$ROOT_TOKEN policy put ci-read-only \
  '[{"path":"secret/ci/*","capabilities":["read","list"]}]'
```

### List policies

```bash
tuckcli --token=$ROOT_TOKEN policy list
```

### Delete a policy

```bash
tuckcli --token=$ROOT_TOKEN policy delete old-policy
```

---

## 8. Kubernetes Operator

### Check operator is running

```bash
kubectl -n tuck-system get pods
kubectl -n tuck-system logs -l app=tuck-operator
```

### TuckSecret status

```bash
kubectl get ts -A
# Shows: SYNCED | TuckPath | SecretName | LastSynced | Age
```

### Force re-sync

```bash
kubectl annotate ts <name> tuck.io/force-sync="$(date)" --overwrite
```

The annotation change triggers a MODIFIED watch event → reconcile.

### Leader election (HA operator)

When running multiple operator replicas with `--leader-elect`:

```bash
kubectl -n tuck-system get lease tuck-operator-leader -o yaml
```

Look for `holderIdentity` — only that pod is actively reconciling.

---

## 9. Metrics & Alerting

Tuck exposes Prometheus metrics at `GET /metrics` (no auth required).

Key metrics:

| Metric | Alert threshold |
|---|---|
| `tuck_barrier_sealed` == 1 | Page immediately |
| `tuck_auth_failures_total` > 100/min | Investigate brute-force |
| `tuck_requests_5xx_total` rising | Investigate errors |
| `tuck_gc_removed_tokens_total` | Monitor for token accumulation |

Example Prometheus alert rule:

```yaml
- alert: TuckSealed
  expr: tuck_barrier_sealed == 1
  for: 1m
  severity: critical
  annotations:
    summary: "Tuck is sealed — all secret reads returning 503"
```

---

## 10. Incident Response

### Secrets potentially exposed

1. **Seal the server immediately**: `tuckcli seal`
2. **Rotate the root key** (new DEK envelope): `tuckcli rotate`
3. **Revoke all tokens**: list and revoke, issue fresh tokens
4. **Rotate secrets** whose values may have been exposed
5. **Review audit log** to determine which secrets were accessed and by whom:
   ```bash
   cat /var/log/tuck/audit.log | jq 'select(.path | startswith("secret/"))'
   ```
6. **Restore from last known-good snapshot** if data integrity is in doubt

### Token stolen

1. Revoke the token immediately: `tuckcli token revoke <id>`
2. Review audit log for the token fingerprint (SHA-256[:6])
3. Rotate any secrets the token had read access to

### Operator pod crash-looping

1. Check logs: `kubectl -n tuck-system logs <pod>`
2. Verify Tuck server is reachable and unsealed
3. Verify SA token file exists and TokenReview succeeds
4. Check TuckSecret conditions: `kubectl get ts -A -o yaml | grep -A5 conditions`

---

## 11. Health Checks

| Endpoint | Use |
|---|---|
| `GET /v1/health` | Liveness probe (always 200 if process is up) |
| `GET /v1/sys/ready` | Readiness probe (200 = unsealed, 503 = sealed) |
| `GET /v1/sys/seal-status` | Seal state details |
| `GET /metrics` | Prometheus scrape |

### Kubernetes probes

```yaml
livenessProbe:
  httpGet: { path: /v1/health, port: 8200 }
  initialDelaySeconds: 5
readinessProbe:
  httpGet: { path: /v1/sys/ready, port: 8200 }
  initialDelaySeconds: 5
```
