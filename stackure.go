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

var httpClient = &http.Client{Timeout: requestTimeout}

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

type User struct {
	UserID          string   `json:"user_id"`
	AccountID       string   `json:"account_id"`
	UserEmail       string   `json:"user_email"`
	UserFirstName   string   `json:"user_first_name"`
	UserLastName    string   `json:"user_last_name"`
	UserPermissions []string `json:"user_permissions"`
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

type callOpts struct {
	body    any
	query   url.Values
	cookies []*http.Cookie
	ua, ip  string
}

func request(ctx context.Context, method, path string, o callOpts) (int, []byte, error) {
	secret, err := appSecret()
	if err != nil {
		return 0, nil, err
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
		req.Header.Set("X-App-Secret", secret)
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
			b, err = io.ReadAll(resp.Body)
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

func SendMagicLink(email string, appID ...string) (*MagicLinkResponse, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}

	body := map[string]string{"user_email": email}
	if len(appID) > 0 && appID[0] != "" {
		if err := validateUUID(appID[0], "App ID"); err != nil {
			return nil, err
		}
		body["app_id"] = appID[0]
	}

	st, b, err := request(context.Background(), http.MethodPost, "/api/public/auth/magic-link/send", callOpts{body: body})
	if err != nil {
		return nil, err
	}

	out := &MagicLinkResponse{}
	if err := handleResponse(st, b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateSession(appID string, r *http.Request) (*Session, error) {
	return validateToken(appID, sessionToken(r), r)
}

func validateToken(appID, tok string, r *http.Request) (*Session, error) {
	if err := validateUUID(appID, "App ID"); err != nil {
		return nil, err
	}

	if !uuidRegex.MatchString(tok) {
		return &Session{SignInURL: baseURL() + "/sign-in/magic-link?app_id=" + appID}, nil
	}

	o := callOpts{
		query:   url.Values{"app_id": {appID}},
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
