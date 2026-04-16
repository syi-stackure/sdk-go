# Security Policy

## Reporting a vulnerability

Do **not** file a public GitHub issue for security vulnerabilities.

Report privately via **[GitHub Security Advisories](https://github.com/stackure/stackure-go/security/advisories/new)**. This routes the report directly to the maintainer and keeps the disclosure channel private until a fix is ready.

Please include:

- A description of the issue and its impact
- Steps to reproduce or proof-of-concept
- The SDK version or commit SHA affected
- Your disclosure timeline preferences

You will receive an acknowledgment within 72 hours. Fixes are released as patch versions with a corresponding GitHub Security Advisory once coordinated disclosure is complete.

## Supply chain

Every release is:

- **Signed** with [Sigstore cosign](https://www.sigstore.dev/) (keyless, GitHub OIDC)
- **Attested** with [SLSA v1.0 provenance](https://slsa.dev/spec/v1.0/)

Verify a release archive:

```bash
# Replace <tag> with the release tag (e.g. v0.1.0)
cosign verify-blob \
  --certificate stackure-go-<tag>.tar.gz.pem \
  --signature stackure-go-<tag>.tar.gz.sig \
  --certificate-identity-regexp "^https://github.com/stackure/stackure-go/.github/workflows/release.yml@.*$" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  stackure-go-<tag>.tar.gz
```

SLSA provenance can be verified with the [slsa-verifier](https://github.com/slsa-framework/slsa-verifier) CLI.

## Supported versions

Only the latest minor release receives security updates until v1.0.0. Starting at v1.0.0, the two most recent minor releases are supported.
