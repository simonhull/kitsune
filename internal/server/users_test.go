package server

import (
	"encoding/json/v2"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const usersPath = "/admin/users"

// setCall is POST /admin/users/{id}/password with token and body.
func setCall(t *testing.T, token, id string, body api.PasswordSet) call {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return call{method: http.MethodPost, path: usersPath + "/" + id + "/password", body: string(b), header: jsonHeader(token)}
}

func idOf(u *store.User) string { return strconv.FormatInt(u.ID, 10) }

func TestAdminSetsPassword(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	admin := ts.sessionFor(t)
	kim := ts.seedUser(t, "kim", testPassword, store.RoleUser)
	kimSession, err := ts.Auth.Login(t.Context(), "kim", testPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	resp := ts.send(t, setCall(t, admin.token, idOf(&kim), api.PasswordSet{New: newPassword}))

	if resp.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.Status, resp.Body)
	}
	if ts.works(t, kimSession.Token) {
		t.Error("kim's session survived")
	}
	if !ts.works(t, admin.token) {
		t.Error("the administrator's session ended")
	}
	if _, err = ts.Auth.Login(t.Context(), "kim", newPassword, ""); err != nil {
		t.Errorf("kim with the new password: %v", err)
	}
	ts.noSecretIn(t, resp, newPassword, admin.token)
}

func TestAdminSetPasswordRefusals(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	admin := ts.sessionFor(t)
	kim := ts.seedUser(t, "kim", testPassword, store.RoleUser)
	kimSession, err := ts.Auth.Login(t.Context(), "kim", testPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	set := api.PasswordSet{New: newPassword}
	tests := []struct {
		name    string
		call    call
		status  int
		invalid bool
	}{
		{name: "own account", call: setCall(t, admin.token, idOf(&admin.user), set), status: http.StatusForbidden},
		{name: "not an administrator", call: setCall(t, kimSession.Token, idOf(&admin.user), set), status: http.StatusForbidden},
		{name: "unknown user", call: setCall(t, admin.token, "9999", set), status: http.StatusNotFound},
		{name: "id not a number", call: setCall(t, admin.token, "kim", set), status: http.StatusNotFound, invalid: true},
		{name: "short password", call: setCall(t, admin.token, idOf(&kim), api.PasswordSet{New: shortPassword}), status: http.StatusBadRequest},
		{name: "extra member", call: call{method: http.MethodPost, path: usersPath + "/" + idOf(&kim) + "/password", body: `{"new":"` + newPassword + `","x":1}`, header: jsonHeader(admin.token)}, status: http.StatusBadRequest, invalid: true},
		{name: "no credentials", call: call{method: http.MethodPost, path: usersPath + "/" + idOf(&kim) + "/password", body: `{"new":"` + newPassword + `"}`, header: http.Header{contentType: {jsonType}}}, status: http.StatusUnauthorized, invalid: true},
	}
	for _, tt := range tests { // In turn: the sessions must outlive them all.
		c := tt.call
		c.invalid = tt.invalid
		resp := ts.send(t, c)
		if resp.Status != tt.status {
			t.Errorf("%s: status = %d, want %d", tt.name, resp.Status, tt.status)
		}
		resp.problem(t, tt.status)
		ts.noSecretIn(t, resp, newPassword)
	}
	if !ts.works(t, kimSession.Token) || !ts.works(t, admin.token) {
		t.Error("a refused set ended a session")
	}
}

func TestAdminListsUsers(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	admin := ts.sessionFor(t)
	ts.seedUser(t, "kim", testPassword, store.RoleUser)
	ts.Clock.Advance(2 * time.Hour)
	ts.send(t, call{method: http.MethodGet, path: mePath, header: bearer(admin.token)}) // Touches last_seen.

	resp := ts.send(t, call{method: http.MethodGet, path: usersPath, header: bearer(admin.token)})

	var got api.Accounts
	resp.decode(t, &got)
	if len(got.Users) != 2 {
		t.Fatalf("users = %+v, want kim and simon", got.Users)
	}
	kim, simon := got.Users[0], got.Users[1]
	if kim.User.Name != "kim" || kim.Sessions != 0 || kim.LastSeen != nil {
		t.Errorf("kim = %+v, want no sessions", kim)
	}
	if simon.User.Name != simonName || simon.Sessions != 1 || simon.LastSeen == nil || !simon.LastSeen.Equal(ts.Clock.Now()) {
		t.Errorf("simon = %+v, want 1 session last seen now", simon)
	}
	noSecretInResponse(t, resp, admin.token, "argon2")
}

func TestUsersListIsForAdministrators(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: usersPath, header: bearer(ts.tokenFor(t, store.RoleUser))}).problem(t, http.StatusForbidden)
	ts.send(t, call{method: http.MethodGet, path: usersPath, invalid: true}).problem(t, http.StatusUnauthorized)
}
