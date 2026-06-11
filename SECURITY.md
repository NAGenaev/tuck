# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| main (v0.9+) | ✅ Active |
| < v0.9 | ❌ No patches |

## Reporting a Vulnerability

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Report security issues via email to **genaevlive@gmail.com** with the subject line:
`[TUCK SECURITY] <brief description>`

Include:
- Description of the vulnerability
- Steps to reproduce (proof of concept if possible)
- Affected versions
- Potential impact assessment
- Your name / handle for acknowledgement (optional)

You will receive an acknowledgement within **48 hours** and a detailed response
within **7 days** indicating next steps.

## Coordinated Disclosure

We follow a **90-day coordinated disclosure** policy:

1. You report the vulnerability privately.
2. We confirm, triage, and develop a fix (target: within 30 days for critical).
3. We coordinate a release date with you.
4. We publish the fix and a CVE (if applicable).
5. You may publicly disclose after the fix is released or after 90 days, whichever comes first.

If you believe the vulnerability is being exploited in the wild or poses immediate
critical risk, please indicate this in your report so we can expedite the timeline.

## Security Scope

**In scope:**
- Authentication bypass (token forgery, policy bypass)
- Cryptographic weaknesses (barrier encryption, Shamir implementation)
- Memory disclosure of key material (root key, DEK)
- Audit log tampering
- Command injection via API inputs
- TLS misconfiguration

**Out of scope:**
- Attacks requiring physical access to the host node (OS-level concern)
- K8s etcd encryption (cluster operator responsibility)
- Vulnerabilities in Go runtime or standard library (report upstream)
- Social engineering

## Known Limitations (by design)

See [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) for a full threat analysis.
Notable limitations at current version:

- Secret **key names** (paths) are stored unencrypted in the bbolt file.
  A bbolt dump reveals the namespace structure but not values.
- The Go GC may copy key material before `clear()` runs.
  `mlockall` is not yet implemented (planned: SEC-9).
- Dev seal stores the root key in plaintext. Never use in production.

## Acknowledgements

We thank the following researchers for responsible disclosure: *(none yet)*
