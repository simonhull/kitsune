package server

import (
	"encoding/json/v2"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	setupPath = "/setup"
	// setupCode is the code the setup tests' servers start with.
	setupCode = "k7m2-q9xd-4tbn-r8ve"
	// publicIP is a client outside the server's network (TEST-NET-3).
	publicIP = "203.0.113.50"
	noStore  = "no-store"
)

// newSetupServer starts a server with no accounts and setupCode.
func newSetupServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServer(t, testServerOptions{SetupCode: setupCode})
}

// setupCall builds a setup request from client IP ip; "" means the
// connection's own, loopback address.
func setupCall(t *testing.T, ip string, req api.SetupRequest) call {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal setup: %v", err)
	}
	h := http.Header{contentType: {jsonType}}
	if ip != "" {
		h.Set(testClientIPHeader, ip)
	}
	return call{method: http.MethodPost, path: setupPath, body: string(body), header: h}
}

// simonSetup is a valid setup request for simon with code.
func simonSetup(code string) api.SetupRequest {
	return api.SetupRequest{Name: simonName, Password: testPassword, Code: code}
}

// status gets GET /setup from client IP ip.
func (ts *testServer) setupStatus(t *testing.T, ip string) api.SetupStatus {
	t.Helper()
	h := http.Header{}
	if ip != "" {
		h.Set(testClientIPHeader, ip)
	}
	resp := ts.send(t, call{method: http.MethodGet, path: setupPath, header: h})
	if resp.Status != http.StatusOK {
		t.Fatalf("GET /setup = %d, want 200: %s", resp.Status, resp.Body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("GET /setup Cache-Control = %q, want no-store", cc)
	}
	noSetupCode(t, resp)
	var st api.SetupStatus
	if err := json.Unmarshal(resp.Body, &st); err != nil {
		t.Fatalf("decode setup status %q: %v", resp.Body, err)
	}
	return st
}

// noSetupCode fails the test if resp carries the setup code anywhere, in any
// of the spellings it is accepted in.
func noSetupCode(t *testing.T, resp response) {
	t.Helper()
	spellings := []string{setupCode, auth.NormalizeCode(setupCode), strings.ToUpper(setupCode)}
	var headers strings.Builder
	for k, vs := range resp.Header {
		headers.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	for _, s := range spellings {
		if strings.Contains(string(resp.Body), s) || strings.Contains(headers.String(), s) {
			t.Errorf("a response carries the setup code: %s %s", headers.String(), resp.Body)
		}
	}
}

// sendSetup sends c, checks that the answer has status, no-store and no
// setup code, and returns it.
func (ts *testServer) sendSetup(t *testing.T, c call, status int) response {
	t.Helper()
	resp := ts.send(t, c)
	if resp.Status != status {
		t.Errorf("POST /setup = %d, want %d: %s", resp.Status, status, resp.Body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("POST /setup Cache-Control = %q, want no-store", cc)
	}
	noSetupCode(t, resp)
	return resp
}

func TestSetupStatus(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	if got := ts.setupStatus(t, ""); got != (api.SetupStatus{Needed: true}) {
		t.Errorf("from loopback = %+v, want needed and no code", got)
	}
	if got := ts.setupStatus(t, "192.168.1.20"); got != (api.SetupStatus{Needed: true}) {
		t.Errorf("from the home network = %+v, want needed and no code", got)
	}
	if got := ts.setupStatus(t, publicIP); got != (api.SetupStatus{Needed: true, CodeRequired: true}) {
		t.Errorf("from outside = %+v, want needed and the code", got)
	}
}

func TestSetupStatusStartedWithAccounts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t) // No code: the server started with accounts.
	if got := ts.setupStatus(t, publicIP); got != (api.SetupStatus{}) {
		t.Errorf("status = %+v, want not needed", got)
	}
	ts.sendSetup(t, setupCall(t, "", simonSetup("")), http.StatusConflict)
	if n, err := ts.Store.CountUsers(t.Context()); err != nil || n != 0 {
		t.Errorf("CountUsers = %d, %v; want 0", n, err)
	}
}

func TestSetupFromTheServersNetwork(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	client := jarClient(t)
	resp := ts.sendSetup(t, withClient(setupCall(t, "", simonSetup("")), client), http.StatusOK)
	s := decodeSession(t, resp)
	if s.Token != "" || s.User.Name != simonName || s.User.Role != string(store.RoleAdmin) {
		t.Errorf("session = %+v, want admin simon and no token", s)
	}
	if c := sessionCookieOf(t, resp); !c.HttpOnly || c.Path != "/" {
		t.Errorf("cookie = %+v, want HttpOnly at /", c)
	}
	if me := ts.send(t, call{method: http.MethodGet, path: mePath, client: client}); me.Status != http.StatusOK {
		t.Errorf("GET /me with the setup cookie = %d, want 200", me.Status)
	}
	ts.checkSetupClosed(t)
}

// checkSetupClosed checks that setup, once done, stays closed.
func (ts *testServer) checkSetupClosed(t *testing.T) {
	t.Helper()
	if got := ts.setupStatus(t, ""); got.Needed {
		t.Errorf("status after setup = %+v, want not needed", got)
	}
	again := ts.sendSetup(t, setupCall(t, "", api.SetupRequest{Name: "mallory", Password: testPassword}), http.StatusConflict)
	if p := again.problem(t, http.StatusConflict); p.Detail != setupDoneDetail {
		t.Errorf("detail = %q, want %q", p.Detail, setupDoneDetail)
	}
	if n, err := ts.Store.CountUsers(t.Context()); err != nil || n != 1 {
		t.Errorf("CountUsers = %d, %v; want 1", n, err)
	}
}

func TestSetupFromOutside(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	ts.sendSetup(t, setupCall(t, publicIP, simonSetup("")), http.StatusForbidden).problem(t, http.StatusForbidden)
	ts.sendSetup(t, setupCall(t, publicIP, simonSetup("k7m2-q9xd-4tbn-r8vf")), http.StatusForbidden).problem(t, http.StatusForbidden)
	if n, err := ts.Store.CountUsers(t.Context()); err != nil || n != 0 {
		t.Fatalf("CountUsers after refusals = %d, %v; want 0", n, err)
	}
	// Any case and grouping: the code is normalized before it is compared.
	ts.sendSetup(t, setupCall(t, publicIP, simonSetup(" K7M2 Q9XD-4tbn r8ve")), http.StatusOK)
	if got := ts.setupStatus(t, publicIP); got != (api.SetupStatus{}) {
		t.Errorf("status after setup = %+v, want not needed", got)
	}
	if strings.Contains(ts.Log.String(), auth.NormalizeCode(setupCode)) || strings.Contains(ts.Log.String(), setupCode) {
		t.Error("the server logged the setup code")
	}
}

func TestSetupThrottlesWrongCodes(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	wrong := simonSetup("0000-0000-0000-0000")
	// The login limiter's rule: 5 free failures, and the one after them starts
	// the wait.
	for i := range maxFreeFailures + 1 {
		if got := ts.send(t, setupCall(t, publicIP, wrong)); got.Status != http.StatusForbidden {
			t.Fatalf("wrong code %d = %d, want 403", i+1, got.Status)
		}
	}
	resp := ts.sendSetup(t, setupCall(t, publicIP, simonSetup(setupCode)), http.StatusTooManyRequests)
	resp.problem(t, http.StatusTooManyRequests)
	if ra := resp.Header.Get("Retry-After"); ra != "1" {
		t.Errorf("Retry-After = %q, want 1", ra)
	}
	// Another address, and the server's own network, are not held back.
	ts.sendSetup(t, setupCall(t, "203.0.113.51", wrong), http.StatusForbidden)
	ts.sendSetup(t, setupCall(t, "", simonSetup("")), http.StatusOK)
}

func TestSetupRightCodeIsNotCounted(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	short := api.SetupRequest{Name: simonName, Password: "short", Code: setupCode}
	for i := range maxFreeFailures + 2 {
		if got := ts.send(t, setupCall(t, publicIP, short)); got.Status != http.StatusBadRequest {
			t.Fatalf("short password %d = %d, want 400", i+1, got.Status)
		}
	}
	ts.sendSetup(t, setupCall(t, publicIP, simonSetup(setupCode)), http.StatusOK)
}

func TestSetupBadRequests(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	tests := []struct {
		name   string
		c      call
		detail string
	}{
		{name: "short password", c: setupCall(t, "", api.SetupRequest{Name: simonName, Password: "short"}), detail: "at least 8 characters"},
		{name: "bad name", c: setupCall(t, "", api.SetupRequest{Name: " simon", Password: testPassword}), detail: "invalid user name"},
		{name: "long password", c: setupCall(t, "", api.SetupRequest{Name: simonName, Password: strings.Repeat("p", 1025)}), detail: "too long"},
		{name: "plain text", c: call{method: http.MethodPost, path: setupPath, body: `{"name":"a","password":"b"}`, header: http.Header{contentType: {"text/plain; charset=utf-8"}}, invalid: true}, detail: badSetupDetail},
		{name: "unknown member", c: call{method: http.MethodPost, path: setupPath, body: `{"name":"a","password":"b","admin":true}`, header: http.Header{contentType: {jsonType}}, invalid: true}, detail: badSetupDetail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := ts.sendSetup(t, tt.c, http.StatusBadRequest).problem(t, http.StatusBadRequest)
			if !strings.Contains(p.Detail, tt.detail) || strings.Contains(p.Detail, testPassword) {
				t.Errorf("detail = %q, want it to contain %q and no password", p.Detail, tt.detail)
			}
		})
	}
}

func TestSetupRace(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	names := []string{"simon", "ada"}
	statuses := make(chan int, len(names))
	for _, name := range names {
		go func() {
			resp, err := ts.trySend(t, setupCall(t, "", api.SetupRequest{Name: name, Password: testPassword}))
			if err != nil {
				t.Error(err)
			}
			statuses <- resp.Status
		}()
	}
	got := map[int]int{}
	for range names {
		got[<-statuses]++
	}
	if got[http.StatusOK] != 1 || got[http.StatusConflict] != 1 {
		t.Errorf("statuses = %v, want one 200 and one 409", got)
	}
	if n, err := ts.Store.CountUsers(t.Context()); err != nil || n != 1 {
		t.Errorf("CountUsers = %d, %v; want 1", n, err)
	}
}

func TestSetupClosedByUsersAdd(t *testing.T) {
	t.Parallel()
	ts := newSetupServer(t)
	if got := ts.setupStatus(t, ""); !got.Needed {
		t.Fatalf("status = %+v, want needed", got)
	}
	// What users add does: a user straight into the store, while the server runs.
	ts.seedUser(t, "ada", testPassword, store.RoleAdmin)
	if got := ts.setupStatus(t, publicIP); got != (api.SetupStatus{}) {
		t.Errorf("status = %+v, want not needed", got)
	}
	ts.sendSetup(t, setupCall(t, publicIP, simonSetup(setupCode)), http.StatusConflict)
	ts.sendSetup(t, setupCall(t, "", simonSetup("")), http.StatusConflict)
}

func TestIsLocal(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"127.0.0.1":                true,
		"127.255.255.254":          true,
		"::1":                      true,
		"10.0.0.0":                 true,
		"10.255.255.255":           true,
		"9.255.255.255":            false,
		"11.0.0.0":                 false,
		"172.16.0.0":               true,
		"172.31.255.255":           true,
		"172.15.255.255":           false,
		"172.32.0.0":               false,
		"192.168.0.0":              true,
		"192.168.255.255":          true,
		"192.167.255.255":          false,
		"192.169.0.0":              false,
		"169.254.0.1":              true,
		"169.254.255.254":          true,
		"169.253.255.255":          false,
		"169.255.0.0":              false,
		"100.64.0.0":               true,
		"100.127.255.255":          true,
		"100.63.255.255":           false,
		"100.128.0.0":              false,
		"fc00::":                   true,
		"fdff:ffff:ffff:ffff::1":   true,
		"fbff:ffff:ffff:ffff::1":   false,
		"fe00::":                   false,
		"fe80::1":                  true,
		"fe80::1%eth0":             true,
		"febf:ffff::1":             true,
		"fec0::1":                  false,
		"::ffff:192.168.1.2":       true,
		"::ffff:100.100.1.1":       true,
		"::ffff:8.8.8.8":           false,
		"8.8.8.8":                  false,
		"203.0.113.50":             false,
		"0.0.0.0":                  false,
		"::":                       false,
		"2001:db8::1":              false,
		"2606:4700:4700::1111":     false,
		"64:ff9b::c0a8:102":        false, // NAT64 of 192.168.1.2: public on the wire.
		"::ffff:7f00:1":            true,
		"2002:c0a8:102::1":         false, // 6to4 of 192.168.1.2.
		"fd7a:115c:a1e0:ab12::1":   true,  // Tailscale's IPv6 range.
		"100.101.102.103":          true,  // A Tailscale IPv4 address.
		"224.0.0.1":                false, // Multicast.
		"ff02::1":                  false,
		"255.255.255.255":          false,
		"192.0.2.1":                false, // TEST-NET-1.
		"198.18.0.1":               false, // Benchmarking.
		"::ffff:169.254.169.254":   true,
		"fe80::1ff:fe23:4567:890a": true,
	}
	for in, want := range tests {
		if got := isLocal(netip.MustParseAddr(in)); got != want {
			t.Errorf("isLocal(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestCallerIsLocalThroughProxies(t *testing.T) {
	t.Parallel()
	trusted := prefixes(t, dockerNet)
	tests := []struct {
		name    string
		trusted []netip.Prefix
		remote  string
		xff     string
		want    bool
	}{
		{name: "loopback peer", remote: "127.0.0.1:5000", want: true},
		{name: "public peer", remote: "203.0.113.9:5000", want: false},
		{name: "IPv6 public peer", remote: "[2001:db8::1]:5000", want: false},
		{name: "public peer claims a private address", remote: "203.0.113.9:5000", xff: "192.168.1.2", want: false},
		{name: "untrusted private peer forwards a public address", remote: "192.168.1.9:5000", xff: "203.0.113.9", want: true},
		{name: "trusted proxy forwards a public address", trusted: trusted, remote: proxyPeer, xff: "203.0.113.9", want: false},
		{name: "trusted proxy forwards a private address", trusted: trusted, remote: proxyPeer, xff: "192.168.1.2", want: true},
		{name: "trusted proxy forwards a spoofed private hop", trusted: trusted, remote: proxyPeer, xff: "192.168.1.2, 203.0.113.9", want: false},
		{name: "trusted proxy forwards garbage", trusted: trusted, remote: proxyPeer, xff: "not-an-address", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Server{clientIP: ForwardedClientIP(tt.trusted)}
			var xff []string
			if tt.xff != "" {
				xff = []string{tt.xff}
			}
			if got := s.callerIsLocal(forwardedRequest(t, tt.remote, xff...)); got != tt.want {
				t.Errorf("callerIsLocal = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetupCodeMatches(t *testing.T) {
	t.Parallel()
	s := newSetupState(setupCode)
	tests := map[string]bool{
		setupCode:              true,
		"K7M2-Q9XD-4TBN-R8VE":  true,
		"k7m2q9xd4tbnr8ve":     true,
		"k7m2 q9xd 4tbn r8ve":  true,
		"k7m2-q9xd-4tbn-r8v":   false,
		"k7m2-q9xd-4tbn-r8vee": false,
		"":                     false,
		"----":                 false,
	}
	for in, want := range tests {
		if got := s.matches(in); got != want {
			t.Errorf("matches(%q) = %v, want %v", in, got, want)
		}
	}
	withLookalikes := newSetupState("0a1b-0c1d-2e3f-4g5h")
	if !withLookalikes.matches("OA1B-ocLd-2e3f-4g5h") || withLookalikes.matches("ua1b-0c1d-2e3f-4g5h") {
		t.Error("o must read as 0 and i and l as 1, and nothing else may stand in")
	}
	if done := newSetupState(""); !done.done.Load() || done.matches("") {
		t.Error("with no code, setup must be done and nothing must match")
	}
}
