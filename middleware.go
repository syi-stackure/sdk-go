package stackure

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type contextKey struct{}

var userContextKey = contextKey{}

func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func Verify(appID string, r *http.Request, perms ...string) *VerifyResult {
	session, err := ValidateSession(appID, r)
	if err != nil {
		log.Printf("stackure: verification error: %v", err)
		return &VerifyResult{Error: &VerifyError{Code: 500, Message: "Authentication verification failed"}}
	}

	if !session.Authenticated || session.User == nil {
		return &VerifyResult{Error: &VerifyError{
			Code:      401,
			Message:   "Valid authentication required",
			SignInURL: session.SignInURL,
		}}
	}

	if len(perms) > 0 && !hasAnyPerm(session.User.UserPermissions, perms) {
		return &VerifyResult{User: session.User, Error: &VerifyError{
			Code:    403,
			Message: "Requires one of: " + strings.Join(perms, ", "),
		}}
	}

	return &VerifyResult{Authenticated: true, User: session.User}
}

func hasAnyPerm(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if w == h {
				return true
			}
		}
	}
	return false
}

func handoffToken(r *http.Request) string {
	if tok := r.URL.Query().Get(tokenParam); tok != "" {
		return tok
	}
	if r.Method != http.MethodPost {
		return ""
	}
	if _, err := r.Cookie(sessionCookie); err == nil {
		return ""
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return ""
	}
	return r.PostFormValue(tokenParam)
}

func adoptToken(w http.ResponseWriter, r *http.Request) bool {
	tok := handoffToken(r)
	if tok == "" {
		return false
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})

	q := r.URL.Query()
	q.Del(tokenParam)
	clean := *r.URL
	clean.RawQuery = q.Encode()
	http.Redirect(w, r, clean.RequestURI(), http.StatusSeeOther)
	return true
}

func Auth(appID string, perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adoptToken(w, r) {
				return
			}

			result := Verify(appID, r, perms...)

			if !result.Authenticated && result.Error != nil {
				if result.Error.Code == 401 {
					accept := r.Header.Get("Accept")
					if strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json") && result.Error.SignInURL != "" {
						http.Redirect(w, r, result.Error.SignInURL, http.StatusFound)
						return
					}
				}

				label := "Error"
				switch result.Error.Code {
				case 401:
					label = "Unauthorized"
				case 403:
					label = "Forbidden"
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(result.Error.Code)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       label,
					"message":     result.Error.Message,
					"sign_in_url": result.Error.SignInURL,
				})
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, result.User)))
		})
	}
}

func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, baseURL()+"/signout", http.StatusSeeOther)
}
