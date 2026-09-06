// Package stackure is the Go SDK for the Stackure authentication API.
//
// Stackure provides passwordless B2B authentication. This SDK wraps the
// public API behind four free functions and a middleware.
//
// # Quickstart
//
// Protect an HTTP route:
//
//	http.Handle("/admin", stackure.Auth(appID, "view_any_app")(handler))
//
// appID is the app's UUID as registered in Stackure.
//
// Access the authenticated user inside the handler:
//
//	user := stackure.UserFromContext(r.Context())
//
// Manual verification without middleware:
//
//	result := stackure.Verify(appID, r)
//	if result.Authenticated {
//	    // use result.User
//	}
//
// Send a magic-link email:
//
//	_, err := stackure.SendMagicLink("user@example.com", appID)
//
// Log the user out:
//
//	stackure.Logout(w, r)
//
// # Sign-in handoff
//
// Stackure's session cookie is scoped to the Stackure host and is never
// visible to your app. After a successful magic-link sign-in, Stackure hands
// the browser back to the app's registered URL with a session_token, either as
// a POST form field or as a query parameter.
//
// The Auth middleware consumes both automatically: it stores the token in a
// cookie on your own domain and redirects to the same URL with the parameter
// stripped, so the token does not linger in the address bar.
//
// # Session binding
//
// Stackure binds each session to the browser's user agent and IP address.
// Because the SDK validates from your server rather than the browser, it
// forwards the original User-Agent and X-Forwarded-For on every validation
// call. Your app must therefore see the real client IP: if it sits behind a
// proxy or CDN, ensure that layer sets X-Forwarded-For correctly.
//
// Every request is validated against Stackure, so revoking a session takes
// effect immediately.
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
// Retry-on-5xx (one retry after 500ms) and the 2-second request timeout are
// hard-coded. Timeouts are never retried.
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
// # Releases
//
// Releases are cut from main automatically and versioned v1.YYYYMMDD.N. Each
// one is signed with cosign and carries a GitHub build-provenance attestation.
package stackure
