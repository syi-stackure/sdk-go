package stackure

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// contextKey is a private type for context keys to avoid collisions with
// other packages that might also use the request context.
type contextKey struct{}

var userContextKey = contextKey{}

// UserFromContext retrieves the authenticated user stored by the Auth
// middleware. Returns nil if no user is present on the context.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

// Verify checks an incoming request's authentication state without throwing.
// It always returns a VerifyResult; callers decide how to handle a failed
// verification (redirect, 401, render an error, etc.).
//
// The roles argument is optional. When provided, the user must have at least
// one of the listed roles; otherwise the result is a 403.
func Verify(appID string, r *http.Request, roles ...string) *VerifyResult {
	session, err := ValidateSession(appID, r.Cookies())
	if err != nil {
		log.Printf("stackure: verification error: %v", err)
		return &VerifyResult{
			Authenticated: false,
			Error: &VerifyError{
				Code:    500,
				Message: "Authentication verification failed",
			},
		}
	}

	if !session.Authenticated || session.User == nil {
		return &VerifyResult{
			Authenticated: false,
			Error: &VerifyError{
				Code:      401,
				Message:   "Valid authentication required",
				SignInURL: session.SignInURL,
			},
		}
	}

	if len(roles) > 0 && !hasAnyRole(session.User.UserRoles, roles) {
		return &VerifyResult{
			Authenticated: false,
			User:          session.User,
			Error: &VerifyError{
				Code:    403,
				Message: "Requires one of: " + strings.Join(roles, ", "),
			},
		}
	}

	return &VerifyResult{Authenticated: true, User: session.User}
}

// hasAnyRole returns true if any role in want is present in have.
func hasAnyRole(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if w == h {
				return true
			}
		}
	}
	return false
}

// Auth returns an HTTP middleware that enforces authentication.
//
// On success, the authenticated User is stored in the request context and
// can be retrieved with UserFromContext. The wrapped handler runs normally.
//
// On 401 with an HTML-accepting client, the middleware redirects to the
// sign-in URL. Otherwise it returns a JSON error with the appropriate status.
//
// Example:
//
//	http.Handle("/admin", stackure.Auth("my-app-id", "admin")(handler))
func Auth(appID string, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			result := Verify(appID, r, roles...)

			if !result.Authenticated && result.Error != nil {
				if result.Error.Code == 401 {
					accept := r.Header.Get("Accept")
					acceptsHTML := strings.Contains(accept, "text/html")
					acceptsJSON := strings.Contains(accept, "application/json")

					if acceptsHTML && !acceptsJSON && result.Error.SignInURL != "" {
						http.Redirect(w, r, result.Error.SignInURL, http.StatusFound)
						return
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(result.Error.Code)

				label := "Error"
				switch result.Error.Code {
				case 401:
					label = "Unauthorized"
				case 403:
					label = "Forbidden"
				}

				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       label,
					"message":     result.Error.Message,
					"sign_in_url": result.Error.SignInURL,
				})
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, result.User)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
