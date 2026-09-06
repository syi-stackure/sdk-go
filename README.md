# Stackure Go SDK

[![Check build](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml/badge.svg)](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml)
[![Go Reference](https://pkg.go.dev/badge/stackure.com/sdk-go.svg)](https://pkg.go.dev/stackure.com/sdk-go)
[![Go Report Card](https://goreportcard.com/badge/stackure.com/sdk-go)](https://goreportcard.com/report/stackure.com/sdk-go)
[![Latest release](https://img.shields.io/github/v/release/syi-stackure/sdk-go?sort=semver)](https://github.com/syi-stackure/sdk-go/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/syi-stackure/sdk-go)](./go.mod)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)

Passwordless magic-link authentication SDK for Go — drop-in `net/http` middleware, zero dependencies.

Protect a route with one line, or verify sessions and send magic links directly against the [Stackure](https://stackure.com) auth API.

## Install

```bash
go get stackure.com/sdk-go
```

## Protect a route

```go
import "stackure.com/sdk-go"

const appID = "7f3c1a2e-9b4d-4e6f-8a1b-2c3d4e5f6071" // your app's UUID in Stackure

http.Handle("/admin", stackure.Auth(appID, "can_approve_invoice")(handler))
```

Access the authenticated user in your handler:

```go
user := stackure.UserFromContext(r.Context())
fmt.Println(user.UserEmail, user.UserPermissions)
```

- API requests get JSON errors
- Browser requests get redirected to sign-in
- The sign-in handoff is automatic: Stackure hands the browser back with a `session_token`, the middleware stores it as a cookie on your domain and strips it from the URL

## Requirements

Stackure binds sessions to the browser's user agent and IP. The SDK validates
from your server, so it forwards the original `User-Agent` and
`X-Forwarded-For`. Your app must see the real client IP — if it runs behind a
proxy or CDN, make sure that layer sets `X-Forwarded-For`.

Every request is validated against Stackure, so revocation is immediate.

## Verify manually

```go
result := stackure.Verify(appID, r)

if !result.Authenticated {
    // result.Error.Code, result.Error.Message, result.Error.SignInURL
}

// result.User
```

## Send a magic link

```go
resp, err := stackure.SendMagicLink("user@example.com", appID)
// resp.Message
```

## Log out

```go
stackure.Logout(w, r)
```

Clears the app's cookie and redirects to Stackure's sign-out.

## Configuration

Set `STACKURE_BASE_URL` to point at a non-production environment:

```bash
export STACKURE_BASE_URL=https://stage.stackure.com
```

## Errors

All errors are `*stackure.StackureError`. Switch on `.Code`:

```go
import "errors"

var se *stackure.StackureError
if errors.As(err, &se) {
    switch se.Code {
    case "validation", "auth", "forbidden", "timeout", "network":
        // ...
    }
}
```

## Contributing

Open a PR. Tag a release when ready: `git tag vX.Y.Z && git push --tags` — the release workflow builds, signs, and publishes.

## Security

Report vulnerabilities via [GitHub Security Advisories](https://github.com/syi-stackure/sdk-go/security/advisories/new). Releases are signed with [cosign](https://www.sigstore.dev/) and carry [GitHub build-provenance attestations](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds).

## License

MIT
