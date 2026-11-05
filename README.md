# Stackure Go SDK

[![Check build](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml/badge.svg)](https://github.com/syi-stackure/sdk-go/actions/workflows/check-build.yml)
[![Go Reference](https://pkg.go.dev/badge/stackure.com/sdk-go.svg)](https://pkg.go.dev/stackure.com/sdk-go)
[![Latest release](https://img.shields.io/github/v/release/syi-stackure/sdk-go?sort=semver)](https://github.com/syi-stackure/sdk-go/releases)

[Stackure](https://stackure.com) is sign-in and app access for your people and your partners. This SDK adds it to a Go app as `net/http` middleware, with zero dependencies.

## Install

```bash
go get stackure.com/sdk-go
```

Needs Go 1.27 or later.

## Quickstart

Set `STACKURE_APP_ID` and `STACKURE_APP_SECRET` from the app's page in Stackure and `STACKURE_BASE_URL` to your region's address, `https://eu.stackure.com` or `https://us.stackure.com`, then serve your app's registered URL behind `Auth`:

```go
package main

import (
	"fmt"
	"net/http"

	"stackure.com/sdk-go"
)

func main() {
	http.Handle("/", stackure.Auth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Signed in as", stackure.UserFromContext(r.Context()).UserEmail)
	})))
	http.ListenAndServe(":8080", nil)
}
```

## Docs

- Guide and reference: [stackure.com/docs/go](https://stackure.com/docs/go)
- For AI agents: [stackure.com/llms.txt](https://stackure.com/llms.txt)

## Model

Modeled in Mappstack as the Stackure project. Every change starts in the model, then the code follows it.

## License

MIT
