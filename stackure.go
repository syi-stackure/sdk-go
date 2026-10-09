package stackure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://stackure.com"
	sessionCookie  = "session"
	tokenParam     = "session_token"
	maxHandoffBody = 4 << 10
	requestTimeout = 2 * time.Second
	maxRetries     = 1
	retryDelay     = 500 * time.Millisecond
)

var httpClient = &http.Client{
	Timeout:       requestTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func origin() string {
	u, err := url.Parse(baseURL())
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func baseURL() string {
	if v := os.Getenv("STACKURE_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

func appSecret() (string, error) {
	if v := os.Getenv("STACKURE_APP_SECRET"); v != "" {
		return v, nil
	}
	return "", newErr("validation", 0, "STACKURE_APP_SECRET is not set")
}

func appID() (string, error) {
	v := os.Getenv("STACKURE_APP_ID")
	if v == "" {
		return "", newErr("validation", 0, "STACKURE_APP_ID is not set")
	}
	if !uuidRegex.MatchString(v) {
		return "", newErr("validation", 0, "invalid STACKURE_APP_ID format (must be a valid UUID)")
	}
	return v, nil
}

type User struct {
	UserID        string `json:"user_id"`
	AccountID     string `json:"account_id"`
	UserEmail     string `json:"user_email"`
	UserFirstName string `json:"user_first_name"`
	UserLastName  string `json:"user_last_name"`
}

type MagicLinkResponse struct {
	Message string `json:"message"`
}

type VerifyError struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	SignInURL string `json:"sign_in_url,omitempty"`
}

type VerifyResult struct {
	Authenticated bool
	User          *User
	Error         *VerifyError
}

type Session struct {
	Authenticated bool   `json:"authenticated"`
	User          *User  `json:"user,omitempty"`
	SignInURL     string `json:"sign_in_url,omitempty"`
}

type mcpSession struct {
	Session
	WWWAuthenticate string `json:"www_authenticate"`
}

type callOpts struct {
	body    any
	query   url.Values
	cookies []*http.Cookie
	ua, ip  string
	bearer  string
	token   string
	noBody  bool
}

func request(ctx context.Context, method, path string, o callOpts) (int, []byte, error) {
	var secret string
	var err error
	if o.bearer == "" {
		if secret, err = appSecret(); err != nil {
			return 0, nil, err
		}
	}
	fullURL := baseURL() + path
	if len(o.query) > 0 {
		fullURL += "?" + o.query.Encode()
	}
	var br []byte
	if o.body != nil {
		if br, err = json.Marshal(o.body); err != nil {
			return 0, nil, newErr("network", 0, fmt.Sprintf("failed to marshal request body: %v", err))
		}
	}
	deadline := time.Now().Add(requestTimeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	fail := func(err error) (int, []byte, error) {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded || (errors.As(err, &netErr) && netErr.Timeout()) {
			return 0, nil, newErr("timeout", 0, fmt.Sprintf("request timed out after %s", requestTimeout))
		}
		return 0, nil, newErr("network", 0, fmt.Sprintf("network request failed: %v", err))
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(br))
		if err != nil {
			return 0, nil, newErr("network", 0, fmt.Sprintf("failed to create request: %v", err))
		}
		if o.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if o.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+o.bearer)
		} else {
			req.Header.Set("X-App-Secret", secret)
		}
		if o.token != "" {
			req.Header.Set("Authorization", "Bearer "+o.token)
		}
		if o.ua != "" {
			req.Header.Set("User-Agent", o.ua)
		}
		if o.ip != "" {
			req.Header.Set("X-Forwarded-For", o.ip)
		}
		for _, c := range o.cookies {
			req.AddCookie(c)
		}
		resp, err := httpClient.Do(req)
		var b []byte
		if err == nil {
			if !o.noBody {
				b, err = io.ReadAll(resp.Body)
			}
			resp.Body.Close()
			if err == nil && resp.StatusCode < 500 {
				return resp.StatusCode, b, nil
			}
		}
		if ctx.Err() != nil || attempt == maxRetries || time.Until(deadline) <= retryDelay {
			if err != nil {
				return fail(err)
			}
			return resp.StatusCode, b, nil
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(retryDelay):
		}
	}
}

func handleResponse(status int, b []byte, out any) error {
	if status < 200 || status >= 300 {
		txt := string(b)
		if txt == "" {
			txt = "unknown error"
		}
		switch status {
		case 401:
			return newErr("auth", 401, txt)
		case 403:
			return newErr("forbidden", 403, txt)
		}
		return newErr("network", status, fmt.Sprintf("api error (%d): %s", status, txt))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return newErr("network", status, "invalid JSON response from server")
	}
	return nil
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

func bearerToken(r *http.Request) string {
	scheme, tok, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if tok = strings.TrimSpace(tok); strings.EqualFold(scheme, "Bearer") && uuidRegex.MatchString(tok) {
		return tok
	}
	return ""
}

func SendMagicLink(email string) (*MagicLinkResponse, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	id, err := appID()
	if err != nil {
		return nil, err
	}

	st, b, err := request(context.Background(), http.MethodPost, "/api/public/auth/magic-link/send", callOpts{body: map[string]string{"user_email": email, "app_id": id}})
	if err != nil {
		return nil, err
	}

	out := &MagicLinkResponse{}
	if err := handleResponse(st, b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateSession(r *http.Request) (*Session, error) {
	return validateToken(sessionToken(r), r)
}

func validateToken(tok string, r *http.Request) (*Session, error) {
	id, err := appID()
	if err != nil {
		return nil, err
	}

	if !uuidRegex.MatchString(tok) {
		return &Session{SignInURL: baseURL() + "/sign-in/magic-link?app_id=" + id}, nil
	}

	o := callOpts{
		query:   url.Values{"app_id": {id}},
		ua:      r.UserAgent(),
		ip:      clientIP(r),
		cookies: []*http.Cookie{{Name: sessionCookie, Value: tok}},
	}

	st, b, err := request(r.Context(), http.MethodGet, "/api/public/auth/session/validate", o)
	if err != nil {
		return nil, err
	}

	out := &Session{}
	if err := handleResponse(st, b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateMCP(r *http.Request) (*mcpSession, error) {
	id, err := appID()
	if err != nil {
		return nil, err
	}

	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}

	path := r.URL.EscapedPath()
	if u, err := url.ParseRequestURI(r.RequestURI); err == nil && strings.HasPrefix(r.RequestURI, "/") {
		path = u.EscapedPath()
	}

	o := callOpts{
		query: url.Values{"app_id": {id}, "mcp": {scheme + "://" + r.Host + path}},
		ua:    r.UserAgent(),
		ip:    clientIP(r),
		token: bearerToken(r),
	}

	st, b, err := request(r.Context(), http.MethodGet, "/api/public/auth/session/validate", o)
	if err != nil {
		return nil, err
	}

	out := &mcpSession{}
	if err := handleResponse(st, b, out); err != nil {
		return nil, err
	}
	return out, nil
}
