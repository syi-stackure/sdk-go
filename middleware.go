package stackure

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey struct{}

// userContextKey is the key used to store the authenticated User in the request context.
var userContextKey = contextKey{}

// UserFromContext extracts the authenticated User from the request context.
// Returns nil if no user is present.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

// ---------- Verify ----------

// VerifyOption configures the behavior of the Verify function.
type VerifyOption func(*verifyConfig)

type verifyConfig struct {
	roles  []string
	client *Client
}

// VerifyWithRoles specifies required roles for verification.
// The user must have at least one of the listed roles.
func VerifyWithRoles(roles ...string) VerifyOption {
	return func(cfg *verifyConfig) {
		cfg.roles = roles
	}
}

// VerifyWithClient specifies a custom Client for verification.
func VerifyWithClient(client *Client) VerifyOption {
	return func(cfg *verifyConfig) {
		cfg.client = client
	}
}

// Verify performs authentication verification for an incoming HTTP request.
// It returns a VerifyResult without returning an error, allowing callers
// to decide how to handle unauthenticated or unauthorized requests.
func Verify(appID string, r *http.Request, opts ...VerifyOption) *VerifyResult {
	cfg := &verifyConfig{client: defaultClient}
	for _, opt := range opts {
		opt(cfg)
	}

	session, err := cfg.client.ValidateSession(appID, r.Cookies())
	if err != nil {
		log.Printf("Stackure verification error: %v", err)
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

	if len(cfg.roles) > 0 {
		userRoles := session.User.UserRoles
		hasRole := false
		for _, required := range cfg.roles {
			for _, userRole := range userRoles {
				if required == userRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}
		if !hasRole {
			return &VerifyResult{
				Authenticated: false,
				User:          session.User,
				Error: &VerifyError{
					Code:    403,
					Message: "Requires one of: " + strings.Join(cfg.roles, ", "),
				},
			}
		}
	}

	return &VerifyResult{
		Authenticated: true,
		User:          session.User,
	}
}

// ---------- Auth Middleware ----------

// AuthOption configures the behavior of the Auth middleware.
type AuthOption func(*authConfig)

type authConfig struct {
	roles  []string
	client *Client
}

// Roles specifies required roles for the Auth middleware.
// The user must have at least one of the listed roles.
func Roles(roles ...string) AuthOption {
	return func(cfg *authConfig) {
		cfg.roles = roles
	}
}

// WithClient specifies a custom Client for the Auth middleware.
func WithClient(client *Client) AuthOption {
	return func(cfg *authConfig) {
		cfg.client = client
	}
}

// Auth returns an HTTP middleware that enforces authentication.
//
// On success, the authenticated User is stored in the request context and
// can be retrieved with UserFromContext.
//
// On authentication failure (401): if the request accepts text/html, the
// middleware redirects to the sign-in URL. Otherwise it returns a JSON error.
//
// On authorization failure (403): returns a JSON forbidden error.
//
// Example:
//
//	http.Handle("/admin", stackure.Auth("my-app-id", stackure.Roles("admin"))(handler))
func Auth(appID string, opts ...AuthOption) func(http.Handler) http.Handler {
	cfg := &authConfig{client: defaultClient}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Build verify options from auth config.
			var verifyOpts []VerifyOption
			if len(cfg.roles) > 0 {
				verifyOpts = append(verifyOpts, VerifyWithRoles(cfg.roles...))
			}
			if cfg.client != nil {
				verifyOpts = append(verifyOpts, VerifyWithClient(cfg.client))
			}

			result := Verify(appID, r, verifyOpts...)

			if !result.Authenticated && result.Error != nil {
				if result.Error.Code == 401 {
					acceptHeader := r.Header.Get("Accept")
					acceptsHTML := strings.Contains(acceptHeader, "text/html")
					acceptsJSON := strings.Contains(acceptHeader, "application/json")

					if acceptsHTML && !acceptsJSON && result.Error.SignInURL != "" {
						http.Redirect(w, r, result.Error.SignInURL, http.StatusFound)
						return
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(result.Error.Code)

				errorLabel := "Error"
				switch result.Error.Code {
				case 401:
					errorLabel = "Unauthorized"
				case 403:
					errorLabel = "Forbidden"
				}

				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":       errorLabel,
					"message":     result.Error.Message,
					"sign_in_url": result.Error.SignInURL,
				})
				return
			}

			// Store the authenticated user in request context.
			ctx := context.WithValue(r.Context(), userContextKey, result.User)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
