// Package stackure is the Go SDK for the Stackure authentication API.
//
// Stackure provides passwordless B2B authentication. This SDK wraps the
// public API and offers an HTTP middleware, a non-throwing verifier, and
// direct client methods for the authentication flow.
//
// # Quickstart
//
// Protect an HTTP route:
//
//	http.Handle("/admin", stackure.Auth("my-app-id", stackure.Roles("admin"))(handler))
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
// # Content negotiation
//
// The Auth middleware inspects the Accept header. Browser requests (Accept:
// text/html) redirect to the sign-in URL on 401. API requests (Accept:
// application/json) receive a JSON error body. This lets the same middleware
// serve both front-end and back-end routes.
//
// # Retry and timeouts
//
// The client retries 5xx responses with exponential backoff and never retries
// timeouts. These behaviors are not configurable: the SDK ships with sensible
// defaults so callers do not have to tune them.
//
// # Typed errors
//
// All errors from this package implement the error interface and can be
// distinguished with errors.As:
//
//	var authErr *stackure.AuthenticationError
//	if errors.As(err, &authErr) { /* ... */ }
//
// Error types: ValidationError, NetworkError, AuthenticationError,
// TimeoutError, ForbiddenError.
//
// # API stability
//
// Pre-v1.0 releases are experimental; breaking changes may occur between
// minor versions. Starting at v1.0.0, this package follows strict Semantic
// Versioning: breaking changes only on major-version bumps.
package stackure
