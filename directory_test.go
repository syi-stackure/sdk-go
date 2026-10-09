package stackure_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	stackure "stackure.com/sdk-go"
)

const (
	adaJSON       = `"user_id":"2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c","account_id":"9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d","user_email":"ada@example.com","user_first_name":"Ada","user_last_name":"Lovelace"`
	opsJSON       = `{"team_id":"4b5c6d7e-8f9a-4b1c-8d2e-3f4a5b6c7d8e","team_name":"Ops"}`
	directoryJSON = `{"users":[{"user_id":"2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c","user_email":"ada@example.com","user_first_name":"Ada","user_last_name":"Lovelace"}],"teams":[` + opsJSON + `]}`
)

var ops = stackure.Team{TeamID: "4b5c6d7e-8f9a-4b1c-8d2e-3f4a5b6c7d8e", TeamName: "Ops"}

func TestIdentityFacts(t *testing.T) {
	ada := stackure.User{
		UserID:        "2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c",
		AccountID:     "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
		UserEmail:     "ada@example.com",
		UserFirstName: "Ada",
		UserLastName:  "Lovelace",
	}
	admin, member := ada, ada
	admin.UserIsAppAdmin, admin.UserTeams = true, []stackure.Team{ops}
	member.UserTeams = []stackure.Team{}
	for _, tc := range []struct {
		name string
		user string
		want stackure.User
	}{
		{name: "app admin in a team", user: adaJSON + `,"user_is_app_admin":true,"user_teams":[` + opsJSON + `]`, want: admin},
		{name: "no teams", user: adaJSON + `,"user_is_app_admin":false,"user_teams":[]`, want: member},
		{name: "older server", user: adaJSON, want: ada},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"authenticated":true,"user":{`+tc.user+`}}`)
			}))
			defer api.Close()
			t.Setenv("STACKURE_BASE_URL", api.URL)
			t.Setenv("STACKURE_APP_ID", appID)
			t.Setenv("STACKURE_APP_SECRET", appSecret)

			r := httptest.NewRequest(http.MethodGet, appOrigin+"/", nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
			s, err := stackure.ValidateSession(r)
			if err != nil || s.User == nil || !reflect.DeepEqual(*s.User, tc.want) {
				t.Errorf("session user = %+v, %v, want %+v", s, err, tc.want)
			}

			var user *stackure.User
			r = httptest.NewRequest(http.MethodPost, mcpURL, nil)
			r.Header.Set("Authorization", mcpBearer)
			stackure.MCP()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user = stackure.UserFromContext(r.Context())
			})).ServeHTTP(httptest.NewRecorder(), r)
			if user == nil || !reflect.DeepEqual(*user, tc.want) {
				t.Errorf("mcp user = %+v, want %+v", user, tc.want)
			}
		})
	}
}

func TestDirectory(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cookie     string
		bearer     bool
		noSecret   bool
		status     int
		body       string
		calls      int32
		want       *stackure.DirectoryResult
		code       string
		statusCode int
	}{
		{name: "listed", cookie: sessionToken, status: 200, body: directoryJSON, calls: 1, want: &stackure.DirectoryResult{
			Users: []stackure.DirectoryUser{{UserID: "2d6f0a1c-3b4e-4f5a-9b6c-7d8e9f0a1b2c", UserEmail: "ada@example.com", UserFirstName: "Ada", UserLastName: "Lovelace"}},
			Teams: []stackure.Team{ops},
		}},
		{name: "empty", cookie: sessionToken, status: 200, body: `{"users":[],"teams":[]}`, calls: 1, want: &stackure.DirectoryResult{Users: []stackure.DirectoryUser{}, Teams: []stackure.Team{}}},
		{name: "invalid session", cookie: sessionToken, status: 401, body: `{"error":"invalid session"}`, calls: 1, code: "auth", statusCode: 401},
		{name: "invalid app secret", cookie: sessionToken, status: 401, body: `{"error":"invalid app secret"}`, calls: 1, code: "auth", statusCode: 401},
		{name: "bad app id", cookie: sessionToken, status: 400, body: `{"error":"invalid app_id format"}`, calls: 1, code: "network", statusCode: 400},
		{name: "rate limited", cookie: sessionToken, status: 429, body: `{"error":"rate limited"}`, calls: 1, code: "network", statusCode: 429},
		{name: "server error", cookie: sessionToken, status: 500, calls: 2, code: "network", statusCode: 500},
		{name: "response not JSON", cookie: sessionToken, status: 200, body: "ok", calls: 1, code: "network", statusCode: 200},
		{name: "no session cookie", code: "auth"},
		{name: "malformed session cookie", cookie: sessionToken + "0", code: "auth"},
		{name: "mcp bearer only", bearer: true, code: "auth"},
		{name: "no app secret", cookie: sessionToken, noSecret: true, code: "validation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/public/directory" || r.URL.RawQuery != "app_id="+appID {
					t.Errorf("request = %s %s", r.Method, r.URL)
				}
				if got := r.Header["X-App-Secret"]; len(got) != 1 || got[0] != appSecret {
					t.Errorf("X-App-Secret = %q", got)
				}
				if got := r.Header["Cookie"]; len(got) != 1 || got[0] != "session="+sessionToken {
					t.Errorf("Cookie = %q", got)
				}
				if got := r.Header["Authorization"]; got != nil {
					t.Errorf("Authorization = %q", got)
				}
				if r.UserAgent() != "browser/1.0" || r.Header.Get("X-Forwarded-For") != "192.0.2.1" {
					t.Errorf("User-Agent = %q, X-Forwarded-For = %q", r.UserAgent(), r.Header.Get("X-Forwarded-For"))
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer api.Close()
			t.Setenv("STACKURE_BASE_URL", api.URL)
			t.Setenv("STACKURE_APP_ID", appID)
			t.Setenv("STACKURE_APP_SECRET", appSecret)
			if tc.noSecret {
				t.Setenv("STACKURE_APP_SECRET", "")
			}

			r := httptest.NewRequest(http.MethodGet, appOrigin+"/share", nil)
			r.Header.Set("User-Agent", "browser/1.0")
			r.Header.Set("X-Forwarded-For", "192.0.2.1")
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "session", Value: tc.cookie})
			}
			if tc.bearer {
				r.Header.Set("Authorization", mcpBearer)
			}
			got, err := stackure.Directory(r)

			if n := calls.Load(); n != tc.calls {
				t.Errorf("calls = %d, want %d", n, tc.calls)
			}
			if tc.code == "" {
				if err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Errorf("directory = %+v, %v, want %+v", got, err, tc.want)
				}
				return
			}
			var se *stackure.StackureError
			if got != nil || !errors.As(err, &se) || se.Code != tc.code || se.StatusCode != tc.statusCode {
				t.Errorf("directory = %+v, %v, want %s error with status %d", got, err, tc.code, tc.statusCode)
			}
			if tc.status == 401 && se.Message != tc.body {
				t.Errorf("message = %q, want %q", se.Message, tc.body)
			}
		})
	}
}
