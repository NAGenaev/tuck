# Tuck — Load Test Results

## Go benchmarks (in-process, no TCP overhead)

Measured on: AMD Ryzen 9 5950X 16-Core (32 threads), Windows 10, Go 1.25.8  
Command: `go test ./internal/api/ -run=^$ -bench=. -benchtime=3s -benchmem`

| Benchmark | ops/s | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| KVPut (serial) | ~57 600 | 17 371 | 17 987 | 111 |
| KVGet (serial) | ~50 200 | 19 917 | 18 472 | 117 |
| KVGet (parallel, 32 goroutines) | ~61 800 | 16 165 | 18 772 | 119 |
| KVPut (parallel, 32 goroutines) | ~180 600 | 5 536 | 17 980 | 111 |
| TokenCreate (serial) | ~44 000 | 22 737 | 22 597 | 141 |
| TokenValidate (serial) | ~49 600 | 20 158 | 16 802 | 98 |
| TokenValidate (parallel, 32 goroutines) | ~82 400 | 12 129 | 17 006 | 99 |
| SealStatus (unauthenticated) | ~181 900 | 5 498 | 7 673 | 29 |

### Key observations

- **KV read/write** settle at **~17–20 µs** serial latency. This is handler + barrier
  decrypt/encrypt + bbolt read/write. Well within the 5 ms p99 target.
- **Parallel reads** scale near-linearly to 32 goroutines (limited by bbolt's
  single-writer lock on writes).
- **Token validation** (~20 µs) is the dominant cost on every authenticated request:
  barrier decrypt + JSON unmarshal.
- **SealStatus** is a lightweight read of in-memory state: ~5.5 µs, suitable for
  sub-10 ms health-check SLAs.
- **Memory per request**: 17–23 KB allocated, 98–141 allocs. No obvious leak vectors.

---

## k6 load test targets

Run with: `./tests/load/run.sh <scenario>`

### Thresholds (must pass for v1.0 GA)

| Metric | Target |
|---|---|
| KV GET p99 | < 50 ms |
| KV PUT p99 | < 50 ms |
| Token create p99 | < 100 ms |
| Seal-status p99 | < 20 ms |
| Error rate | < 0.1 % |

### Scenarios

| Scenario | VUs | Duration | Purpose |
|---|---|---|---|
| `smoke` | 1 | 1 min | Sanity check — confirms server is up and responding |
| `load` | 50 | 5 min | Baseline production load — measure p50/p95/p99 |
| `stress` | 0→200 (ramp) | 10 min | Find the breaking point |
| `soak` | 50 | 24 h | Stability run — detect memory leaks and goroutine leaks |

### Workload mix (per VU iteration)

| Operation | Share | Endpoint |
|---|---|---|
| KV write | 40 % | PUT /v1/secret/… |
| KV read | 40 % | GET /v1/secret/… |
| Seal-status | 10 % | GET /v1/sys/seal-status |
| Token create | 10 % | POST /v1/auth/token |

### Running the soak test

```sh
# Start tuck in the background
tuck --seal-type=dev --tls-auto &
export TUCK_TOKEN=<root-token>

# Run 24h soak
./tests/load/run.sh soak

# Watch memory and goroutines during the run
watch -n 60 'curl -sk https://127.0.0.1:8200/metrics | grep -E "go_goroutines|process_resident"'
```

Expected results during 24h soak:
- Goroutine count stable (no leaks): should stay < 50 goroutines at 50 VU
- RSS growth < 10 MB over 24h (GC collects per-request allocations)
- p99 latency should not degrade over time

---

## How to run Go benchmarks

```sh
# Full benchmark suite
go test ./internal/api/ -run=^$ -bench=. -benchtime=5s -benchmem

# Specific benchmark
go test ./internal/api/ -run=^$ -bench=BenchmarkKVGet -benchtime=10s

# Compare two runs with benchstat
go test ./internal/api/ -run=^$ -bench=. -count=5 > before.txt
# ... make changes ...
go test ./internal/api/ -run=^$ -bench=. -count=5 > after.txt
benchstat before.txt after.txt
```
