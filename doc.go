// Package stackure is the Go SDK for the Stackure authentication API.
//
// Stackure provides passwordless B2B authentication. This SDK wraps the
// public API behind five free functions and the Auth middleware.
//
// # Quickstart
//
// Protect an HTTP route:
//
//	http.Handle("/admin", stackure.Auth(appID, "can_approve_invoice")(handler))
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
//	http.HandleFunc("/logout", stackure.Logout)
//
// Mount Logout on the path alone, with no method in the pattern, so every
// request to it reaches the SDK. Trigger it with a form or button that POSTs
// from the app's own page; a link or any other request is sent to Stackure's
// sign-out page, where the user confirms.
//
// That POST signs the user out of Stackure everywhere with a server-side call,
// clears the app's cookie and redirects to Stackure. If the call fails, the
// redirect goes to Stackure's sign-out page, where the user can finish signing
// out.
//
// # Sign-in handoff
//
// Stackure's session cookie is scoped to the Stackure host and is never
// visible to your app. After a successful magic-link sign-in, Stackure hands
// the browser back to the app's registered URL with a session_token POST form
// field: an app-scoped session token valid only for this app, accepted by the
// validate endpoint for your app ID and never by Stackure itself.
//
// The Auth middleware consumes it automatically: it validates the token, stores
// it in a cookie on your own domain and redirects to the same URL as a GET.
// Invalid tokens are ignored, and handoff bodies over 4 KB are ignored (treated
// as no token) and passed to your app untouched.
//
// A newly registered app is not usable by anyone, even its creator, until it is
// shared with the organization or assigned to a team in Stackure. Do that
// before testing sign-in.
//
// # Session binding
//
// Sessions are not bound to the browser's user agent or IP address. The SDK
// still forwards the original User-Agent and X-Forwarded-For on every
// validation call, but they are informational only; validation does not
// depend on them.
//
// Every request with a session token is validated against Stackure, so revoking
// a session takes effect immediately. Requests without a well-formed token get
// the sign-in URL without a Stackure call.
//
// # Content negotiation
//
// The Auth middleware inspects the Accept header. Browser requests (Accept:
// text/html) redirect to the sign-in URL on 401. API requests (Accept:
// application/json) receive a JSON error body.
//
// # Configuration
//
// STACKURE_APP_SECRET must be set to the app secret shown when the app was
// registered (or last rotated) in Stackure. It is sent as the X-App-Secret
// header on every call except Logout's sign-out call, which carries only the
// session token; the first call that actually reaches Stackure fails with a
// "validation" error when it is unset. STACKURE_BASE_URL overrides the API host
// (default https://stackure.com).
//
// Every SDK call has one 2-second deadline covering connect, headers, body and
// the single retry. The SDK retries once after 500 ms on a 5xx response or a
// connection failure (refused, reset, DNS, TLS), and only if more than 500 ms
// of the deadline remain. Timeouts are never retried and surface as a
// "timeout" error, even when they happen while reading the body. Redirects are
// not followed; a 3xx response surfaces as a "network" error.
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
