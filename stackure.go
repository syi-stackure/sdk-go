package stackure

import (
	"bytes"
	"context"
	"encoding/json"
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
	requestTimeout = 2 * time.Second
	maxRetries     = 1
)

var httpClient = &http.Client{Timeout: requestTimeout}

func baseURL() string {
	if v := os.Getenv("STACKURE_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

type User struct {
	UserID          string   `json:"user_id"`
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

func request(ctx context.Context, method, path string, o callOpts) (*http.Response, error) {
	fullURL := baseURL() + path
	if len(o.query) > 0 {
		fullURL += "?" + o.query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(500*(1<<(attempt-1))) * time.Millisecond)
		}

		var br io.Reader
		if o.body != nil {
			b, err := json.Marshal(o.body)
			if err != nil {
				return nil, newErr("network", 0, fmt.Sprintf("failed to marshal request body: %v", err))
			}
			br = bytes.NewReader(b)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, br)
		if err != nil {
			return nil, newErr("network", 0, fmt.Sprintf("failed to create request: %v", err))
		}
		if o.body != nil {
			req.Header.Set("Content-Type", "application/json")
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
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded || isTimeoutError(err) {
				return nil, newErr("timeout", 0, fmt.Sprintf("request timed out after %s", requestTimeout))
			}
			lastErr = newErr("network", 0, fmt.Sprintf("network request failed: %v", err))
			continue
		}
		if resp.StatusCode >= 500 && attempt < maxRetries {
			resp.Body.Close()
			lastErr = newErr("network", resp.StatusCode, fmt.Sprintf("server error (%d)", resp.StatusCode))
			continue
		}
		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, newErr("network", 0, "request failed after retries")
}

func isTimeoutError(err error) bool {
	type timeout interface{ Timeout() bool }
	if t, ok := err.(timeout); ok {
		return t.Timeout()
	}
	return false
}

func handleResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return newErr("network", resp.StatusCode, "failed to read response body")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		txt := string(b)
		if txt == "" {
			txt = "unknown error"
		}
		switch resp.StatusCode {
		case 401:
			return newErr("auth", 401, txt)
		case 403:
			return newErr("forbidden", 403, txt)
		}
		return newErr("network", resp.StatusCode, fmt.Sprintf("api error (%d): %s", resp.StatusCode, txt))
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return newErr("network", resp.StatusCode, "invalid JSON response from server")
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
	if t := r.URL.Query().Get(tokenParam); t != "" {
		return t
	}
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

	resp, err := request(context.Background(), http.MethodPost, "/api/public/auth/magic-link/send", callOpts{body: body})
	if err != nil {
		return nil, err
	}

	out := &MagicLinkResponse{}
	if err := handleResponse(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateSession(appID string, r *http.Request) (*Session, error) {
	if err := validateUUID(appID, "App ID"); err != nil {
		return nil, err
	}

	o := callOpts{
		query: url.Values{"app_id": {appID}},
		ua:    r.UserAgent(),
		ip:    clientIP(r),
	}
	if tok := sessionToken(r); tok != "" {
		o.cookies = []*http.Cookie{{Name: sessionCookie, Value: tok}}
	}

	resp, err := request(r.Context(), http.MethodGet, "/api/public/auth/session/validate", o)
	if err != nil {
		return nil, err
	}

	out := &Session{}
	if err := handleResponse(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}
