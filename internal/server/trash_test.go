package server

import (
	"encoding/json/v2"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	trashPath   = "/admin/trash"
	restorePath = "/admin/trash/restore"
	emptyPath   = "/admin/trash/empty"
	emptyAll    = `{"all":true}`
)

// trashFixture is an album of two songs on a server with tag writes on.
func trashFixture(t *testing.T) *editFixture {
	t.Helper()
	return newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
}

// trashCall is a POST of body to path as the fixture's administrator.
func (fx *editFixture) trashCall(path, body string) call {
	return call{method: http.MethodPost, path: path, body: body, header: jsonHeader(fx.token)}
}

// trashed posts body to path, wants a 200, and returns what moved.
func (fx *editFixture) trashed(t *testing.T, path, body string) api.TrashChange {
	t.Helper()
	resp := fx.send(t, fx.trashCall(path, body))
	if resp.Status != http.StatusOK {
		t.Fatalf("POST %s %s = %d: %s", path, body, resp.Status, resp.Body)
	}
	var change api.TrashChange
	if err := json.Unmarshal(resp.Body, &change); err != nil {
		t.Fatal(err)
	}
	return change
}

// trash reads GET /admin/trash.
func (fx *editFixture) trash(t *testing.T) api.Trash {
	t.Helper()
	var list api.Trash
	fx.getJSON(t, fx.token, trashPath, &list)
	return list
}

// A song moved to the Trash leaves the album and every stream, shows in the
// Trash with who moved it and when it goes, and comes back whole.
func TestTrashAndRestore(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	song := fx.album.Tracks[1].ID
	change := fx.trashed(t, trashPath, tracksBody(song))
	if !slices.Equal(change.Tracks, []string{song}) || !slices.Equal(change.Albums, []string{fx.album.ID}) {
		t.Errorf("change = %+v, want the song and its album", change)
	}
	fx.reload(t)
	if len(fx.album.Tracks) != 1 || fx.album.TrackCount != 1 {
		t.Errorf("album after trash = %d songs, counted %d; want 1", len(fx.album.Tracks), fx.album.TrackCount)
	}
	fx.send(t, call{method: http.MethodGet, path: streamPath(song), header: bearer(fx.token)}).problem(t, http.StatusNotFound)
	fx.wantTrashed(t, song)
	if back := fx.trashed(t, restorePath, tracksBody(song)); !slices.Equal(back.Tracks, []string{song}) {
		t.Errorf("restored = %+v, want the song", back)
	}
	fx.reload(t)
	if len(fx.album.Tracks) != 2 || fx.trash(t).Count != 0 {
		t.Errorf("after restore: %d songs on the album, %d in the Trash; want 2 and 0", len(fx.album.Tracks), fx.trash(t).Count)
	}
}

// wantTrashed checks that the Trash holds song alone, on the fixture's album,
// moved by its administrator and going 30 days on.
func (fx *editFixture) wantTrashed(t *testing.T, song string) {
	t.Helper()
	list := fx.trash(t)
	if list.Count != 1 || list.RetentionDays != 30 || len(list.Albums) != 1 || list.Albums[0].ID != fx.album.ID {
		t.Fatalf("trash = %+v, want the one song on its album, for 30 days", list)
	}
	tr := list.Albums[0].Tracks[0]
	want := api.TrashedTrack{
		ID: song, Title: tr.Title, Size: list.Bytes, TrashedAt: tr.TrashedAt, TrashedBy: "user-admin",
		PurgeAt: tr.TrashedAt.Add(30 * 24 * time.Hour),
	}
	if !reflect.DeepEqual(tr, want) || tr.Size == 0 {
		t.Errorf("trashed song = %+v, want %+v", tr, want)
	}
}

// An album goes to the Trash whole, and emptying deletes it for good.
func TestTrashAnAlbumAndEmpty(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	change := fx.trashed(t, trashPath, `{"albums":["`+fx.album.ID+`"]}`)
	if len(change.Tracks) != 2 {
		t.Fatalf("change = %+v, want both songs", change)
	}
	fx.send(t, call{method: http.MethodGet, path: albumsPath + "/" + fx.album.ID, header: bearer(fx.token)}).problem(t, http.StatusNotFound)
	for _, body := range []string{tracksBody(change.Tracks[0]), emptyAll} {
		var gone api.TrashEmptied
		resp := fx.send(t, fx.trashCall(emptyPath, body))
		if err := json.Unmarshal(resp.Body, &gone); err != nil || resp.Status != http.StatusOK || gone.Deleted != 1 {
			t.Fatalf("empty %s = %d %s, want 1 deleted", body, resp.Status, resp.Body)
		}
	}
	if list := fx.trash(t); list.Count != 0 || len(list.Albums) != 0 {
		t.Errorf("trash = %+v, want empty", list)
	}
	if _, err := os.Stat(filepath.Join(fx.root, fileOne)); !os.IsNotExist(err) {
		t.Errorf("the file is still there: %v", err)
	}
}

// What each route refuses.
func TestTrashRefuses(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	song := fx.album.Tracks[0].ID
	user := fx.tokenFor(t, store.RoleUser)
	// In turn, not as subtests at once: each takes the library lock, and all but
	// one of them at once would be busy.
	for _, tc := range []struct {
		name   string
		c      call
		status int
	}{
		{"both songs and albums", fx.trashCall(trashPath, `{"tracks":["`+song+`"],"albums":["`+fx.album.ID+`"]}`), http.StatusBadRequest},
		{"an unknown song", fx.trashCall(trashPath, tracksBody("01J00000000000000000000000")), http.StatusNotFound},
		{"an album not there", fx.trashCall(trashPath, `{"albums":["01J00000000000000000000000"]}`), http.StatusNotFound},
		{"a song not in the Trash", fx.trashCall(restorePath, tracksBody(song)), http.StatusNotFound},
		{"emptying a song not in the Trash", fx.trashCall(emptyPath, tracksBody(song)), http.StatusNotFound},
		{"a bad body", invalidCall(fx.trashCall(trashPath, `{"songs":[]}`)), http.StatusBadRequest},
		{"a bad restore", invalidCall(fx.trashCall(restorePath, `{}`)), http.StatusBadRequest},
		{"a bad empty", invalidCall(fx.trashCall(emptyPath, `{"all":false}`)), http.StatusBadRequest},
		{"no credentials to list", invalidCall(call{method: http.MethodGet, path: trashPath}), http.StatusUnauthorized},
		{"a bad token to trash", call{method: http.MethodPost, path: trashPath, body: tracksBody(song), header: jsonHeader("")}, http.StatusUnauthorized},
		{"a bad token to restore", call{method: http.MethodPost, path: restorePath, body: tracksBody(song), header: jsonHeader("")}, http.StatusUnauthorized},
		{"a bad token to empty", call{method: http.MethodPost, path: emptyPath, body: emptyAll, header: jsonHeader("")}, http.StatusUnauthorized},
		{"a listener lists", call{method: http.MethodGet, path: trashPath, header: bearer(user)}, http.StatusForbidden},
		{"a listener trashes", call{method: http.MethodPost, path: trashPath, body: tracksBody(song), header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener restores", call{method: http.MethodPost, path: restorePath, body: tracksBody(song), header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener empties", call{method: http.MethodPost, path: emptyPath, body: emptyAll, header: jsonHeader(user)}, http.StatusForbidden},
	} {
		if resp := fx.send(t, tc.c); resp.Status != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, resp.Status, tc.status, resp.Body)
		} else if tc.status != http.StatusUnauthorized {
			resp.problem(t, tc.status)
		}
	}
	if list := fx.trash(t); list.Count != 0 {
		t.Errorf("trash = %+v, want nothing moved", list)
	}
}

// invalidCall marks c as breaking the contract on purpose.
func invalidCall(c call) call {
	c.invalid = true
	return c
}

// With tag writes off nothing under a root may change: the Trash is a 409
// with the reason, and Me says so.
func TestTrashNeedsTagWrites(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	song := fx.album.Tracks[0].ID
	for _, path := range []string{trashPath, restorePath, emptyPath} {
		p := fx.send(t, fx.trashCall(path, tracksBody(song))).problem(t, http.StatusConflict)
		if p.Detail != trashOffDetail {
			t.Errorf("%s: detail = %q, want %q", path, p.Detail, trashOffDetail)
		}
	}
	var me api.Me
	fx.getJSON(t, fx.token, mePath, &me)
	if me.CanTrash {
		t.Error("canTrash = true with tag writes off")
	}
}

// Me says who may use the Trash: an administrator, with tag writes on.
func TestMeSaysWhoMayTrash(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	for token, want := range map[string]bool{fx.token: true, fx.tokenFor(t, store.RoleUser): false} {
		var me api.Me
		fx.getJSON(t, token, mePath, &me)
		if me.CanTrash != want {
			t.Errorf("%s: canTrash = %t, want %t", me.Role, me.CanTrash, want)
		}
	}
}

// A restore never overwrites: a file at the old path refuses it, naming the song.
func TestRestoreRefusedWhenThePathIsTaken(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	song := fx.album.Tracks[0]
	fx.trashed(t, trashPath, tracksBody(song.ID))
	rel, err := fx.Store.TrashedTracks(t.Context(), []string{song.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fx.root, rel[0].RestorePath), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := fx.send(t, fx.trashCall(restorePath, tracksBody(song.ID))).problem(t, http.StatusConflict)
	if want := "“" + song.Title + "” can't go back: a file is at its old path"; p.Detail != want {
		t.Errorf("detail = %q, want %q", p.Detail, want)
	}
}

// A file that won't move is a 409 naming the song, never a path, and nothing moves.
func TestTrashOfAFileThatWontMove(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	song := fx.album.Tracks[0]
	if err := os.MkdirAll(filepath.Join(fx.root, library.TrashDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.root, library.TrashDir, song.ID), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := fx.send(t, fx.trashCall(trashPath, tracksBody(song.ID))).problem(t, http.StatusConflict)
	if want := "“" + song.Title + "” would not move, so nothing did; the server's log says why"; p.Detail != want {
		t.Errorf("detail = %q, want %q", p.Detail, want)
	}
	fx.reload(t)
	if len(fx.album.Tracks) != 2 {
		t.Errorf("album = %d songs, want both still there", len(fx.album.Tracks))
	}
}

// The Trash waits for no one: during an edit it is a 409 with Retry-After.
func TestTrashDuringAnEditIsBusy(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, Write: holdingWrite(entered, release)}, twoFLACs())
	first := make(chan error, 1)
	c := fx.postCall(fx.editsPath(), fx.album.Etag, yearBody)
	go func() {
		_, err := fx.trySend(t, c)
		first <- err
	}()
	<-entered
	resp := fx.send(t, fx.trashCall(trashPath, tracksBody(fx.album.Tracks[0].ID)))
	resp.problem(t, http.StatusConflict)
	if resp.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

// Trashing and restoring tell everyone which album changed, so each client
// patches its index; emptying tells no one.
func TestTrashEvents(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	tap := fx.Events.Subscribe(events.Viewer{UserID: 99}, "")
	defer fx.Events.Unsubscribe(tap)
	if ev := <-tap.C(); ev.Kind != events.Ready {
		t.Fatalf("first event = %s, want ready", ev.Kind)
	}
	song := fx.album.Tracks[0].ID
	for _, path := range []string{trashPath, restorePath, trashPath} {
		fx.trashed(t, path, tracksBody(song))
		ev := <-tap.C()
		var change api.EventLibrary
		if err := json.Unmarshal(ev.Data, &change); err != nil || ev.Kind != events.Library || !slices.Equal(change.Albums, []string{fx.album.ID}) {
			t.Errorf("after %s: event %s %s, want a library event for the album", path, ev.Kind, ev.Data)
		}
	}
	fx.send(t, fx.trashCall(emptyPath, emptyAll))
	if heard := drainTap(tap); len(heard) != 0 {
		t.Errorf("emptying published %q, want nothing", heard)
	}
}
