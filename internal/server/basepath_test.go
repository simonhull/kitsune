package server

import (
	"net/http"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const musicBase = "/music"

// plainClient is a client that does not validate against the contract; base
// path requests are outside its paths.
func plainClient() *http.Client { return &http.Client{} }

func TestBasePathServes(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{BasePath: musicBase})
	resp := ts.send(t, call{method: http.MethodGet, path: musicBase + healthPath, client: plainClient()})
	if resp.Status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.Status)
	}
}

func TestBasePathMisses(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{BasePath: musicBase})
	for _, path := range []string{healthPath, musicBase, "/musicx/healthz", musicBase + "%2Fhealthz"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			resp := ts.send(t, call{method: http.MethodGet, path: path, client: plainClient()})
			p := resp.problem(t, http.StatusNotFound)
			if p.RequestID == "" {
				t.Error("problem has no requestId")
			}
		})
	}
}

func TestBasePathCookies(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{BasePath: musicBase})
	ts.seedUser(t, simonName, testPassword, store.RoleUser)

	login := loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword, Cookie: true})
	login.path, login.client = musicBase+loginPath, plainClient()
	resp := ts.send(t, login)
	decodeSession(t, resp)
	if c := sessionCookieOf(t, resp); c.Path != musicBase {
		t.Errorf("login cookie Path = %q, want %q", c.Path, musicBase)
	}

	token := loginCall(t, "", api.LoginRequest{Name: simonName, Password: testPassword})
	token.path, token.client = musicBase+loginPath, plainClient()
	s := decodeSession(t, ts.send(t, token))
	logout := call{method: http.MethodPost, path: musicBase + logoutPath, header: bearer(s.Token), client: plainClient()}
	out := ts.send(t, logout)
	if out.Status != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", out.Status)
	}
	if c := sessionCookieOf(t, out); c.Path != musicBase || c.MaxAge >= 0 {
		t.Errorf("logout cookie = %+v, want Path %q and a negative Max-Age", c, musicBase)
	}
}

func TestNewPanicsOnBadBasePath(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"music", "/music/", "/"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("New with BasePath %q did not panic", base)
				}
			}()
			ts := newTestServer(t)
			New(Config{Auth: ts.Auth, BasePath: base})
		})
	}
}
