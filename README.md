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

## Configure

Set the app secret from the Stackure app page (shown once at registration and on each rotation):

```bash
export STACKURE_APP_SECRET=...
```

It is sent as `X-App-Secret` on every call except `Logout`'s sign-out call, which carries only the session token. The first call that actually reaches Stackure fails with a `validation` error if it is unset. `STACKURE_BASE_URL` optionally overrides the API host (default `https://stackure.com`).

Every call has one 2-second deadline. The SDK retries once after 500 ms on a 5xx response or a connection failure (refused, reset, DNS, TLS), only if more than 500 ms of the deadline remain; timeouts are never retried and surface as a `timeout` error. Redirects are not followed; a 3xx response surfaces as a `network` error.

## Protect a route

```go
import "stackure.com/sdk-go"

const appID = "7f3c1a2e-9b4d-4e6f-8a1b-2c3d4e5f6071" // your app's UUID in Stackure

http.Handle("/admin", stackure.Auth(appID, "can_approve_invoice")(handler))
```

Access the authenticated user in your handler:

```go
user := stackure.UserFromContext(r.Context())
fmt.Println(user.UserEmail, user.AccountID, user.UserPermissions)
```

- API requests get JSON errors
- Browser requests get redirected to sign-in
- The sign-in handoff is automatic: Stackure POSTs a `session_token` (an app-scoped session token valid only for this app) back to your app, the middleware validates it and stores it as a cookie on your domain. Handoff bodies over 4 KB are ignored and passed to your app untouched.

Before testing sign-in: a newly registered app is not usable by anyone, even its creator, until it is shared with the organization or assigned to a team in Stackure.

## Requirements

Sessions are not bound to the browser's user agent or IP. The SDK still
forwards the original `User-Agent` and `X-Forwarded-For` when validating from
your server, but they are informational only.

Every request with a session token is validated against Stackure, so revocation
is immediate. Requests without a well-formed token get the sign-in URL without a
Stackure call.

## MCP

```go
http.Handle("/mcp", stackure.MCP(appID)(mcpHandler)) // mcpHandler is your MCP server's http.Handler
```

AI clients such as Claude, Claude Code, VS Code and Cursor sign users in through Stackure. This one line checks every MCP request against Stackure in real time with the same app secret. There is no extra setup.

The MCP endpoint must be served from the same site as the app's registered URL unless an MCP URL is set for the app in Stackure.

The user and permissions work as with `Auth`. A request that is not signed in gets a `401` that tells the AI client where to sign in, a missing permission gets a `403`, and a check that cannot be completed gets a `503`.

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
http.HandleFunc("/logout", stackure.Logout)
```

```html
<form method="post" action="/logout"><button>Sign out</button></form>
```

Mount `Logout` on the path alone, with no method in the pattern (`"/logout"`, not `"POST /logout"`), so every request to it reaches the SDK. Trigger it with a form or button that POSTs from the app's own page; a link or any other request is sent to Stackure's sign-out page, where the user confirms.

That POST signs the user out of Stackure everywhere with a server-side call, clears the app's cookie and redirects to Stackure. If the call fails, the redirect goes to Stackure's sign-out page, where the user can finish signing out.

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

Open a PR.

## Security

Report vulnerabilities via [GitHub Security Advisories](https://github.com/syi-stackure/sdk-go/security/advisories/new). Releases are signed with [cosign](https://www.sigstore.dev/) and carry [GitHub build-provenance attestations](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds).

## License

MIT
