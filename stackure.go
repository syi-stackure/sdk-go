package stackure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://stackure.com"
	defaultTimeout = 10 * time.Second
	maxRetries     = 2
)

// ---------- Types ----------

// User represents an authenticated user returned from Stackure.
type User struct {
	UserID        string   `json:"user_id"`
	UserEmail     string   `json:"user_email"`
	UserFirstName string   `json:"user_first_name"`
	UserLastName  string   `json:"user_last_name"`
	UserRoles     []string `json:"user_roles"`
}

// SessionValidationResponse is the response from a session validation request.
type SessionValidationResponse struct {
	Authenticated bool   `json:"authenticated"`
	User          *User  `json:"user,omitempty"`
	SignInURL     string `json:"sign_in_url,omitempty"`
}

// MagicLinkResponse is the response from a magic-link send request.
type MagicLinkResponse struct {
	Message string `json:"message"`
	Token   string `json:"token,omitempty"`
}

// Config holds configuration options for the Stackure client.
type Config struct {
	// BaseURL is the base URL of the Stackure API. Default: "https://stackure.com"
	BaseURL string
	// Timeout is the HTTP request timeout. Default: 10s
	Timeout time.Duration
}

// VerifyError contains error details from a failed verification.
type VerifyError struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	SignInURL string `json:"sign_in_url,omitempty"`
}

// VerifyResult is the result of an authentication verification check.
type VerifyResult struct {
	Authenticated bool
	User          *User
	Error         *VerifyError
}

// ---------- Client ----------

// Client is an HTTP client for the Stackure authentication API.
type Client struct {
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
}

// NewClient creates a new Stackure client. If no Config is provided, defaults
// are used (base URL: https://stackure.com, timeout: 10s).
func NewClient(config ...Config) *Client {
	baseURL := defaultBaseURL
	timeout := defaultTimeout

	if len(config) > 0 {
		cfg := config[0]
		if cfg.BaseURL != "" {
			baseURL = strings.TrimRight(cfg.BaseURL, "/")
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
	}

	return &Client{
		baseURL: baseURL,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// request performs an HTTP request with retry logic.
// Retries up to 2 times on 5xx errors with exponential backoff (500ms, 1s).
// Timeouts are never retried.
func (c *Client) request(ctx context.Context, method, path string, body interface{}, cookies []*http.Cookie, query url.Values) (*http.Response, error) {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1000ms
			backoff := time.Duration(500*(1<<(attempt-1))) * time.Millisecond
			time.Sleep(backoff)
		}

		var bodyReader io.Reader
		if body != nil {
			jsonBytes, err := json.Marshal(body)
			if err != nil {
				return nil, NewNetworkError(fmt.Sprintf("Failed to marshal request body: %v", err), 0)
			}
			bodyReader = bytes.NewReader(jsonBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, NewNetworkError(fmt.Sprintf("Failed to create request: %v", err), 0)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check if the error is a timeout — do not retry timeouts.
			if ctx.Err() == context.DeadlineExceeded || isTimeoutError(err) {
				return nil, NewTimeoutError(fmt.Sprintf("Request timed out after %s", c.timeout))
			}
			lastErr = NewNetworkError(fmt.Sprintf("Network request failed: %v", err), 0)
			continue
		}

		// Retry 5xx errors unless this is the last attempt.
		if resp.StatusCode >= 500 && attempt < maxRetries {
			resp.Body.Close()
			lastErr = NewNetworkError(fmt.Sprintf("Server error (%d)", resp.StatusCode), resp.StatusCode)
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, NewNetworkError("Request failed after retries", 0)
}

// isTimeoutError checks whether an error is a timeout.
func isTimeoutError(err error) bool {
	type timeout interface {
		Timeout() bool
	}
	if t, ok := err.(timeout); ok {
		return t.Timeout()
	}
	return false
}

// handleResponse reads the response body and returns the parsed JSON.
// It returns an appropriate error for non-2xx status codes.
func handleResponse(resp *http.Response) (map[string]interface{}, error) {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewNetworkError("Failed to read response body", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorText := string(bodyBytes)
		if errorText == "" {
			errorText = "Unknown error"
		}
		if resp.StatusCode == 401 {
			return nil, NewAuthenticationError(errorText)
		}
		return nil, NewNetworkError(fmt.Sprintf("API error (%d): %s", resp.StatusCode, errorText), resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, NewNetworkError("Invalid JSON response from server", resp.StatusCode)
	}
	return data, nil
}

// SendMagicLink sends a magic-link authentication email to a user.
// The appID parameter is optional; pass an empty variadic to omit it.
func (c *Client) SendMagicLink(email string, appID ...string) (*MagicLinkResponse, error) {
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

	resp, err := c.request(context.Background(), http.MethodPost, "/api/public/auth/magic-link/send", body, nil, nil)
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
	if token, ok := data["token"].(string); ok {
		result.Token = token
	}
	return result, nil
}

// ValidateSession validates the current session for an application.
func (c *Client) ValidateSession(appID string, cookies []*http.Cookie) (*SessionValidationResponse, error) {
	if err := validateUUID(appID, "App ID"); err != nil {
		return nil, err
	}

	query := url.Values{"app_id": {appID}}
	resp, err := c.request(context.Background(), http.MethodGet, "/api/public/auth/session/validate", nil, cookies, query)
	if err != nil {
		return nil, err
	}

	data, err := handleResponse(resp)
	if err != nil {
		return nil, err
	}

	result := &SessionValidationResponse{}
	if auth, ok := data["authenticated"].(bool); ok {
		result.Authenticated = auth
	}
	if signInURL, ok := data["sign_in_url"].(string); ok {
		result.SignInURL = signInURL
	}

	if userData, ok := data["user"].(map[string]interface{}); ok {
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
		if roles, ok := userData["user_roles"].([]interface{}); ok {
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

// Logout signs out the current user from all Stackure applications.
func (c *Client) Logout(cookies []*http.Cookie) error {
	resp, err := c.request(context.Background(), http.MethodPost, "/api/public/auth/sign-out", nil, cookies, nil)
	if err != nil {
		return err
	}
	_, err = handleResponse(resp)
	return err
}

// SignIn initiates sign-in for a user. When email is provided, sends a
// magic-link directly. When omitted, returns nil.
func (c *Client) SignIn(appID string, email ...string) (*MagicLinkResponse, error) {
	if err := validateUUID(appID, "App ID"); err != nil {
		return nil, err
	}
	if len(email) > 0 && email[0] != "" {
		return c.SendMagicLink(email[0], appID)
	}
	return nil, nil
}

// ---------- Default client (package-level convenience functions) ----------

var defaultClient = NewClient()

// SendMagicLink sends a magic-link authentication email using the default client.
func SendMagicLink(email string, appID ...string) (*MagicLinkResponse, error) {
	return defaultClient.SendMagicLink(email, appID...)
}

// ValidateSession validates a session using the default client.
func ValidateSession(appID string, cookies []*http.Cookie) (*SessionValidationResponse, error) {
	return defaultClient.ValidateSession(appID, cookies)
}

// Logout signs out the current user using the default client.
func Logout(cookies []*http.Cookie) error {
	return defaultClient.Logout(cookies)
}

// SignIn initiates sign-in using the default client.
func SignIn(appID string, email ...string) (*MagicLinkResponse, error) {
	return defaultClient.SignIn(appID, email...)
}
