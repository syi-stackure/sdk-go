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

http.Handle("/admin", stackure.Auth("my-app-id", "admin")(handler))
```

Access the authenticated user in your handler:

```go
user := stackure.UserFromContext(r.Context())
fmt.Println(user.UserEmail, user.UserRoles)
```

- API requests get JSON errors
- Browser requests get redirected to sign-in

## Verify manually

```go
result := stackure.Verify("my-app-id", r)

if !result.Authenticated {
    // result.Error.Code, result.Error.Message, result.Error.SignInURL
}

// result.User
```

## Send a magic link

```go
resp, err := stackure.SendMagicLink("user@example.com", "my-app-id")
// resp.Message
```

## Log out

```go
err := stackure.Logout(r.Cookies())
```

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
