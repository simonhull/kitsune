package server

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
)

const (
	myPasswordPath = "/me/password"
	newPassword    = "battery staple"
	shortPassword  = "2short"
)

// changeCall is POST /me/password with token and body.
func changeCall(t *testing.T, token string, body api.PasswordChange) call {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return call{method: http.MethodPost, path: myPasswordPath, body: string(b), header: jsonHeader(token)}
}

// cookieJSON is a cookie credential on a JSON body, without X-Kitsune-Request.
func cookieJSON(token string) http.Header {
	h := cookieOnly(token)
	h.Set(contentType, jsonType)
	return h
}

// works reports whether token still signs its holder in.
func (ts *testServer) works(t *testing.T, token string) bool {
	t.Helper()
	_, err := ts.Auth.Authenticate(t.Context(), token)
	if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal(err)
	}
	return err == nil
}

// signsIn reports whether simon logs in with password.
func (ts *testServer) signsIn(t *testing.T, password string) bool {
	t.Helper()
	_, err := ts.Auth.Login(t.Context(), simonName, password, "")
	if err != nil && !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	return err == nil
}

// noSecretIn fails the test if any of secrets is in the response or the log.
func (ts *testServer) noSecretIn(t *testing.T, resp response, secrets ...string) {
	t.Helper()
	noSecretInResponse(t, resp, secrets...)
	for _, s := range secrets {
		if strings.Contains(ts.Log.String(), s) {
			t.Errorf("the log holds %q", s)
		}
	}
}

// noSecretInResponse fails the test if any of secrets is in resp's body or headers.
func noSecretInResponse(t *testing.T, resp response, secrets ...string) {
	t.Helper()
	var all strings.Builder
	all.Write(resp.Body)
	for k, vs := range resp.Header {
		all.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	for _, s := range secrets {
		if strings.Contains(all.String(), s) {
			t.Errorf("the response holds %q", s)
		}
	}
}

func TestChangePassword(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	here := ts.sessionFor(t)
	away, err := ts.Auth.Login(t.Context(), simonName, testPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	resp := ts.send(t, changeCall(t, here.token, api.PasswordChange{Current: testPassword, New: newPassword}))

	if resp.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.Status, resp.Body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if !ts.works(t, here.token) {
		t.Error("the session that changed the password ended")
	}
	if ts.works(t, away.Token) {
		t.Error("another session survived the change")
	}
	if ts.signsIn(t, testPassword) || !ts.signsIn(t, newPassword) {
		t.Error("the old password still works, or the new one doesn't")
	}
	ts.noSecretIn(t, resp, testPassword, newPassword, here.token)
}

func TestChangePasswordRefusals(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	tests := []struct {
		name    string
		call    call
		status  int
		detail  string
		invalid bool
	}{
		{name: "wrong current", call: changeCall(t, sess.token, api.PasswordChange{Current: wrongPass, New: newPassword}), status: http.StatusForbidden, detail: "the current password is wrong"},
		{name: "short new", call: changeCall(t, sess.token, api.PasswordChange{Current: testPassword, New: shortPassword}), status: http.StatusBadRequest, detail: auth.ErrPasswordTooShort.Error()},
		{name: "body not JSON", call: call{method: http.MethodPost, path: myPasswordPath, body: "current=" + testPassword, header: jsonHeader(sess.token)}, status: http.StatusBadRequest, detail: badPasswordChangeDetail, invalid: true},
		{name: "no credentials", call: call{method: http.MethodPost, path: myPasswordPath, body: `{"current":"a","new":"b"}`, header: http.Header{contentType: {jsonType}}}, status: http.StatusUnauthorized, invalid: true},
		{name: "cookie without the request header", call: call{method: http.MethodPost, path: myPasswordPath, body: `{"current":"a","new":"b"}`, header: cookieJSON(sess.token)}, status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.call
			c.invalid = tt.invalid
			resp := ts.send(t, c)
			p := resp.problem(t, tt.status)
			if tt.detail != "" && p.Detail != tt.detail {
				t.Errorf("detail = %q, want %q", p.Detail, tt.detail)
			}
			ts.noSecretIn(t, resp, testPassword, wrongPass, newPassword, sess.token)
		})
	}
	if !ts.signsIn(t, testPassword) || !ts.works(t, sess.token) {
		t.Error("a refused change changed something")
	}
}

func TestChangePasswordRateLimited(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	wrong := changeCall(t, sess.token, api.PasswordChange{Current: wrongPass, New: newPassword})
	for range 6 {
		ts.send(t, wrong).problem(t, http.StatusForbidden)
	}
	right := changeCall(t, sess.token, api.PasswordChange{Current: testPassword, New: newPassword})
	resp := ts.send(t, right)
	resp.problem(t, http.StatusTooManyRequests)
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
	// The guesses counted against the name, so login is blocked too.
	ts.send(t, loginCall(t, "192.0.2.9", api.LoginRequest{Name: simonName, Password: testPassword})).problem(t, http.StatusTooManyRequests)
	if !ts.signsIn(t, testPassword) {
		t.Error("a blocked change changed the password")
	}
}
