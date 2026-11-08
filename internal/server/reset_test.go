package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
)

const (
	resetPath   = "/auth/reset"
	confirmPath = "/auth/reset/confirm"
)

// loggedCode matches the reset code in the server's text log.
var loggedCode = regexp.MustCompile(`resetCode=([0-9a-z-]+)`)

// jsonCall is a POST of body, as JSON, to path from client IP ip ("" for loopback).
func jsonCall(t *testing.T, path, ip string, body any) call {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{contentType: {jsonType}}
	if ip != "" {
		h.Set(testClientIPHeader, ip)
	}
	return call{method: http.MethodPost, path: path, body: string(b), header: h}
}

// requestReset asks for a code for name and checks the 202.
func (ts *testServer) requestReset(t *testing.T, name string) response {
	t.Helper()
	resp := ts.send(t, jsonCall(t, resetPath, "", api.ResetRequest{Name: name}))
	if resp.Status != http.StatusAccepted || len(resp.Body) != 0 {
		t.Fatalf("POST /auth/reset = %d %q, want an empty 202", resp.Status, resp.Body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	return resp
}

// codes returns every reset code the log holds, oldest first.
func (ts *testServer) codes() []string {
	matches := loggedCode.FindAllStringSubmatch(ts.Log.String(), -1)
	codes := make([]string, 0, len(matches))
	for _, m := range matches {
		codes = append(codes, m[1])
	}
	return codes
}

// lastCode returns the newest reset code in the log.
func (ts *testServer) lastCode(t *testing.T) string {
	t.Helper()
	codes := ts.codes()
	if len(codes) == 0 {
		t.Fatalf("no reset code in the log:\n%s", ts.Log.String())
	}
	return codes[len(codes)-1]
}

// confirm sends POST /auth/reset/confirm for simon with code and the new password.
func (ts *testServer) confirm(t *testing.T, code string) response {
	t.Helper()
	return ts.send(t, jsonCall(t, confirmPath, "", api.ResetConfirm{Name: simonName, Code: code, New: newPassword}))
}

// loggedOnce fails the test unless code is in exactly one log line, a WARN
// that names simon.
func (ts *testServer) loggedOnce(t *testing.T, code string) {
	t.Helper()
	var lines []string
	for l := range strings.SplitSeq(ts.Log.String(), "\n") {
		if strings.Contains(l, code) {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "level=WARN") || !strings.Contains(lines[0], "user="+simonName) {
		t.Errorf("lines with the code = %q, want one WARN naming simon", lines)
	}
}

func TestResetPassword(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	old := ts.sessionFor(t)

	resp := ts.requestReset(t, "SIMON")

	code := ts.lastCode(t)
	noSecretInResponse(t, resp, code) // The log has it, on purpose.
	ts.loggedOnce(t, code)

	client := jarClient(t)
	confirm := jsonCall(t, confirmPath, "", api.ResetConfirm{Name: simonName, Code: strings.ToUpper(strings.ReplaceAll(code, "-", "")), New: newPassword, Cookie: true})
	resp = ts.send(t, withClient(confirm, client))

	s := decodeSession(t, resp)
	if s.Token != "" || s.User.Name != simonName {
		t.Errorf("session = %+v, want simon and no token", s)
	}
	sessionCookieOf(t, resp)
	if me := ts.send(t, call{method: http.MethodGet, path: mePath, client: client}); me.Status != http.StatusOK {
		t.Errorf("GET /me with the new session = %d, want 200", me.Status)
	}
	if ts.works(t, old.token) {
		t.Error("a session from before the reset survived it")
	}
	if ts.signsIn(t, testPassword) || !ts.signsIn(t, newPassword) {
		t.Error("the old password still works, or the new one doesn't")
	}
	ts.loggedOnce(t, code)
	noSecretInResponse(t, resp, code)
	ts.noSecretIn(t, resp, newPassword, testPassword)

	again := ts.confirm(t, code)
	again.problem(t, http.StatusForbidden)
}

func TestResetUnknownNameLooksTheSame(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, "user")
	known, unknown := ts.requestReset(t, simonName), ts.requestReset(t, "no-one")
	for _, h := range []string{"Cache-Control", contentType, "Content-Length"} {
		if known.Header.Get(h) != unknown.Header.Get(h) {
			t.Errorf("%s: %q for a real name, %q for an unknown one", h, known.Header.Get(h), unknown.Header.Get(h))
		}
	}
	if n := len(ts.codes()); n != 1 {
		t.Errorf("logged %d codes, want 1, for the real name only", n)
	}
	if strings.Contains(ts.Log.String(), "no-one") {
		t.Error("the log names the unknown name")
	}
	ts.send(t, jsonCall(t, confirmPath, "", api.ResetConfirm{Name: "no-one", Code: ts.lastCode(t), New: newPassword})).
		problem(t, http.StatusForbidden)
}

func TestResetNewCodeReplacesTheOld(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, "user")
	ts.requestReset(t, simonName)
	first := ts.lastCode(t)
	ts.requestReset(t, simonName)
	second := ts.lastCode(t)
	if first == second {
		t.Fatal("the second request logged the same code")
	}
	if p := ts.confirm(t, first).problem(t, http.StatusForbidden); p.Detail != auth.ErrInvalidResetCode.Error() {
		t.Errorf("detail = %q", p.Detail)
	}
	if s := decodeSession(t, ts.confirm(t, second)); s.Token == "" {
		t.Error("a token reset gave no token")
	}
}

func TestResetRefusals(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, "user")
	ts.requestReset(t, simonName)
	code := ts.lastCode(t)
	tests := []struct {
		name    string
		call    call
		status  int
		detail  string
		invalid bool
	}{
		{name: "request without a name", call: call{method: http.MethodPost, path: resetPath, body: `{}`, header: http.Header{contentType: {jsonType}}}, status: http.StatusBadRequest, detail: badResetDetail, invalid: true},
		{name: "request as a form", call: call{method: http.MethodPost, path: resetPath, body: "name=simon", header: http.Header{contentType: {"application/x-www-form-urlencoded"}}}, status: http.StatusBadRequest, detail: badResetDetail, invalid: true},
		{name: "confirm without a code", call: call{method: http.MethodPost, path: confirmPath, body: `{"name":"simon","new":"` + newPassword + `"}`, header: http.Header{contentType: {jsonType}}}, status: http.StatusBadRequest, detail: badResetConfirmDetail, invalid: true},
		{name: "confirm a short password", call: jsonCall(t, confirmPath, "", api.ResetConfirm{Name: simonName, Code: code, New: "2short"}), status: http.StatusBadRequest, detail: auth.ErrPasswordTooShort.Error()},
		{name: "confirm a wrong code", call: jsonCall(t, confirmPath, "", api.ResetConfirm{Name: simonName, Code: "0000-0000-0000-0000", New: newPassword}), status: http.StatusForbidden},
	}
	for _, tt := range tests { // In turn: the code must outlive them all.
		c := tt.call
		c.invalid = tt.invalid
		resp := ts.send(t, c)
		p := resp.problem(t, tt.status)
		if tt.detail != "" && p.Detail != tt.detail {
			t.Errorf("%s: detail = %q, want %q", tt.name, p.Detail, tt.detail)
		}
		ts.noSecretIn(t, resp, newPassword)
	}
	// None of it used the code up: a short password is refused before the code is.
	decodeSession(t, ts.confirm(t, code))
}

func TestResetRequestsRateLimited(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, "user")
	for i := range 6 {
		ts.send(t, jsonCall(t, resetPath, fmt.Sprintf("192.0.2.%d", i+1), api.ResetRequest{Name: simonName}))
	}
	resp := ts.send(t, jsonCall(t, resetPath, "192.0.2.50", api.ResetRequest{Name: simonName}))
	resp.problem(t, http.StatusTooManyRequests)
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
	if n := len(ts.codes()); n != 6 {
		t.Errorf("logged %d codes, want 6: none while blocked", n)
	}
	// Asking for codes never locks the account out of signing in.
	decodeSession(t, ts.send(t, loginCall(t, "192.0.2.50", api.LoginRequest{Name: simonName, Password: testPassword})))
	// Per IP too: one address asking for many names.
	for i := range 6 {
		ts.send(t, jsonCall(t, resetPath, "198.51.100.7", api.ResetRequest{Name: fmt.Sprintf("name%d", i)}))
	}
	ts.send(t, jsonCall(t, resetPath, "198.51.100.7", api.ResetRequest{Name: "fresh"})).problem(t, http.StatusTooManyRequests)
}

func TestResetWrongCodesCountAgainstLogin(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedUser(t, simonName, testPassword, "user")
	ts.requestReset(t, simonName)
	for i := range 6 {
		ts.send(t, jsonCall(t, confirmPath, fmt.Sprintf("192.0.2.%d", i+1), api.ResetConfirm{Name: simonName, Code: "0000-0000-0000-0000", New: newPassword})).
			problem(t, http.StatusForbidden)
	}
	resp := ts.confirm(t, ts.lastCode(t))
	resp.problem(t, http.StatusTooManyRequests) // Blocked before the right code is even compared.
	ts.send(t, loginCall(t, "192.0.2.60", api.LoginRequest{Name: simonName, Password: testPassword})).problem(t, http.StatusTooManyRequests)
}
