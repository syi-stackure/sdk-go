package stackure_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	stackure "stackure.com/sdk-go"
)

const (
	sessionToken = "0b9d2c4e-5f6a-4b7c-8d9e-0a1b2c3d4e5f"
	appID        = "7f3c1a2e-9b4d-4e6f-8a1b-2c3d4e5f6071"
	appSecret    = "app-secret-value"
	appHost      = "app.test:8080"
	appOrigin    = "http://" + appHost
	tlsOrigin    = "https://" + appHost
	signedOut    = `{"message":"signed out successfully"}`
)

func TestLogout(t *testing.T) {
	twice := func(v string) []string { return []string{v, v} }
	forwardedTLS := http.Header{"X-Forwarded-Proto": {"https"}}
	for _, tc := range []struct {
		name     string
		method   string
		tls      bool
		site     string
		origin   string
		hdr      http.Header
		token    string
		noSecret bool
		status   int
		body     string
		cut      bool
		stall    bool
		down     bool
		gone     bool
		calls    int32
		cleared  bool
		dest     string
	}{
		{name: "signed out", method: "POST", site: "same-origin", token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "empty body", method: "POST", site: "same-origin", token: sessionToken, status: 200, calls: 1, cleared: true, dest: "/"},
		{name: "body not JSON", method: "POST", site: "same-origin", token: sessionToken, status: 200, body: "ok", calls: 1, cleared: true, dest: "/"},
		{name: "body cut short", method: "POST", site: "same-origin", token: sessionToken, status: 200, cut: true, calls: 1, cleared: true, dest: "/"},
		{name: "body never arrives", method: "POST", site: "same-origin", token: sessionToken, status: 200, stall: true, calls: 1, cleared: true, dest: "/"},
		{name: "no content", method: "POST", site: "same-origin", token: sessionToken, status: 204, calls: 1, cleared: true, dest: "/"},
		{name: "no app secret", method: "POST", site: "same-origin", token: sessionToken, noSecret: true, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "client gone", method: "POST", site: "same-origin", token: sessionToken, status: 200, body: signedOut, gone: true, calls: 1, cleared: true, dest: "/"},
		{name: "no token", method: "POST", site: "same-origin", status: 200, body: signedOut, cleared: true, dest: "/"},
		{name: "malformed token", method: "POST", site: "same-origin", token: sessionToken + "0", status: 200, body: signedOut, cleared: true, dest: "/"},
		{name: "rejected", method: "POST", site: "same-origin", token: sessionToken, status: 401, calls: 1, cleared: true, dest: "/signout"},
		{name: "rejected with body cut short", method: "POST", site: "same-origin", token: sessionToken, status: 401, cut: true, calls: 1, cleared: true, dest: "/signout"},
		{name: "server error", method: "POST", site: "same-origin", token: sessionToken, status: 500, calls: 2, cleared: true, dest: "/signout"},
		{name: "redirected", method: "POST", site: "same-origin", token: sessionToken, status: 302, calls: 1, cleared: true, dest: "/signout"},
		{name: "unreachable", method: "POST", site: "same-origin", token: sessionToken, down: true, cleared: true, dest: "/signout"},
		{name: "same-origin with other origin", method: "POST", site: "same-origin", origin: "https://public.test", token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "same-origin with host header", method: "POST", site: "same-origin", hdr: http.Header{"Host": {appHost}}, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "matching origin", method: "POST", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "matching origin in other case", method: "POST", origin: "http://APP.Test:8080", token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "matching origin with host header", method: "POST", origin: appOrigin, hdr: http.Header{"Host": {appHost}}, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "https origin over TLS", method: "POST", tls: true, origin: tlsOrigin, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "https origin behind TLS proxy", method: "POST", origin: tlsOrigin, hdr: forwardedTLS, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "https origin with scheme not forwarded", method: "POST", origin: tlsOrigin, token: sessionToken, status: 200, body: signedOut, calls: 1, cleared: true, dest: "/"},
		{name: "http origin over TLS", method: "POST", tls: true, origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "http origin behind TLS proxy", method: "POST", origin: appOrigin, hdr: forwardedTLS, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "get", method: "GET", site: "same-origin", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "head", method: "HEAD", site: "same-origin", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "cross-site", method: "POST", site: "cross-site", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "cross-site with matching origin", method: "POST", site: "cross-site", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "same-site", method: "POST", site: "same-site", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "none", method: "POST", site: "none", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "same-origin in other case", method: "POST", site: "Same-Origin", origin: appOrigin, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "empty site with matching origin", method: "POST", origin: appOrigin, hdr: http.Header{"Sec-Fetch-Site": {""}}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "repeated site", method: "POST", hdr: http.Header{"Sec-Fetch-Site": twice("same-origin")}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "repeated site with matching origin", method: "POST", origin: appOrigin, hdr: http.Header{"Sec-Fetch-Site": twice("same-origin")}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "repeated origin", method: "POST", hdr: http.Header{"Origin": twice(appOrigin)}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "same-origin with repeated origin", method: "POST", site: "same-origin", hdr: http.Header{"Origin": twice(appOrigin)}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "repeated host", method: "POST", origin: appOrigin, hdr: http.Header{"Host": twice(appHost)}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "same-origin with repeated host", method: "POST", site: "same-origin", hdr: http.Header{"Host": twice(appHost)}, token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "no site or origin", method: "POST", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "other origin", method: "POST", origin: "http://evil.test:8080", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "other port", method: "POST", origin: "http://app.test", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "null origin", method: "POST", origin: "null", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin host with prefix", method: "POST", origin: "http://evilapp.test:8080", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin host as parent domain", method: "POST", origin: "http://evil.app.test:8080", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin host with suffix", method: "POST", origin: "http://app.test.evil.test:8080", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin host and port with suffix", method: "POST", origin: "http://app.test:8080.evil.test", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin port with extra digit", method: "POST", origin: "http://app.test:80800", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
		{name: "origin host as userinfo", method: "POST", origin: "http://app.test:8080@evil.test", token: sessionToken, status: 200, body: signedOut, dest: "/signout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				b, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || r.URL.Path != "/api/public/auth/sign-out" || r.URL.RawQuery != "" {
					t.Errorf("request = %s %s", r.Method, r.URL)
					return
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+sessionToken {
					t.Errorf("Authorization = %q", got)
				}
				if len(b) != 0 || r.Header.Get("Cookie") != "" {
					t.Errorf("body = %q, headers = %v", b, r.Header)
				}
				if !tc.noSecret {
					if dump, _ := httputil.DumpRequest(r, false); r.Header["X-App-Secret"] != nil || bytes.Contains(dump, []byte(appSecret)) {
						t.Errorf("app secret sent on sign-out call: %s", dump)
					}
				}
				if got := r.UserAgent(); got != "browser/1.0" {
					t.Errorf("User-Agent = %q", got)
				}
				if tc.cut {
					c, bw, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					fmt.Fprintf(bw, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s", tc.status, http.StatusText(tc.status), len(signedOut), signedOut[:8])
					bw.Flush()
					c.Close()
					return
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

			target := appOrigin
			if tc.tls {
				target = tlsOrigin
			}
			r := httptest.NewRequest(tc.method, target+"/logout", nil)
			r.Header.Set("User-Agent", "browser/1.0")
			if tc.site != "" {
				r.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			for k, v := range tc.hdr {
				r.Header[k] = v
			}
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: "session", Value: tc.token})
			}
			if tc.gone {
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			stackure.Logout(w, r)
			res := w.Result()

			if got := calls.Load(); got != tc.calls {
				t.Errorf("sign-out calls = %d, want %d", got, tc.calls)
			}
			if res.StatusCode != http.StatusSeeOther {
				t.Errorf("status = %d, want 303", res.StatusCode)
			}
			if got, want := res.Header.Get("Location"), srv.URL+tc.dest; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			cs := res.Cookies()
			if tc.cleared {
				if len(cs) != 1 || cs[0].Name != "session" || cs[0].Value != "" || cs[0].MaxAge >= 0 || cs[0].Path != "/" || !cs[0].HttpOnly {
					t.Errorf("cookies = %v, want cleared session cookie", cs)
				}
			} else if len(cs) != 0 {
				t.Errorf("cookies = %v, want none", cs)
			}
			dump, _ := httputil.DumpResponse(res, true)
			for _, secret := range []string{sessionToken, appSecret} {
				if strings.Contains(string(dump), secret) || strings.Contains(logs.String(), secret) {
					t.Errorf("%q exposed in response or logs", secret)
				}
			}
		})
	}
}

func TestLogoutMounted(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer api.Close()
	t.Setenv("STACKURE_BASE_URL", api.URL)
	mux := http.NewServeMux()
	mux.HandleFunc("/logout", stackure.Logout)
	app := httptest.NewServer(mux)
	defer app.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sameOrigin := http.Header{"Sec-Fetch-Site": {"same-origin"}}
	for _, tc := range []struct {
		name   string
		method string
		hdr    http.Header
		calls  int32
		dest   string
	}{
		{name: "post from own page", method: "POST", hdr: sameOrigin, calls: 1, dest: "/"},
		{name: "post with matching origin", method: "POST", hdr: http.Header{"Origin": {app.URL}}, calls: 1, dest: "/"},
		{name: "get", method: "GET", hdr: sameOrigin, dest: "/signout"},
		{name: "head", method: "HEAD", hdr: sameOrigin, dest: "/signout"},
		{name: "put", method: "PUT", hdr: sameOrigin, dest: "/signout"},
		{name: "delete", method: "DELETE", hdr: sameOrigin, dest: "/signout"},
		{name: "empty site line with matching origin", method: "POST", hdr: http.Header{"Sec-Fetch-Site": {""}, "Origin": {app.URL}}, dest: "/signout"},
		{name: "repeated site line", method: "POST", hdr: http.Header{"Sec-Fetch-Site": {"same-origin", "same-origin"}}, dest: "/signout"},
		{name: "repeated origin line", method: "POST", hdr: http.Header{"Origin": {app.URL, app.URL}}, dest: "/signout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls.Store(0)
			req, err := http.NewRequest(tc.method, app.URL+"/logout", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header = tc.hdr.Clone()
			req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if got := calls.Load(); got != tc.calls {
				t.Errorf("sign-out calls = %d, want %d", got, tc.calls)
			}
			if got, want := res.Header.Get("Location"), api.URL+tc.dest; res.StatusCode != http.StatusSeeOther || got != want {
				t.Errorf("response = %d %q, want 303 %q", res.StatusCode, got, want)
			}
		})
	}
}
