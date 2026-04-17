# Stackure Go SDK

[![CI](https://github.com/syi-stackure/sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/syi-stackure/sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/syi-stackure/sdk-go.svg)](https://pkg.go.dev/github.com/syi-stackure/sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/syi-stackure/sdk-go)](https://goreportcard.com/report/github.com/syi-stackure/sdk-go)
[![Latest release](https://img.shields.io/github/v/release/syi-stackure/sdk-go?sort=semver)](https://github.com/syi-stackure/sdk-go/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/syi-stackure/sdk-go)](./go.mod)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)

Authentication for your app. One line. Zero dependencies.

## Install

```bash
go get github.com/syi-stackure/sdk-go
```

## Protect a route

```go
import "github.com/syi-stackure/sdk-go"

http.Handle("/admin", stackure.Auth("my-app-id", stackure.Roles("admin"))(handler))
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

## Client functions

```go
resp, err := stackure.SendMagicLink("user@example.com", "my-app-id")
resp, err := stackure.SignIn("my-app-id", "user@example.com")

session, err := stackure.ValidateSession("my-app-id", r.Cookies())
// session.Authenticated, session.User, session.SignInURL

err := stackure.Logout(r.Cookies())
```

## Custom client

```go
client := stackure.NewClient(stackure.Config{
    BaseURL: "https://staging.stackure.com",
    Timeout: 5 * time.Second,
})
```

## Errors

`ValidationError` | `NetworkError` | `AuthenticationError` | `TimeoutError` | `ForbiddenError`

Distinguish with `errors.As`:

```go
var authErr *stackure.AuthenticationError
if errors.As(err, &authErr) { /* ... */ }
```

## Docs

Full API reference lives on [pkg.go.dev](https://pkg.go.dev/github.com/syi-stackure/sdk-go).

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/) — release-please depends on this.

## Security

See [SECURITY.md](./SECURITY.md). Releases are signed with [cosign](https://www.sigstore.dev/) and ship with [SLSA v1.0 provenance](https://slsa.dev/spec/v1.0/).

## License

MIT
