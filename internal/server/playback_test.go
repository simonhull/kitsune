package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	playbackPath = "/me/playback"
	aClientID    = "0123456789abcdef"
	kindSongs    = "songs"
	kindList     = "list"
	kindShuffle  = "shuffle"
	deviceTablet = "tablet"
)

// playbackFixture is a server with two albums of five songs and a signed-in admin.
func playbackFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 5}, browseAlbum{n: 2, sortName: "b", tracks: 5})
	sess := ts.sessionFor(t)
	return ts, sess
}

func (ts *testServer) putPlayback(t *testing.T, token string, save *api.PlaybackSave) response {
	t.Helper()
	body, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	return ts.send(t, call{method: http.MethodPut, path: playbackPath, body: string(body), header: jsonHeader(token)})
}

func (ts *testServer) playback(t *testing.T, token string) api.Playback {
	t.Helper()
	var out api.Playback
	ts.getJSON(t, token, playbackPath, &out)
	return out
}

// albumSave is album 1's songs, on the third, 2:13 in.
func albumSave() *api.PlaybackSave {
	return &api.PlaybackSave{
		Kind: scopeAlbum, Tracks: []string{songAt(1, 1), songAt(1, 2), songAt(1, 3), songAt(1, 4), songAt(1, 5)},
		Index: 2, PositionMs: 133_000, Ordered: true, Continues: albumULID(1), ClientID: aClientID, Device: "phone",
	}
}

func TestPlaybackIsRemembered(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	ts.send(t, call{method: http.MethodGet, path: playbackPath, header: bearer(sess.token)}).problem(t, http.StatusNotFound)

	wantNoContent(t, ts.putPlayback(t, sess.token, albumSave()))
	resp := ts.send(t, call{method: http.MethodGet, path: playbackPath, header: bearer(sess.token)})
	if cc := resp.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got api.Playback
	resp.decode(t, &got)
	songs := make([]api.PlaybackSong, 5)
	for i := range songs {
		songs[i] = api.PlaybackSong{ID: songAt(1, i+1), AlbumID: albumULID(1)}
	}
	want := api.Playback{
		Kind: scopeAlbum, Tracks: songs, Index: 2, PositionMs: 133_000, Ordered: true, Continues: albumULID(1),
		ClientID: aClientID, Device: "phone", UpdatedAt: ts.Clock.Now().UnixMilli(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GET = %+v, want %+v", got, want)
	}
	other := ts.tokenFor(t, store.RoleUser)
	ts.send(t, call{method: http.MethodGet, path: playbackPath, header: bearer(other)}).problem(t, http.StatusNotFound)
}

func TestPlaybackLastWriteWins(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	wantNoContent(t, ts.putPlayback(t, sess.token, albumSave()))
	list := &api.PlaybackSave{
		Kind: kindList, Tracks: []string{songAt(2, 1)}, Later: []string{songAt(2, 2), songAt(1, 1)},
		ClientID: aClientID, Device: "computer",
	}
	wantNoContent(t, ts.putPlayback(t, sess.token, list))
	if got := ts.playback(t, sess.token); got.Kind != kindList || len(got.Later) != 2 || got.Later[1].AlbumID != albumULID(1) {
		t.Errorf("after a replace, GET = %+v", got)
	}
}

func TestPlaybackRefusesABadRecord(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	many := make([]string, 1001)
	for i := range many {
		many[i] = fmt.Sprintf("01J%023d", i)
	}
	tests := map[string]struct {
		edit    func(s *api.PlaybackSave)
		invalid bool // By the contract, not only by the server's rules.
	}{
		"over the cap":            {func(s *api.PlaybackSave) { s.Kind, s.Continues, s.Tracks = kindList, "", many }, true},
		"over the cap with later": {func(s *api.PlaybackSave) { s.Kind, s.Continues, s.Later = kindList, "", many[:996] }, false},
		"index past the end":      {func(s *api.PlaybackSave) { s.Index = 5 }, false},
		"shuffle on an album":     {func(s *api.PlaybackSave) { s.Shuffle = &api.PlaybackShuffleSave{ID: albumULID(9)} }, false},
		"a bad device":            {func(s *api.PlaybackSave) { s.Device = "watch" }, true},
		"a bad ULID":              {func(s *api.PlaybackSave) { s.Tracks[0] = strings.Repeat("u", 26) }, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			save := albumSave()
			tt.edit(save)
			body, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			c := call{method: http.MethodPut, path: playbackPath, body: string(body), header: jsonHeader(sess.token), invalid: tt.invalid}
			ts.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
	t.Run("not JSON", func(t *testing.T) {
		t.Parallel()
		c := call{method: http.MethodPut, path: playbackPath, body: "{", header: jsonHeader(sess.token), invalid: true}
		ts.send(t, c).problem(t, http.StatusBadRequest)
	})
	t.Run("over 64 KiB", func(t *testing.T) {
		t.Parallel()
		body := `{"kind":"album","tracks":["` + songAt(1, 1) + `"],"index":0,"positionMs":0,"ordered":false,` +
			`"clientId":"` + aClientID + `","device":"phone"` + strings.Repeat(" ", maxPlaybackBody) + `}`
		// Valid by the contract, which has no size: only the server counts bytes.
		c := call{method: http.MethodPut, path: playbackPath, body: body, header: jsonHeader(sess.token)}
		ts.send(t, c).problem(t, http.StatusBadRequest)
	})
}

func TestPlaybackNeedsASession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	for _, c := range []call{
		{method: http.MethodGet, path: playbackPath},
		{method: http.MethodPut, path: playbackPath, body: "{}", header: http.Header{contentType: {api.JSONMediaType}}},
	} {
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

func TestPlaybackByCookieNeedsTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	body, err := json.Marshal(albumSave())
	if err != nil {
		t.Fatal(err)
	}
	c := call{method: http.MethodPut, path: playbackPath, body: string(body), header: cookieOnly(sess.token)}
	c.header.Set(contentType, api.JSONMediaType)
	ts.send(t, c).problem(t, http.StatusForbidden)
	c.header.Set(requestHeader, "1")
	wantNoContent(t, ts.send(t, c))
}

// rowOf returns the row ID of the track with the ULID id.
func (ts *testServer) rowOf(t *testing.T, id string) int64 {
	t.Helper()
	rows, err := ts.Store.TrackRows(t.Context(), []string{id})
	if err != nil {
		t.Fatal(err)
	}
	return rows[id]
}

func TestPlaybackMovesOnFromAGoneSong(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	wantNoContent(t, ts.putPlayback(t, sess.token, albumSave()))
	gone := []int64{ts.rowOf(t, songAt(1, 1)), ts.rowOf(t, songAt(1, 3))}
	if err := ts.Store.MarkMissing(t.Context(), gone, time.Now()); err != nil {
		t.Fatal(err)
	}
	got := ts.playback(t, sess.token)
	if !got.Moved || got.PositionMs != 0 || len(got.Tracks) != 3 || got.Tracks[got.Index].ID != songAt(1, 4) {
		t.Errorf("GET = %+v, want song 4 current at 0 ms, moved", got)
	}

	more := []int64{ts.rowOf(t, songAt(1, 4)), ts.rowOf(t, songAt(1, 5))}
	if err := ts.Store.MarkMissing(t.Context(), more, time.Now()); err != nil {
		t.Fatal(err)
	}
	ts.send(t, call{method: http.MethodGet, path: playbackPath, header: bearer(sess.token)}).problem(t, http.StatusNotFound)
}

func TestPlaybackFollowsASongToItsNewAlbum(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	wantNoContent(t, ts.putPlayback(t, sess.token, albumSave()))
	// What a merge does to a song: the same track, now on album 2.
	moved := (&browseAlbum{n: 1, sortName: "a", tracks: 3}).writes()[2]
	moved.ID, moved.Album = ts.rowOf(t, songAt(1, 3)), (&browseAlbum{n: 2, sortName: "b", tracks: 1}).writes()[0].Album
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{moved}}); err != nil {
		t.Fatal(err)
	}
	if got := ts.playback(t, sess.token); got.Tracks[2] != (api.PlaybackSong{ID: songAt(1, 3), AlbumID: albumULID(2)}) {
		t.Errorf("current song = %+v, want it on album 2", got.Tracks[2])
	}
}

func TestPlaybackDropsAGoneContinues(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	save := albumSave()
	save.Continues = albumULID(9)
	wantNoContent(t, ts.putPlayback(t, sess.token, save))
	if got := ts.playback(t, sess.token); got.Continues != "" || got.Kind != scopeAlbum {
		t.Errorf("GET = %+v, want no continues", got)
	}
}

func TestPlaybackKeepsItsShuffle(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	first := songsOf(&page)[0]
	save := &api.PlaybackSave{
		Kind: kindShuffle, Tracks: []string{first}, Shuffle: &api.PlaybackShuffleSave{ID: page.ID, After: 1},
		ClientID: aClientID, Device: deviceTablet,
	}
	wantNoContent(t, ts.putPlayback(t, sess.token, save))
	got := ts.playback(t, sess.token)
	if got.Kind != kindShuffle || got.Shuffle == nil || got.Shuffle.ID != page.ID || got.Shuffle.After != 1 ||
		got.Shuffle.Scope.Kind != scopeLibrary || got.Shuffle.Mode != modeEndless {
		t.Errorf("GET = %+v, want the shuffle with its scope and mode", got)
	}
}

// Another person's session isn't theirs to go on: the songs come back alone.
func TestPlaybackLeavesAnotherPersonsShuffle(t *testing.T) {
	t.Parallel()
	ts, sess := playbackFixture(t)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	first := songsOf(&page)[0]
	save := &api.PlaybackSave{
		Kind: kindShuffle, Tracks: []string{first}, Shuffle: &api.PlaybackShuffleSave{ID: page.ID, After: 1},
		ClientID: aClientID, Device: deviceTablet,
	}
	other := ts.tokenFor(t, store.RoleUser)
	wantNoContent(t, ts.putPlayback(t, other, save))
	if got := ts.playback(t, other); got.Kind != kindSongs || got.Shuffle != nil || got.Tracks[0].ID != first {
		t.Errorf("GET = %+v, want songs without the shuffle", got)
	}
}
