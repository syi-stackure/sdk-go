# Stackure Go SDK

[![Check build](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml/badge.svg)](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml)
[![Go Reference](https://pkg.go.dev/badge/stackure.com/sdk-go.svg)](https://pkg.go.dev/stackure.com/sdk-go)
[![Go Report Card](https://goreportcard.com/badge/stackure.com/sdk-go)](https://goreportcard.com/report/stackure.com/sdk-go)
[![Latest release](https://img.shields.io/github/v/release/syi-stackure/sdk-go?sort=semver)](https://github.com/syi-stackure/sdk-go/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/syi-stackure/sdk-go)](./go.mod)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)

Add Stackure sign-in to a Go app with `net/http` middleware. Zero dependencies.

## Install

```bash
go get stackure.com/sdk-go
```

## Example

Set `STACKURE_APP_ID` and `STACKURE_APP_SECRET` to the app ID and secret from Stackure, then serve your app's registered URL behind `Auth`:

```go
package main

import (
	"fmt"
	"net/http"

	"stackure.com/sdk-go"
)

func main() {
	home := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Signed in as", stackure.UserFromContext(r.Context()).UserEmail)
	})
	http.Handle("/", stackure.Auth()(home))
	http.ListenAndServe(":8080", nil)
}
```

## Documentation

Configuration, sign-in handoff, MCP, sign-out, errors and limits: [stackure.com/docs/go](https://stackure.com/docs/go)

## License

MIT
