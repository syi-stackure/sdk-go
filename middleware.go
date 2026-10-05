package stackure

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
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

func Verify(r *http.Request, perms ...string) *VerifyResult {
	session, err := ValidateSession(r)
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
	if r.Method != http.MethodPost {
		return ""
	}
	if r.Header.Get("Origin") != origin() {
		return ""
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, maxHandoffBody+1))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), r.Body))
	if len(b) > maxHandoffBody {
		return ""
	}
	v, err := url.ParseQuery(string(b))
	if err != nil {
		return ""
	}
	return v.Get(tokenParam)
}

func adoptToken(w http.ResponseWriter, r *http.Request) bool {
	tok := handoffToken(r)
	if tok == "" {
		return false
	}
	if s, err := validateToken(tok, r); err != nil || !s.Authenticated {
		return false
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		MaxAge:   604800,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, safePath(r.URL.RequestURI()), http.StatusSeeOther)
	return true
}

func safePath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return "/"
	}
	return p
}

func Auth(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adoptToken(w, r) {
				return
			}

			result := Verify(r, perms...)

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

func mcpDeny(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"error":"`+msg+`"}`)
}

func MCP(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := validateMCP(r)
			if err != nil {
				log.Printf("stackure: verification error: %v", err)
				mcpDeny(w, http.StatusServiceUnavailable, "unavailable")
				return
			}

			if !session.Authenticated || session.User == nil {
				challenge := session.WWWAuthenticate
				if challenge == "" {
					challenge = "Bearer"
				}
				w.Header().Set("WWW-Authenticate", challenge)
				mcpDeny(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if len(perms) > 0 && !hasAnyPerm(session.User.UserPermissions, perms) {
				mcpDeny(w, http.StatusForbidden, "forbidden")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, session.User)))
		})
	}
}

func sameOriginPost(r *http.Request) bool {
	h := r.Header
	site := h.Values("Sec-Fetch-Site")
	if r.Method != http.MethodPost || len(site) > 1 || len(h.Values("Origin")) > 1 || len(h.Values("Host")) > 1 {
		return false
	}
	if len(site) == 1 {
		return site[0] == "same-origin"
	}
	u, err := url.Parse(h.Get("Origin"))
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) && (u.Scheme == "https" || !isHTTPS(r))
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if !sameOriginPost(r) {
		http.Redirect(w, r, baseURL()+"/signout", http.StatusSeeOther)
		return
	}
	dest := baseURL() + "/"
	if tok := sessionToken(r); uuidRegex.MatchString(tok) {
		st, _, err := request(context.WithoutCancel(r.Context()), http.MethodPost, "/api/public/auth/sign-out", callOpts{bearer: tok, noBody: true, ua: r.UserAgent(), ip: clientIP(r)})
		if err != nil || st/100 != 2 {
			dest = baseURL() + "/signout"
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
