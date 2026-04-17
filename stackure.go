package stackure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://stackure.com"
	requestTimeout = 10 * time.Second
	maxRetries     = 2
)

// httpClient is the shared HTTP client used by all SDK calls. The timeout is
// not configurable; callers who need a different ceiling can wrap the package
// with their own transport.
var httpClient = &http.Client{Timeout: requestTimeout}

// baseURL resolves from the STACKURE_BASE_URL environment variable at call
// time, falling back to the public Stackure API. This is the only way to
// point the SDK at staging or local environments — no code-level override.
func baseURL() string {
	if v := os.Getenv("STACKURE_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

// User represents an authenticated user returned from Stackure.
type User struct {
	UserID        string   `json:"user_id"`
	UserEmail     string   `json:"user_email"`
	UserFirstName string   `json:"user_first_name"`
	UserLastName  string   `json:"user_last_name"`
	UserRoles     []string `json:"user_roles"`
}

// MagicLinkResponse is returned from SendMagicLink when the request succeeds.
type MagicLinkResponse struct {
	Message string `json:"message"`
}

// VerifyError describes why a verification failed. Present on VerifyResult
// when Authenticated is false.
type VerifyError struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	SignInURL string `json:"sign_in_url,omitempty"`
}

// VerifyResult is the outcome of a Verify call. Exactly one of Error or User
// is populated depending on Authenticated.
type VerifyResult struct {
	Authenticated bool
	User          *User
	Error         *VerifyError
}

// sessionValidationResponse is the wire format returned by the session-validate
// endpoint. Internal; callers interact with VerifyResult instead.
type sessionValidationResponse struct {
	Authenticated bool   `json:"authenticated"`
	User          *User  `json:"user,omitempty"`
	SignInURL     string `json:"sign_in_url,omitempty"`
}

// request performs an HTTP request with retry on 5xx (exponential backoff:
// 500ms, 1s) and no retry on timeouts. All SDK calls go through this helper.
func request(ctx context.Context, method, path string, body any, cookies []*http.Cookie, query url.Values) (*http.Response, error) {
	fullURL := baseURL() + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(500*(1<<(attempt-1))) * time.Millisecond
			time.Sleep(backoff)
		}

		var bodyReader io.Reader
		if body != nil {
			jsonBytes, err := json.Marshal(body)
			if err != nil {
				return nil, newErr("network", 0, fmt.Sprintf("failed to marshal request body: %v", err))
			}
			bodyReader = bytes.NewReader(jsonBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, newErr("network", 0, fmt.Sprintf("failed to create request: %v", err))
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		for _, cookie := range cookies {
			req.AddCookie(cookie)
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

// isTimeoutError checks whether an error implements the standard Timeout()
// bool interface exposed by net/http and net.
func isTimeoutError(err error) bool {
	type timeout interface {
		Timeout() bool
	}
	if t, ok := err.(timeout); ok {
		return t.Timeout()
	}
	return false
}

// handleResponse reads the response body and returns the parsed JSON or a
// typed StackureError for non-2xx responses.
func handleResponse(resp *http.Response) (map[string]any, error) {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newErr("network", resp.StatusCode, "failed to read response body")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorText := string(bodyBytes)
		if errorText == "" {
			errorText = "unknown error"
		}
		if resp.StatusCode == 401 {
			return nil, newErr("auth", 401, errorText)
		}
		if resp.StatusCode == 403 {
			return nil, newErr("forbidden", 403, errorText)
		}
		return nil, newErr("network", resp.StatusCode, fmt.Sprintf("api error (%d): %s", resp.StatusCode, errorText))
	}

	var data map[string]any
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, newErr("network", resp.StatusCode, "invalid JSON response from server")
	}
	return data, nil
}

// SendMagicLink sends a passwordless sign-in email to the user. The appID is
// optional; when omitted, the link lands on stackure.com's app launcher.
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

	resp, err := request(context.Background(), http.MethodPost, "/api/public/auth/magic-link/send", body, nil, nil)
	if err != nil {
		return nil, err
	}

	data, err := handleResponse(resp)
	if err != nil {
		return nil, err
	}

	result := &MagicLinkResponse{}
	if msg, ok := data["message"].(string); ok {
		result.Message = msg
	}
	return result, nil
}

// ValidateSession checks whether the given cookies hold a valid session for
// the app. It is used internally by Verify and Auth; most callers prefer
// those higher-level helpers.
func ValidateSession(appID string, cookies []*http.Cookie) (*sessionValidationResponse, error) {
	if err := validateUUID(appID, "App ID"); err != nil {
		return nil, err
	}

	query := url.Values{"app_id": {appID}}
	resp, err := request(context.Background(), http.MethodGet, "/api/public/auth/session/validate", nil, cookies, query)
	if err != nil {
		return nil, err
	}

	data, err := handleResponse(resp)
	if err != nil {
		return nil, err
	}

	result := &sessionValidationResponse{}
	if auth, ok := data["authenticated"].(bool); ok {
		result.Authenticated = auth
	}
	if signInURL, ok := data["sign_in_url"].(string); ok {
		result.SignInURL = signInURL
	}
	if userData, ok := data["user"].(map[string]any); ok {
		user := &User{}
		if v, ok := userData["user_id"].(string); ok {
			user.UserID = v
		}
		if v, ok := userData["user_email"].(string); ok {
			user.UserEmail = v
		}
		if v, ok := userData["user_first_name"].(string); ok {
			user.UserFirstName = v
		}
		if v, ok := userData["user_last_name"].(string); ok {
			user.UserLastName = v
		}
		if roles, ok := userData["user_roles"].([]any); ok {
			for _, r := range roles {
				if s, ok := r.(string); ok {
					user.UserRoles = append(user.UserRoles, s)
				}
			}
		}
		result.User = user
	}

	return result, nil
}

// Logout revokes the session represented by the given cookies.
func Logout(cookies []*http.Cookie) error {
	resp, err := request(context.Background(), http.MethodPost, "/api/public/auth/sign-out", nil, cookies, nil)
	if err != nil {
		return err
	}
	_, err = handleResponse(resp)
	return err
}
