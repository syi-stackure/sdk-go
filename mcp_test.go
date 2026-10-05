package stackure_test

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	stackure "stackure.com/sdk-go"
)

const (
	mcpURL       = appOrigin + "/mcp"
	mcpTLSURL    = tlsOrigin + "/mcp"
	mcpBearer    = "Bearer " + sessionToken
	mcpMetadata  = "https://stackure.test/.well-known/oauth-protected-resource/mcp/" + appID + "?resource=http%3A%2F%2Fapp.test%3A8080%2Fmcp"
	mcpChallenge = `Bearer resource_metadata="` + mcpMetadata + `"`
	mcpSignedOut = `{"authenticated":false,"sign_in_url":"https://stackure.test/sign-in/magic-link?app_id=` + appID + `","www_authenticate":"Bearer resource_metadata=\"` + mcpMetadata + `\""}`
	mcpSignedIn  = `{"authenticated":true,"user":{"user_id":"2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c","account_id":"9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d","user_email":"ada@example.com","user_first_name":"Ada","user_last_name":"Lovelace","user_permissions":["can_read","can_write"]}}`
	mcpNoPerms   = `{"authenticated":true,"user":{"user_id":"2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c","account_id":"9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d","user_email":"ada@example.com","user_first_name":"Ada","user_last_name":"Lovelace"}}`
	mcpCall      = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	mcpTools     = `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
)

func TestMCP(t *testing.T) {
	bodies := map[int]string{200: mcpTools, 401: `{"error":"unauthorized"}`, 403: `{"error":"forbidden"}`, 503: `{"error":"unavailable"}`}
	forwardedTLS := http.Header{"X-Forwarded-Proto": {"https"}}
	for _, tc := range []struct {
		name      string
		appID     string
		target    string
		hdr       http.Header
		auth      string
		cookie    bool
		handoff   bool
		perms     []string
		noSecret  bool
		status    int
		body      string
		stall     bool
		down      bool
		bearer    bool
		mcp       string
		calls     int32
		code      int
		challenge string
		user      *stackure.User
	}{
		{name: "valid bearer", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "lowercase scheme", appID: appID, target: mcpURL, auth: "bearer " + sessionToken, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "uppercase scheme", appID: appID, target: mcpURL, auth: "BEARER " + sessionToken, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "spaces after scheme", appID: appID, target: mcpURL, auth: "Bearer   " + sessionToken, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "valid bearer with session cookie", appID: appID, target: mcpURL, auth: mcpBearer, cookie: true, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "valid bearer without permissions", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: mcpNoPerms, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "required permission held", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_write"}, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "one of the required permissions held", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_admin", "can_read"}, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "https over TLS", appID: appID, target: mcpTLSURL, auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpTLSURL, calls: 1, code: 200},
		{name: "https behind TLS proxy", appID: appID, target: mcpURL, hdr: forwardedTLS, auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpTLSURL, calls: 1, code: 200},
		{name: "http forwarded", appID: appID, target: mcpURL, hdr: http.Header{"X-Forwarded-Proto": {"http"}}, auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "nested path", appID: appID, target: appOrigin + "/api/v1/mcp", auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: appOrigin + "/api/v1/mcp", calls: 1, code: 200},
		{name: "trailing slash", appID: appID, target: mcpURL + "/", auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL + "/", calls: 1, code: 200},
		{name: "root path", appID: appID, target: appOrigin + "/", auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: appOrigin + "/", calls: 1, code: 200},
		{name: "escaped path", appID: appID, target: appOrigin + "/mcp%20tools/a%2Fb", auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: appOrigin + "/mcp%20tools/a%2Fb", calls: 1, code: 200},
		{name: "query string dropped", appID: appID, target: mcpURL + "?session=1&next=%2Fadmin", auth: mcpBearer, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 200},
		{name: "query string dropped over TLS", appID: appID, target: tlsOrigin + "/api/mcp?mcp=https%3A%2F%2Fevil.test%2Fmcp&app_id=x", status: 200, body: mcpSignedOut, mcp: tlsOrigin + "/api/mcp", calls: 1, code: 401, challenge: mcpChallenge},
		{name: "no authorization header", appID: appID, target: mcpURL, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "empty authorization header", appID: appID, target: mcpURL, hdr: http.Header{"Authorization": {""}}, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "malformed token", appID: appID, target: mcpURL, auth: "Bearer not-a-token", status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "token with extra character", appID: appID, target: mcpURL, auth: mcpBearer + "0", status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "token cut short", appID: appID, target: mcpURL, auth: mcpBearer[:len(mcpBearer)-1], status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "two tokens", appID: appID, target: mcpURL, auth: mcpBearer + " " + sessionToken, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "scheme without token", appID: appID, target: mcpURL, auth: "Bearer", status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "token without scheme", appID: appID, target: mcpURL, auth: sessionToken, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "other scheme", appID: appID, target: mcpURL, auth: "Basic " + sessionToken, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "scheme with prefix", appID: appID, target: mcpURL, auth: "XBearer " + sessionToken, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "session cookie without bearer", appID: appID, target: mcpURL, cookie: true, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "session cookie with malformed bearer", appID: appID, target: mcpURL, auth: "Bearer not-a-token", cookie: true, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "sign-in handoff without bearer", appID: appID, target: mcpURL, handoff: true, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "browser without bearer", appID: appID, target: mcpURL, hdr: http.Header{"Accept": {"text/html"}}, status: 200, body: mcpSignedOut, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "bearer rejected", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: mcpSignedOut, bearer: true, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "bearer rejected with permission required", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_write"}, status: 200, body: mcpSignedOut, bearer: true, mcp: mcpURL, calls: 1, code: 401, challenge: mcpChallenge},
		{name: "no challenge in response", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: `{"authenticated":false}`, bearer: true, mcp: mcpURL, calls: 1, code: 401, challenge: "Bearer"},
		{name: "authenticated without user", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: `{"authenticated":true}`, bearer: true, mcp: mcpURL, calls: 1, code: 401, challenge: "Bearer"},
		{name: "user without authenticated", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: strings.Replace(mcpSignedIn, "true", "false", 1), bearer: true, mcp: mcpURL, calls: 1, code: 401, challenge: "Bearer"},
		{name: "required permission missing", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_admin"}, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 403},
		{name: "every required permission missing", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_admin", "can_delete"}, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 403},
		{name: "required permission in other case", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"CAN_WRITE"}, status: 200, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 403},
		{name: "permission required with none granted", appID: appID, target: mcpURL, auth: mcpBearer, perms: []string{"can_write"}, status: 200, body: mcpNoPerms, bearer: true, mcp: mcpURL, calls: 1, code: 403},
		{name: "invalid app secret", appID: appID, target: mcpURL, auth: mcpBearer, status: 401, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "bad request", appID: appID, target: mcpURL, auth: mcpBearer, status: 400, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "forbidden", appID: appID, target: mcpURL, auth: mcpBearer, status: 403, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "rate limited", appID: appID, target: mcpURL, auth: mcpBearer, status: 429, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "server error", appID: appID, target: mcpURL, auth: mcpBearer, status: 500, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 2, code: 503},
		{name: "redirected", appID: appID, target: mcpURL, auth: mcpBearer, status: 302, body: mcpSignedIn, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "response not JSON", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, body: "ok", bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "empty response", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "response never arrives", appID: appID, target: mcpURL, auth: mcpBearer, status: 200, stall: true, bearer: true, mcp: mcpURL, calls: 1, code: 503},
		{name: "unreachable", appID: appID, target: mcpURL, auth: mcpBearer, down: true, code: 503},
		{name: "unreachable without bearer", appID: appID, target: mcpURL, down: true, code: 503},
		{name: "no app secret", appID: appID, target: mcpURL, auth: mcpBearer, noSecret: true, status: 200, body: mcpSignedIn, code: 503},
		{name: "malformed app id", appID: appID + "0", target: mcpURL, auth: mcpBearer, status: 200, body: mcpSignedIn, code: 503},
		{name: "no app id", target: mcpURL, auth: mcpBearer, status: 200, body: mcpSignedIn, code: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/public/auth/session/validate" {
					t.Errorf("request = %s %s", r.Method, r.URL)
					return
				}
				if got, want := r.URL.RawQuery, "app_id="+appID+"&mcp="+url.QueryEscape(tc.mcp); got != want {
					t.Errorf("query = %q, want %q", got, want)
				}
				if got := r.Header["X-App-Secret"]; len(got) != 1 || got[0] != appSecret {
					t.Errorf("X-App-Secret = %q", got)
				}
				if got := r.Header["Authorization"]; tc.bearer && (len(got) != 1 || got[0] != mcpBearer) || !tc.bearer && got != nil {
					t.Errorf("Authorization = %q, bearer wanted = %v", got, tc.bearer)
				}
				if dump, _ := httputil.DumpRequest(r, true); r.Header["Cookie"] != nil || !tc.bearer && bytes.Contains(dump, []byte(sessionToken)) {
					t.Errorf("cookie or unchecked token sent on validate call: %s", dump)
				}
				if got := r.UserAgent(); got != "claude-code/1.0" {
					t.Errorf("User-Agent = %q", got)
				}
				if got := r.Header.Get("X-Forwarded-For"); got != "192.0.2.1" {
					t.Errorf("X-Forwarded-For = %q", got)
				}
				if tc.stall {
					w.Header().Set("Content-Length", "64")
					w.WriteHeader(tc.status)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				if tc.status == http.StatusFound {
					w.Header().Set("Location", "/followed")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			if tc.down {
				srv.Close()
			}
			t.Setenv("STACKURE_BASE_URL", srv.URL)
			if tc.noSecret {
				t.Setenv("STACKURE_APP_SECRET", "")
			} else {
				t.Setenv("STACKURE_APP_SECRET", appSecret)
			}
			var logs bytes.Buffer
			log.SetOutput(&logs)
			defer log.SetOutput(os.Stderr)

			r := httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(mcpCall))
			r.Header.Set("Content-Type", "application/json")
			if tc.handoff {
				r = httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader("session_token="+sessionToken))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Header.Set("Origin", srv.URL)
			}
			r.Header.Set("User-Agent", "claude-code/1.0")
			if tc.auth != "" {
				r.Header.Set("Authorization", tc.auth)
			}
			for k, v := range tc.hdr {
				r.Header[k] = v
			}
			if tc.cookie {
				r.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
			}
			var reached bool
			var user *stackure.User
			var received string
			w := httptest.NewRecorder()
			stackure.MCP(tc.appID, tc.perms...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				user = stackure.UserFromContext(r.Context())
				b, _ := io.ReadAll(r.Body)
				received = string(b)
				io.WriteString(w, mcpTools)
			})).ServeHTTP(w, r)
			res := w.Result()

			if got := calls.Load(); got != tc.calls {
				t.Errorf("validate calls = %d, want %d", got, tc.calls)
			}
			if res.StatusCode != tc.code {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.code)
			}
			if got, want := w.Body.String(), bodies[tc.code]; got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if got := res.Header["Www-Authenticate"]; tc.challenge != "" && (len(got) != 1 || got[0] != tc.challenge) || tc.challenge == "" && got != nil {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tc.challenge)
			}
			if res.Header["Location"] != nil || res.Header["Set-Cookie"] != nil {
				t.Errorf("headers = %v, want no redirect and no cookie", res.Header)
			}
			if tc.code == http.StatusOK {
				want := stackure.User{
					UserID:        "2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c",
					AccountID:     "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
					UserEmail:     "ada@example.com",
					UserFirstName: "Ada",
					UserLastName:  "Lovelace",
				}
				if tc.body == mcpSignedIn {
					want.UserPermissions = []string{"can_read", "can_write"}
				}
				if !reached || user == nil || !reflect.DeepEqual(*user, want) {
					t.Errorf("reached = %v, user = %+v, want %+v", reached, user, want)
				}
				if received != mcpCall {
					t.Errorf("handler received body %q, want %q", received, mcpCall)
				}
			} else {
				if reached {
					t.Errorf("handler reached on %d", tc.code)
				}
				if got := res.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q", got)
				}
			}
			if tc.code == http.StatusServiceUnavailable && !strings.Contains(logs.String(), "stackure: verification error") {
				t.Errorf("validate failure not logged: %q", logs.String())
			}
			dump, _ := httputil.DumpResponse(res, false)
			for _, secret := range []string{sessionToken, appSecret} {
				if bytes.Contains(dump, []byte(secret)) || strings.Contains(w.Body.String(), secret) || strings.Contains(logs.String(), secret) {
					t.Errorf("%q exposed in response or logs", secret)
				}
			}
		})
	}
}

func TestMCPMounted(t *testing.T) {
	var calls atomic.Int32
	var mcp, cookie atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mcp.Store(r.URL.Query().Get("mcp"))
		cookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-App-Secret") != appSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") == mcpBearer {
			io.WriteString(w, mcpSignedIn)
			return
		}
		io.WriteString(w, mcpSignedOut)
	}))
	defer api.Close()
	t.Setenv("STACKURE_BASE_URL", api.URL)
	t.Setenv("STACKURE_APP_SECRET", appSecret)
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		io.WriteString(w, stackure.UserFromContext(r.Context()).UserEmail+" "+string(b))
	})
	mux := http.NewServeMux()
	mux.Handle("/api/mcp", stackure.MCP(appID)(echo))
	mux.Handle("/admin/mcp", stackure.MCP(appID, "can_admin")(echo))
	app := httptest.NewServer(mux)
	defer app.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, tc := range []struct {
		name      string
		method    string
		path      string
		hdr       http.Header
		mcp       string
		code      int
		challenge string
		body      string
	}{
		{name: "post with bearer", method: "POST", path: "/api/mcp", hdr: http.Header{"Authorization": {mcpBearer}}, mcp: "/api/mcp", code: 200, body: "ada@example.com " + mcpCall},
		{name: "post with bearer and query", method: "POST", path: "/api/mcp?trace=1", hdr: http.Header{"Authorization": {mcpBearer}}, mcp: "/api/mcp", code: 200, body: "ada@example.com " + mcpCall},
		{name: "post without bearer", method: "POST", path: "/api/mcp", hdr: http.Header{}, mcp: "/api/mcp", code: 401, challenge: mcpChallenge, body: `{"error":"unauthorized"}`},
		{name: "get from browser with cookie", method: "GET", path: "/api/mcp", hdr: http.Header{"Accept": {"text/html"}, "Cookie": {"session=" + sessionToken}}, mcp: "/api/mcp", code: 401, challenge: mcpChallenge, body: `{"error":"unauthorized"}`},
		{name: "delete with bearer", method: "DELETE", path: "/api/mcp", hdr: http.Header{"Authorization": {mcpBearer}}, mcp: "/api/mcp", code: 200, body: "ada@example.com " + mcpCall},
		{name: "post with bearer lacking permission", method: "POST", path: "/admin/mcp", hdr: http.Header{"Authorization": {mcpBearer}}, mcp: "/admin/mcp", code: 403, body: `{"error":"forbidden"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls.Store(0)
			req, err := http.NewRequest(tc.method, app.URL+tc.path, strings.NewReader(mcpCall))
			if err != nil {
				t.Fatal(err)
			}
			req.Header = tc.hdr.Clone()
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if got := calls.Load(); got != 1 {
				t.Errorf("validate calls = %d, want 1", got)
			}
			if got, want := mcp.Load(), app.URL+tc.mcp; got != want {
				t.Errorf("mcp = %q, want %q", got, want)
			}
			if got := cookie.Load(); got != "" {
				t.Errorf("Cookie = %q on validate call", got)
			}
			if got := res.Header.Get("WWW-Authenticate"); res.StatusCode != tc.code || got != tc.challenge || string(b) != tc.body {
				t.Errorf("response = %d %q %q, want %d %q %q", res.StatusCode, got, b, tc.code, tc.challenge, tc.body)
			}
			if res.Header["Location"] != nil || res.Header["Set-Cookie"] != nil {
				t.Errorf("headers = %v, want no redirect and no cookie", res.Header)
			}
		})
	}
}

func TestMCPStripPrefix(t *testing.T) {
	var mcp atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mcp.Store(r.URL.Query().Get("mcp"))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, mcpSignedIn)
	}))
	defer api.Close()
	t.Setenv("STACKURE_BASE_URL", api.URL)
	t.Setenv("STACKURE_APP_SECRET", appSecret)
	mux := http.NewServeMux()
	mux.Handle("/nested/", http.StripPrefix("/nested", stackure.MCP(appID)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))))
	app := httptest.NewServer(mux)
	defer app.Close()
	req, _ := http.NewRequest(http.MethodPost, app.URL+"/nested/mcp?x=1", nil)
	req.Header.Set("Authorization", mcpBearer)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if got, want := mcp.Load(), app.URL+"/nested/mcp"; got != want {
		t.Fatalf("mcp = %v, want %v", got, want)
	}
}
