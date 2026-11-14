package server

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	simonName    = "simon"
	mePath       = "/me"
	testPassword = "correct horse"
	cookieHeader = "Cookie"
)

// session is a user and a live session token.
type session struct {
	user  store.User
	token string
}

// sessionFor seeds simon as an admin and logs him in through the service.
func (ts *testServer) sessionFor(tb testing.TB) session {
	tb.Helper()
	u := ts.seedUser(tb, simonName, testPassword, store.RoleAdmin)
	res, err := ts.Auth.Login(tb.Context(), simonName, testPassword, "test/1")
	if err != nil {
		tb.Fatalf("login: %v", err)
	}
	return session{user: u, token: res.Token}
}

func cookieOnly(token string) http.Header {
	return http.Header{cookieHeader: {sessionCookie + "=" + token}}
}

func TestMeWithBearer(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	token, u := sess.token, sess.user

	resp := ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(token)})
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(resp.Body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]any{
		"id":        float64(u.ID),
		"name":      simonName,
		"role":      "admin",
		"createdAt": u.CreatedAt.Format(time.RFC3339Nano),

		"canEditTags": false,
		"canTrash":    false,
	}
	if len(got) != len(want) {
		t.Errorf("body = %v, want exactly the fields of %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestMeWithoutCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	resp := ts.send(t, call{method: http.MethodGet, path: mePath, invalid: true})
	resp.problem(t, http.StatusUnauthorized)
	if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
}

func TestMeWithBadScheme(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	h := http.Header{authorizationHeader: {"Basic " + token}}
	ts.send(t, call{method: http.MethodGet, path: mePath, header: h, invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestMeWithExpiredSession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.Clock.Advance(31 * 24 * time.Hour)
	ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(token)}).problem(t, http.StatusUnauthorized)
}

// TestMeWithCookie also proves that the contract validator accepts a request
// whose only credential is the declared session cookie.
func TestMeWithCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	resp := ts.send(t, call{method: http.MethodGet, path: mePath, header: cookieOnly(token)})
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
}

func TestBearerWinsOverCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	h := cookieOnly(token)
	h.Set(authorizationHeader, "Bearer garbage")
	ts.send(t, call{method: http.MethodGet, path: mePath, header: h}).problem(t, http.StatusUnauthorized)
}

func TestMeSchemeIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	h := http.Header{authorizationHeader: {"bearer " + token}}
	if resp := ts.send(t, call{method: http.MethodGet, path: mePath, header: h}); resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
}

func TestUnauthorizedNeverEchoesToken(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	valid := cookieOnly(token)
	valid.Set(authorizationHeader, "Bearer garbage")
	expired := newTestServer(t)
	expiredToken := expired.sessionFor(t).token
	expired.Clock.Advance(31 * 24 * time.Hour)

	tests := []struct {
		name    string
		ts      *testServer
		header  http.Header
		token   string
		invalid bool
	}{
		{"expired session", expired, bearer(expiredToken), expiredToken, false},
		{"wrong scheme", ts, http.Header{authorizationHeader: {"Basic " + token}}, token, true},
		{"cookie under bad bearer", ts, valid, token, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := tt.ts.send(t, call{method: http.MethodGet, path: mePath, header: tt.header, invalid: tt.invalid})
			resp.problem(t, http.StatusUnauthorized)
			if strings.Contains(string(resp.Body), tt.token) {
				t.Error("the problem body contains the token")
			}
			for k, vs := range resp.Header {
				if strings.Contains(strings.Join(vs, ","), tt.token) {
					t.Errorf("response header %s contains the token", k)
				}
			}
			if strings.Contains(tt.ts.Log.String(), tt.token) {
				t.Error("the log contains the token")
			}
		})
	}
}

// TestOffContractNotFoundIsAProblem uses a plain client, since the contract cannot declare a 404 here. It checks that authentication does not get
// in front of the mux's own 404.
func TestOffContractNotFoundIsAProblem(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: nopePath, client: http.DefaultClient}).problem(t, http.StatusNotFound)
}

func TestMeShowsTheRole(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, "kit", "another password", store.RoleUser)
	res, err := ts.Auth.Login(t.Context(), "kit", "another password", "test/1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp := ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(res.Token)})
	var u api.User
	if err = json.Unmarshal(resp.Body, &u); err != nil {
		t.Fatalf("decode %q: %v", resp.Body, err)
	}
	if u.Name != "kit" || u.Role != "user" {
		t.Errorf("user = %+v, want kit with role user", u)
	}
}

func TestMeReportsTagEditing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		tagWrites bool
		role      store.Role
		want      bool
	}{
		{"writes on, admin", true, store.RoleAdmin, true},
		{"writes on, user", true, store.RoleUser, false},
		{"writes off, admin", false, store.RoleAdmin, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t, testServerOptions{TagWrites: tt.tagWrites, LibraryRoot: t.TempDir()})
			var me api.Me
			ts.getJSON(t, ts.tokenFor(t, tt.role), mePath, &me)
			if me.CanEditTags != tt.want || me.Role != string(tt.role) {
				t.Errorf("GET /me = %+v, want canEditTags %t", me, tt.want)
			}
		})
	}
}

func TestAdminRoutesRefuseEveryoneElse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		role store.Role // Empty sends no credentials.
		want int
	}{
		{"admin", store.RoleAdmin, http.StatusNoContent},
		{"user", store.RoleUser, http.StatusForbidden},
		{"nobody", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t)
			ran := false
			h := ts.Handle.guarded(&route{admin: true, handle: func(w http.ResponseWriter, _ *http.Request) {
				ran = true
				w.WriteHeader(http.StatusNoContent)
			}})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/", http.NoBody)
			if tt.role != "" {
				req.Header.Set(authorizationHeader, "Bearer "+ts.tokenFor(t, tt.role))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want || ran != (tt.want == http.StatusNoContent) {
				t.Errorf("status = %d, ran = %t, want %d", rec.Code, ran, tt.want)
			}
		})
	}
}

func TestRequireAdminOutsideAuthenticateRefuses(t *testing.T) {
	t.Parallel()
	h := requireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran without a caller")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/", http.NoBody))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
