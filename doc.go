// Package stackure is the Go SDK for the Stackure authentication API.
//
// Stackure provides passwordless B2B authentication. This SDK wraps the
// public API behind four free functions and a middleware.
//
// # Quickstart
//
// Protect an HTTP route:
//
//	http.Handle("/admin", stackure.Auth("my-app-id", "admin")(handler))
//
// Access the authenticated user inside the handler:
//
//	user := stackure.UserFromContext(r.Context())
//
// Manual verification without middleware:
//
//	result := stackure.Verify("my-app-id", r)
//	if result.Authenticated {
//	    // use result.User
//	}
//
// Send a magic-link email:
//
//	_, err := stackure.SendMagicLink("user@example.com", "my-app-id")
//
// Log the user out:
//
//	err := stackure.Logout(r.Cookies())
//
// # Content negotiation
//
// The Auth middleware inspects the Accept header. Browser requests (Accept:
// text/html) redirect to the sign-in URL on 401. API requests (Accept:
// application/json) receive a JSON error body.
//
// # Configuration
//
// The SDK has no configuration API. Point it at a non-production environment
// by setting the STACKURE_BASE_URL environment variable before the first call:
//
//	os.Setenv("STACKURE_BASE_URL", "https://stage.stackure.com")
//
// Retry-on-5xx (two attempts with exponential backoff) and the 10-second
// request timeout are hard-coded. Timeouts are never retried.
//
// # Errors
//
// All errors returned from this package are *StackureError. Inspect the
// Code field to branch on category:
//
//	var se *stackure.StackureError
//	if errors.As(err, &se) {
//	    // se.Code is one of: "validation", "auth", "forbidden", "timeout", "network"
//	}
//
// # API stability
//
// Pre-v1.0 releases are experimental; breaking changes may occur between
// minor versions. Starting at v1.0.0, this package follows strict Semantic
// Versioning.
package stackure
