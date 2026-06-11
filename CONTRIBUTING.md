# Contributing to Tuck

Thank you for your interest in contributing! Tuck is a community-driven project and welcomes all contributions — bug reports, feature requests, documentation improvements, and code changes.

## Getting Started

### Prerequisites

- Go 1.23+
- Docker Desktop (for integration tests with minikube)
- `golangci-lint` for linting: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Local Development

```bash
git clone https://github.com/NAGenaev/tuck.git
cd tuck

# Build everything
go build ./...

# Run tests
go test ./...

# Build server binary
go build -o bin/tuck ./cmd/tuck

# Run dev server (in-memory, auto-unseal, no TLS)
./bin/tuck --seal-type=dev --addr=127.0.0.1:8200
```

### Project Structure

```
cmd/tuck/           # Server binary
cmd/tuck-operator/  # Kubernetes operator binary
cmd/tuckcli/        # CLI client
internal/
  api/              # HTTP handlers and routing
  audit/            # Tamper-evident audit log
  barrier/          # AES-256-GCM cryptographic barrier
  core/             # Business logic (secrets, tokens, policies)
  k8s/              # Kubernetes TokenReview auth
  metrics/          # Prometheus metrics
  operator/         # CRD controller and leader election
  physical/         # Storage backends (bbolt, in-memory)
  policy/           # Path-glob ACL
  ratelimit/        # Per-IP token bucket
  seal/             # Seal backends (dev, shamir, transit)
  shamir/           # GF(256) secret sharing
  tlsutil/          # TLS helpers
  token/            # Token store
  ui/               # Embedded web dashboard
deploy/             # Kubernetes manifests and CRD
docs/               # Architecture, threat model, runbook
test/load/          # k6 load test scripts
```

## How to Contribute

### Bug Reports

Use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml). Please include:
- Tuck version (`tuck --version`)
- Steps to reproduce
- Expected vs. actual behaviour
- Relevant logs (redact any tokens or secret values)

### Feature Requests

Use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml).
Check the [ROADMAP](docs/ROADMAP.md) first — your feature may already be planned.

### Pull Requests

1. **Fork** the repository and create a feature branch: `git checkout -b feat/my-change`
2. **Write tests** for new behaviour. Aim for coverage ≥ 70% on `crypto`/`auth` packages.
3. **Run the full suite** before opening a PR:
   ```bash
   go test ./...
   golangci-lint run ./...
   go build ./...
   ```
4. **Keep PRs focused.** One feature or fix per PR makes review faster.
5. **Reference the issue** in your PR description: `Closes #123`.
6. **Sign your commits** if your org requires it; otherwise standard commits are fine.

### Commit Style

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(barrier): add Rekey() for root-key rotation
fix(api): binary-safe KV response for non-UTF8 values
docs(threat-model): add bbolt exfiltration scenario
test(operator): add UpdateStatus mock to controller test
```

### Code Style

- `gofmt` and `goimports` (enforced by CI)
- No unused exports in internal packages
- No `//nolint` without an explanation comment
- No secrets in test fixtures — use `t.TempDir()` for key files

## Security Issues

Do **not** open public issues for security vulnerabilities.
See [SECURITY.md](SECURITY.md) for the coordinated disclosure process.

## Licence

By contributing, you agree your contributions are licensed under the [Apache 2.0 Licence](LICENSE).
