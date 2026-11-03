package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	loginPath    = "/auth/login"
	logoutPath   = "/auth/logout"
	jsonType     = "application/json"
	wrongPass    = "hunter2hunter2"
	invalidLogin = "invalid name or password"
)

// loginCall builds a login request from client IP ip.
func loginCall(t *testing.T, ip string, req api.LoginRequest) call {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	h := http.Header{"Content-Type": {jsonType}}
	if ip != "" {
		h.Set(testClientIPHeader, ip)
	}
	return call{method: http.MethodPost, path: loginPath, body: string(body), header: h}
}

// decodeSession decodes a 200 login response.
func decodeSession(t *testing.T, resp response) api.Session {
	t.Helper()
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
	var s api.Session
	if err := json.Unmarshal(resp.Body, &s); err != nil {
		t.Fatalf("decode session %q: %v", resp.Body, err)
	}
	return s
}

// sessionCookieOf returns the one session cookie a response sets.
func sessionCookieOf(t *testing.T, resp response) *http.Cookie {
	t.Helper()
	values := resp.Header.Values("Set-Cookie")
	if len(values) != 1 {
		t.Fatalf("Set-Cookie = %q, want exactly one", values)
	}
	c, err := http.ParseSetCookie(values[0])
	if err != nil {
		t.Fatalf("parse Set-Cookie %q: %v", values[0], err)
	}
	if c.Name != sessionCookie {
		t.Fatalf("cookie name = %q, want %q", c.Name, sessionCookie)
	}
	return c
}

// jarClient returns a contract client that keeps cookies.
func jarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Transport: contract.Client(t).Transport, Jar: jar}
}

func TestCookieUnsafeMethodNeedsHeader(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	client := jarClient(t)
	decodeSession(t, ts.send(t, withClient(loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword, Cookie: true}), client)))

	resp := ts.send(t, call{method: http.MethodPost, path: logoutPath, client: client})
	if p := resp.problem(t, http.StatusForbidden); !strings.Contains(p.Detail, requestHeader) {
		t.Errorf("detail = %q, want it to name %s", p.Detail, requestHeader)
	}
	if me := ts.send(t, call{method: http.MethodGet, path: mePath, client: client}); me.Status != http.StatusOK {
		t.Errorf("GET /me after the refused logout = %d, want 200: the session must survive", me.Status)
	}
}

func withClient(c call, client *http.Client) call {
	c.client = client
	return c
}

func TestLoginToken(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)

	resp := ts.send(t, loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword}))
	s := decodeSession(t, resp)
	if !regexp.MustCompile(`^kss_`).MatchString(s.Token) {
		t.Errorf("token = %q, want a kss_ token", s.Token)
	}
	if want := ts.Clock.Now().Add(30 * 24 * time.Hour); !s.ExpiresAt.Equal(want) {
		t.Errorf("expiresAt = %v, want %v", s.ExpiresAt, want)
	}
	if s.User.Name != simonName {
		t.Errorf("user.name = %q, want simon", s.User.Name)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if v := resp.Header.Values("Set-Cookie"); len(v) != 0 {
		t.Errorf("Set-Cookie = %q, want none for a token login", v)
	}
	if me := ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(s.Token)}); me.Status != http.StatusOK {
		t.Errorf("GET /me with the token = %d, want 200", me.Status)
	}
}

func TestLoginCookie(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	client := jarClient(t)

	resp := ts.send(t, withClient(loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword, Cookie: true}), client))
	decodeSession(t, resp)
	var raw map[string]any
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, has := raw["token"]; has {
		t.Error("the body has a token member for a cookie login")
	}
	c := sessionCookieOf(t, resp)
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Secure {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Lax, Path=/ and not Secure", c)
	}
	if want := int(auth.SessionMaxAge.Seconds()); c.MaxAge != want {
		t.Errorf("Max-Age = %d, want %d", c.MaxAge, want)
	}
	if me := ts.send(t, call{method: http.MethodGet, path: mePath, client: client}); me.Status != http.StatusOK {
		t.Errorf("GET /me with the cookie = %d, want 200", me.Status)
	}
}

func TestLoginCookieSecure(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{SecureCookies: true})
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	resp := ts.send(t, loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword, Cookie: true}))
	if c := sessionCookieOf(t, resp); !c.Secure {
		t.Errorf("cookie = %+v, want Secure", c)
	}
}

func TestLoginFailures(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)

	wrong := ts.send(t, loginCall(t, "192.0.2.1", api.LoginRequest{Name: simonName, Password: wrongPass}))
	unknown := ts.send(t, loginCall(t, "192.0.2.2", api.LoginRequest{Name: "nobody", Password: wrongPass}))
	pw, pu := wrong.problem(t, http.StatusUnauthorized), unknown.problem(t, http.StatusUnauthorized)
	if pw.Detail != invalidLogin {
		t.Errorf("detail = %q, want %q", pw.Detail, invalidLogin)
	}
	pw.RequestID, pu.RequestID = "", ""
	if pw != pu {
		t.Errorf("wrong password %+v and unknown name %+v differ", pw, pu)
	}
	for name, got := range map[string]string{"wrong body": string(wrong.Body), "unknown body": string(unknown.Body), "log": ts.Log.String()} {
		if strings.Contains(got, wrongPass) {
			t.Errorf("the %s contains the password", name)
		}
	}
}

func TestLoginBadRequests(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	valid := `{"name":"a","password":"` + wrongPass + `"}`
	tests := []struct {
		name        string
		body        string
		contentType string
		invalid     bool // Breaks the contract; otherwise only the server's limits refuse it.
	}{
		{"not JSON", "nope " + wrongPass, jsonType, true},
		{"unknown member", `{"name":"a","password":"` + wrongPass + `","extra":1}`, jsonType, true},
		{"missing password", `{"name":"a"}`, jsonType, true},
		{"empty name", `{"name":"","password":"` + wrongPass + `"}`, jsonType, true},
		{"text/plain", valid, "text/plain", true},
		{"too large", `{"name":"a","password":"` + strings.Repeat("x", 5<<10) + `"}`, jsonType, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := ts.send(t, call{
				method: http.MethodPost, path: loginPath, body: tt.body,
				header:  http.Header{"Content-Type": {tt.contentType}},
				invalid: tt.invalid,
			})
			p := resp.problem(t, http.StatusBadRequest)
			if p.Detail != badLoginDetail {
				t.Errorf("detail = %q, want %q", p.Detail, badLoginDetail)
			}
			if strings.Contains(string(resp.Body), wrongPass) || strings.Contains(ts.Log.String(), wrongPass) {
				t.Error("the response or the log contains the password")
			}
		})
	}
}

func TestLoginRateLimitedByName(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	for i := range 6 {
		resp := ts.send(t, loginCall(t, fmt.Sprintf("192.0.2.%d", i+1), api.LoginRequest{Name: simonName, Password: wrongPass}))
		resp.problem(t, http.StatusUnauthorized)
	}

	good := loginCall(t, "192.0.2.50", api.LoginRequest{Name: simonName, Password: testPassword})
	resp := ts.send(t, good)
	resp.problem(t, http.StatusTooManyRequests)
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}

	ts.Clock.Advance(time.Second)
	decodeSession(t, ts.send(t, good))
	bad := loginCall(t, "192.0.2.51", api.LoginRequest{Name: simonName, Password: wrongPass})
	ts.send(t, bad).problem(t, http.StatusUnauthorized) // The success reset the name key.
}

func TestLoginRateLimitedByIP(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	for i := range 6 {
		ts.send(t, loginCall(t, "192.0.2.1", api.LoginRequest{Name: fmt.Sprintf("user%d", i), Password: wrongPass})).
			problem(t, http.StatusUnauthorized)
	}
	ts.send(t, loginCall(t, "192.0.2.1", api.LoginRequest{Name: "never-tried", Password: wrongPass})).
		problem(t, http.StatusTooManyRequests)
	decodeSession(t, ts.send(t, loginCall(t, "192.0.2.2", api.LoginRequest{Name: simonName, Password: testPassword})))
}

func TestLoginSuccessDoesNotResetIP(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	const ip = "192.0.2.1"
	for i := range 5 {
		ts.send(t, loginCall(t, ip, api.LoginRequest{Name: fmt.Sprintf("user%d", i), Password: wrongPass})).
			problem(t, http.StatusUnauthorized)
	}
	decodeSession(t, ts.send(t, loginCall(t, ip, api.LoginRequest{Name: simonName, Password: testPassword})))
	ts.send(t, loginCall(t, ip, api.LoginRequest{Name: "user9", Password: wrongPass})) // Refused or not, it must not reset anything.
	ts.send(t, loginCall(t, ip, api.LoginRequest{Name: "user10", Password: wrongPass})).
		problem(t, http.StatusTooManyRequests)
}

func TestLoginParallelAttemptsAreCounted(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	client := contract.Client(t)

	const attempts = 20
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		statuses = make(map[int]int)
	)
	for i := range attempts {
		wg.Go(func() {
			c := withClient(loginCall(t, fmt.Sprintf("192.0.2.%d", i+1), api.LoginRequest{Name: simonName, Password: wrongPass}), client)
			resp, err := ts.trySend(t, c)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			statuses[resp.Status]++
			mu.Unlock()
		})
	}
	wg.Wait()
	if statuses[http.StatusUnauthorized] > 6 || statuses[http.StatusUnauthorized]+statuses[http.StatusTooManyRequests] != attempts {
		t.Errorf("statuses = %v, want at most 6 x 401 and the rest 429", statuses)
	}
}

// TestLoginRateLimitDoesNoHashingWhileBlocked proves a blocked client costs
// no hash: a refused correct-password attempt must be far quicker than the
// fastest real attempt. The bound is relative, so -race and slow CI don't
// matter.
func TestLoginRateLimitDoesNoHashingWhileBlocked(t *testing.T) { //nolint:paralleltest // Timing; runs alone.
	ts := newTestServer(t, testServerOptions{Params: auth.DefaultParams()})
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)

	fastest := time.Duration(1<<63 - 1)
	for i := range 6 {
		start := time.Now()
		ts.send(t, loginCall(t, fmt.Sprintf("192.0.2.%d", i+1), api.LoginRequest{Name: simonName, Password: wrongPass})).
			problem(t, http.StatusUnauthorized)
		fastest = min(fastest, time.Since(start))
	}
	start := time.Now()
	ts.send(t, loginCall(t, "192.0.2.50", api.LoginRequest{Name: simonName, Password: testPassword})).
		problem(t, http.StatusTooManyRequests)
	if blocked := time.Since(start); blocked >= fastest/4 {
		t.Errorf("a blocked attempt took %v; the fastest hashing attempt took %v; want under a quarter", blocked, fastest)
	}
}

// cookieSession logs simon in with a cookie and returns the cookie's token.
func cookieSession(t *testing.T, ts *testServer) string {
	t.Helper()
	ts.seedUser(t, simonName, testPassword, store.RoleAdmin)
	resp := ts.send(t, loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword, Cookie: true}))
	decodeSession(t, resp)
	return sessionCookieOf(t, resp).Value
}

func TestLogout(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := cookieSession(t, ts)

	h := cookieOnly(token)
	h.Set(requestHeader, "1")
	resp := ts.send(t, call{method: http.MethodPost, path: logoutPath, header: h})
	if resp.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.Status, resp.Body)
	}
	if c := sessionCookieOf(t, resp); c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("cookie = %+v, want it cleared with a negative Max-Age", c)
	}
	ts.send(t, call{method: http.MethodGet, path: mePath, header: cookieOnly(token)}).
		problem(t, http.StatusUnauthorized)
}

func TestLogoutBearer(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	if resp := ts.send(t, call{method: http.MethodPost, path: logoutPath, header: bearer(token)}); resp.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.Status, resp.Body)
	}
	ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(token)}).
		problem(t, http.StatusUnauthorized)
}

func TestLogoutUnauthenticated(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodPost, path: logoutPath, invalid: true}).
		problem(t, http.StatusUnauthorized)
}
