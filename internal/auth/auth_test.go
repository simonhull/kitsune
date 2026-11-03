package auth_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	day      = 24 * time.Hour
	simon    = "simon"
	password = "correct horse"
)

func cheapParams() auth.Params { return auth.Params{Time: 1, MemoryKiB: 64, Threads: 1} }

type env struct {
	svc   *auth.Service
	st    *store.Store
	clock *testClock
	dir   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{clock: newTestClock(), dir: t.TempDir()}
	var err error
	e.st, err = store.Open(t.Context(), filepath.Join(e.dir, store.FileName), store.Options{Now: e.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.st.Close() })
	e.svc, err = auth.New(e.st, auth.Options{Params: cheapParams(), Now: e.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) login(t *testing.T) auth.LoginResult {
	t.Helper()
	if _, err := e.svc.CreateUser(t.Context(), simon, password, store.RoleAdmin); err != nil && !errors.Is(err, store.ErrNameTaken) {
		t.Fatal(err)
	}
	res, err := e.svc.Login(t.Context(), simon, password, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sessionOf(t *testing.T, e *env, token string) (store.Session, error) {
	t.Helper()
	sum := sha256.Sum256([]byte(token))
	sess, err := e.st.SessionByTokenHash(t.Context(), sum[:])
	if err != nil {
		return store.Session{}, fmt.Errorf("session lookup: %w", err)
	}
	return sess, nil
}

func TestCreateUserStoresHashNotPassword(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if _, err := e.svc.CreateUser(t.Context(), simon, password, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.UserByName(t.Context(), simon)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u.PasswordHash, "$argon2id$") || strings.Contains(u.PasswordHash, password) {
		t.Errorf("stored hash has the wrong shape")
	}
}

func TestCreateUserValidates(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if _, err := e.svc.CreateUser(t.Context(), " ", password, store.RoleUser); !errors.Is(err, auth.ErrInvalidName) {
		t.Errorf("bad name: err = %v", err)
	}
	if _, err := e.svc.CreateUser(t.Context(), simon, "short", store.RoleUser); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Errorf("short password: err = %v", err)
	}
	if _, err := e.svc.CreateUser(t.Context(), simon, password, store.RoleUser); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateUser(t.Context(), "SIMON", password, store.RoleUser); !errors.Is(err, store.ErrNameTaken) {
		t.Errorf("duplicate: err = %v", err)
	}
}

func TestLoginAndAuthenticate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	if !strings.HasPrefix(res.Token, "kss_") {
		t.Errorf("token %q has no kss_ prefix", res.Token[:min(len(res.Token), 4)])
	}
	if want := e.clock.Now().Add(30 * day); !res.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
	}
	got, err := e.svc.Authenticate(t.Context(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if got != res.User {
		t.Errorf("Authenticate = %+v, Login returned %+v", got, res.User)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.login(t)
	_, wrong := e.svc.Login(t.Context(), simon, "wrong horse", "")
	_, unknown := e.svc.Login(t.Context(), "nobody", password, "")
	if !errors.Is(wrong, auth.ErrInvalidCredentials) || !errors.Is(unknown, auth.ErrInvalidCredentials) {
		t.Fatalf("errors = %v, %v", wrong, unknown)
	}
	if wrong.Error() != unknown.Error() {
		t.Errorf("messages differ: %q vs %q", wrong, unknown)
	}
}

func TestAuthenticateRejectsGarbage(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	valid := "kss_" + strings.Repeat("A", 43)
	for _, tok := range []string{"", "Bearer x", valid} {
		if _, err := e.svc.Authenticate(t.Context(), tok); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("Authenticate(%q) = %v", tok, err)
		}
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	e.clock.Advance(30 * day)
	if _, err := e.svc.Authenticate(t.Context(), res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	if _, err := sessionOf(t, e, res.Token); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expired session still stored: %v", err)
	}
}

func TestSessionTouchIsThrottled(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	login := e.clock.Now()

	e.clock.Advance(30 * time.Minute)
	if _, err := e.svc.Authenticate(t.Context(), res.Token); err != nil {
		t.Fatal(err)
	}
	s, err := sessionOf(t, e, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !s.LastSeen.Equal(login) {
		t.Errorf("LastSeen = %v after 30 minutes, want the login time", s.LastSeen)
	}

	e.clock.Advance(31 * time.Minute)
	if _, err = e.svc.Authenticate(t.Context(), res.Token); err != nil {
		t.Fatal(err)
	}
	if s, err = sessionOf(t, e, res.Token); err != nil {
		t.Fatal(err)
	}
	if now := e.clock.Now(); !s.LastSeen.Equal(now) || !s.ExpiresAt.Equal(now.Add(30*day)) {
		t.Errorf("LastSeen, ExpiresAt = %v, %v; want %v, %v", s.LastSeen, s.ExpiresAt, now, now.Add(30*day))
	}
}

func TestSessionAbsoluteCap(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	created := e.clock.Now()
	for i := 1; i <= 12; i++ {
		e.clock.Advance(29 * day)
		if _, err := e.svc.Authenticate(t.Context(), res.Token); err != nil {
			t.Fatalf("day %d: %v", i*29, err)
		}
	}
	s, err := sessionOf(t, e, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if want := created.Add(365 * day); !s.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt at day 348 = %v, want the cap %v", s.ExpiresAt, want)
	}
	e.clock.Advance(29 * day)
	if _, err = e.svc.Authenticate(t.Context(), res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("past the cap: err = %v, want ErrUnauthenticated", err)
	}
}

func TestLogout(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	if err := e.svc.Logout(t.Context(), res.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(t.Context(), res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("after logout: err = %v", err)
	}
	for _, tok := range []string{res.Token, "garbage"} {
		if err := e.svc.Logout(t.Context(), tok); err != nil {
			t.Errorf("Logout(%q) = %v, want nil", tok, err)
		}
	}
}

func TestPurgeExpiredSessions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.login(t)
	e.login(t)
	e.clock.Advance(31 * day)
	e.login(t)
	n, err := e.svc.PurgeExpiredSessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("purged %d, want 2", n)
	}
}

func TestLoginRehashes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.login(t)
	newer, err := auth.New(e.st, auth.Options{Params: auth.Params{Time: 2, MemoryKiB: 64, Threads: 1}, Now: e.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = newer.Login(t.Context(), simon, password, ""); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.UserByName(t.Context(), simon)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u.PasswordHash, "t=2") {
		t.Error("hash was not upgraded to t=2")
	}
	if _, err = e.svc.Login(t.Context(), simon, password, ""); err != nil {
		t.Errorf("login through the original service: %v", err)
	}
}

func TestNoSecretsAtRest(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	res := e.login(t)
	if err := e.st.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no database files to inspect")
	}
	for _, f := range files {
		data, readErr := os.ReadFile(filepath.Join(e.dir, f.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if bytes.Contains(data, []byte(res.Token)) || bytes.Contains(data, []byte(password)) {
			t.Errorf("%s contains a token or password", f.Name())
		}
	}
}

func TestUserAgentIsBounded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if _, err := e.svc.CreateUser(t.Context(), simon, password, store.RoleUser); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Login(t.Context(), simon, password, strings.Repeat("€", 333)+"\xff")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sessionOf(t, e, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.UserAgent) != 255 || !utf8.ValidString(s.UserAgent) {
		t.Errorf("stored user agent is %d bytes, valid = %v; want 255, true", len(s.UserAgent), utf8.ValidString(s.UserAgent))
	}
}

func TestLoginRejectsHugePassword(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.login(t)
	huge := strings.Repeat("a", 1025)
	for _, name := range []string{simon, "nobody"} {
		if _, err := e.svc.Login(t.Context(), name, huge, ""); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Errorf("Login(%q, huge) = %v", name, err)
		}
	}
}

func TestNewRejectsBadParams(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if _, err := auth.New(e.st, auth.Options{Params: auth.Params{Time: 0, MemoryKiB: 64, Threads: 1}}); err == nil {
		t.Error("New accepted Time 0")
	}
	if _, err := auth.New(e.st, auth.Options{MaxConcurrentHashes: -1}); err == nil {
		t.Error("New accepted a negative MaxConcurrentHashes")
	}
}

func TestReturnedUsersCarryNoHash(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	created, err := e.svc.CreateUser(t.Context(), simon, password, store.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Login(t.Context(), simon, password, "")
	if err != nil {
		t.Fatal(err)
	}
	authed, err := e.svc.Authenticate(t.Context(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]store.User{"CreateUser": created, "Login": res.User, "Authenticate": authed} {
		if u.PasswordHash != "" {
			t.Errorf("%s returned a password hash", name)
		}
	}
}
