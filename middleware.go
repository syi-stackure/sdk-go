package stackure

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

func Verify(r *http.Request) *VerifyResult {
	return check(r, nil)
}

func check(r *http.Request, dev *devSignIn) *VerifyResult {
	session, err := validateToken(sessionToken(r), r, dev)
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

	return &VerifyResult{Authenticated: true, User: session.User}
}

func handoffForm(r *http.Request, local bool) url.Values {
	if r.Method != http.MethodPost {
		return nil
	}
	if o := r.Header.Get("Origin"); o != origin() && (!local || o != "null") {
		return nil
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, maxHandoffBody+1))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), r.Body))
	if len(b) > maxHandoffBody {
		return nil
	}
	v, err := url.ParseQuery(string(b))
	if err != nil {
		return nil
	}
	return v
}

func setCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func adoptToken(w http.ResponseWriter, r *http.Request, form url.Values) bool {
	tok := form.Get(tokenParam)
	if tok == "" || r.Header.Get("Origin") != origin() {
		return false
	}
	if s, err := validateToken(tok, r, nil); err != nil || !s.Authenticated {
		return false
	}

	setCookie(w, r, sessionCookie, tok, sessionMaxAge)
	http.Redirect(w, r, safePath(r.URL.RequestURI()), http.StatusSeeOther)
	return true
}

func newVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func newDevSignIn(r *http.Request, verifier string) *devSignIn {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	sum := sha256.Sum256([]byte(verifier))
	return &devSignIn{
		url:       scheme + "://" + r.Host + safePath(r.URL.RequestURI()),
		challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}
}

func stashCode(w http.ResponseWriter, r *http.Request, form url.Values) bool {
	code := form.Get(codeParam)
	if !codeRegex.MatchString(code) {
		return false
	}

	setCookie(w, r, codeCookie, code, codeMaxAge)
	http.Redirect(w, r, safePath(r.URL.RequestURI()), http.StatusSeeOther)
	return true
}

func redeemCode(w http.ResponseWriter, r *http.Request) bool {
	code := cookie(r, codeCookie)
	if code == "" {
		return false
	}
	setCookie(w, r, codeCookie, "", -1)
	setCookie(w, r, verifierCookie, "", -1)

	tok, err := exchangeCode(code, cookie(r, verifierCookie), r)
	if err != nil {
		log.Printf("stackure: localhost sign-in failed: %v", err)
		deny(w, http.StatusUnauthorized, "localhost sign-in failed, reload to try again")
		return true
	}

	setCookie(w, r, sessionCookie, tok, sessionMaxAge)
	http.Redirect(w, r, safePath(r.URL.RequestURI()), http.StatusSeeOther)
	return true
}

func safePath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return "/"
	}
	return p
}

func Auth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			local := isLoopback(r)
			form := handoffForm(r, local)
			if adoptToken(w, r, form) {
				return
			}
			if local && (stashCode(w, r, form) || redeemCode(w, r)) {
				return
			}

			var verifier string
			var dev *devSignIn
			if local {
				verifier = newVerifier()
				dev = newDevSignIn(r, verifier)
			}
			result := check(r, dev)

			if !result.Authenticated && result.Error != nil {
				if result.Error.Code == 401 {
					accept := r.Header.Get("Accept")
					if strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json") && result.Error.SignInURL != "" {
						if local {
							setCookie(w, r, verifierCookie, verifier, verifierMaxAge)
						}
						http.Redirect(w, r, result.Error.SignInURL, http.StatusFound)
						return
					}
				}

				label := "Error"
				if result.Error.Code == 401 {
					label = "Unauthorized"
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

func deny(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"error":"`+msg+`"}`)
}

func MCP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := validateMCP(r)
			if err != nil {
				log.Printf("stackure: verification error: %v", err)
				deny(w, http.StatusServiceUnavailable, "unavailable")
				return
			}

			if !session.Authenticated || session.User == nil {
				challenge := session.WWWAuthenticate
				if challenge == "" {
					challenge = "Bearer"
				}
				w.Header().Set("WWW-Authenticate", challenge)
				deny(w, http.StatusUnauthorized, "unauthorized")
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
	setCookie(w, r, sessionCookie, "", -1)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
