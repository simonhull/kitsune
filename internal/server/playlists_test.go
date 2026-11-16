package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	playlistsPath = "/me/playlists"
	mixName       = "Mix"
)

// playlistPath is the path of a playlist, or of one of its parts.
func playlistPath(id string, part ...string) string {
	return strings.Join(append([]string{playlistsPath, id}, part...), "/")
}

// playlistsFixture is a server with two albums of three songs, a signed-in
// admin and another person.
func playlistsFixture(t *testing.T) (*testServer, session, string) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 3}, browseAlbum{n: 2, sortName: "b", tracks: 3})
	sess := ts.sessionFor(t)
	other := ts.tokenFor(t, store.RoleUser)
	return ts, sess, other
}

// jsonBody marshals v for a request.
func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// makePlaylist sends POST /me/playlists and decodes the 201.
func (ts *testServer) makePlaylist(t *testing.T, token string, req *api.PlaylistCreate) api.PlaylistAdded {
	t.Helper()
	resp := ts.send(t, call{method: http.MethodPost, path: playlistsPath, body: jsonBody(t, req), header: jsonHeader(token)})
	if resp.Status != http.StatusCreated {
		t.Fatalf("POST %s = %d, want 201: %s", playlistsPath, resp.Status, resp.Body)
	}
	var out api.PlaylistAdded
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// playlist reads GET /me/playlists/{id}.
func (ts *testServer) playlist(t *testing.T, token, id string) api.PlaylistDetail {
	t.Helper()
	var d api.PlaylistDetail
	ts.getJSON(t, token, playlistPath(id), &d)
	return d
}

// entryTracks lists a playlist's songs, in order.
func entryTracks(d *api.PlaylistDetail) []string {
	out := make([]string, len(d.Entries))
	for i := range d.Entries {
		out[i] = d.Entries[i].Track.ID
	}
	return out
}

// entryIDs lists a playlist's entries, in order.
func entryIDs(d *api.PlaylistDetail) []string {
	out := make([]string, len(d.Entries))
	for i := range d.Entries {
		out[i] = d.Entries[i].ID
	}
	return out
}

// A playlist is made with songs, listed with them counted, read with each entry
// and its album, renamed and deleted.
func TestPlaylistRoundTrip(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	made := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: "  Falling Up ", Tracks: []string{songAt(2, 1), songAt(1, 1)}})
	id := made.Playlist.ID
	if got := fmt.Sprintf("%s %d %d %d", made.Playlist.Name, made.Playlist.SongCount, len(made.Added), len(made.Skipped)); got != "Falling Up 2 2 0" {
		t.Errorf("made %s, want Falling Up with its two songs added", got)
	}
	var list api.Playlists
	ts.getJSON(t, sess.token, playlistsPath+"?holding="+songAt(1, 1)+","+songAt(1, 2), &list)
	if got := fmt.Sprint(len(list.Playlists), list.Playlists[0].Holds != nil && *list.Playlists[0].Holds == 1, len(list.Playlists[0].Mosaic)); got != "1 true 2" {
		t.Errorf("list = %+v, want Falling Up holding one of the two, with two albums", list)
	}
	d := ts.playlist(t, sess.token, id)
	albums := make([]string, len(d.Albums))
	for i := range d.Albums {
		albums[i] = d.Albums[i].ID
	}
	if !slices.Equal(entryTracks(&d), []string{songAt(2, 1), songAt(1, 1)}) || !slices.Equal(albums, []string{albumULID(2), albumULID(1)}) {
		t.Errorf("entries %v of albums %v, want album 2's first song, then album 1's", entryTracks(&d), albums)
	}
	var renamed api.Playlist
	ts.send(t, call{method: http.MethodPatch, path: playlistPath(id), body: `{"name":"Falling Down"}`, header: jsonHeader(sess.token)}).decode(t, &renamed)
	if renamed.Name != "Falling Down" {
		t.Errorf("renamed = %+v, want Falling Down", renamed)
	}
	if resp := ts.send(t, call{method: http.MethodDelete, path: playlistPath(id), header: bearer(sess.token)}); resp.Status != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204: %s", resp.Status, resp.Body)
	}
	ts.send(t, call{method: http.MethodGet, path: playlistPath(id), header: bearer(sess.token)}).problem(t, http.StatusNotFound)
}

// Another person's playlist is a 404 to every route, never a 403.
func TestAnotherPersonsPlaylistIsNotFound(t *testing.T) {
	t.Parallel()
	ts, sess, other := playlistsFixture(t)
	made := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: "Mine", Tracks: []string{songAt(1, 1), songAt(1, 2)}})
	id, entries := made.Playlist.ID, made.Added
	for _, c := range []call{
		{method: http.MethodGet, path: playlistPath(id), header: bearer(other)},
		{method: http.MethodPatch, path: playlistPath(id), body: `{"name":"Theirs"}`, header: jsonHeader(other)},
		{method: http.MethodDelete, path: playlistPath(id), header: bearer(other)},
		{method: http.MethodPost, path: playlistPath(id, "tracks"), body: tracksBody(songAt(1, 3)), header: jsonHeader(other)},
		{method: http.MethodDelete, path: playlistPath(id, "entries"), body: `{"entries":["` + entries[0].ID + `"]}`, header: jsonHeader(other)},
		{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"` + entries[0].ID + `","after":"` + entries[1].ID + `"}`, header: jsonHeader(other)},
	} {
		ts.send(t, c).problem(t, http.StatusNotFound)
	}
	var list api.Playlists
	ts.getJSON(t, other, playlistsPath, &list)
	if len(list.Playlists) != 0 {
		t.Errorf("the other person lists %+v, want nothing", list)
	}
	if d := ts.playlist(t, sess.token, id); d.Name != "Mine" || len(d.Entries) != 2 {
		t.Errorf("after all that the playlist is %+v, want it untouched", d)
	}
}

// An add skips what is there, names it, and adds it again when asked; an album
// goes in, in its order, and before an entry puts songs there.
func TestAddToPlaylistRoute(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	id := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{songAt(1, 1)}}).Playlist.ID
	add := func(req *api.PlaylistAdd) api.PlaylistAdded {
		t.Helper()
		var out api.PlaylistAdded
		ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "tracks"), body: jsonBody(t, req), header: jsonHeader(sess.token)}).decode(t, &out)
		return out
	}
	got := add(&api.PlaylistAdd{Tracks: []string{songAt(1, 1), songAt(1, 2)}})
	if len(got.Added) != 1 || got.Added[0].Track != songAt(1, 2) || !slices.Equal(got.Skipped, []string{songAt(1, 1)}) || got.Playlist.SongCount != 2 {
		t.Errorf("add = %+v, want song 2 added and song 1 skipped", got)
	}
	if again := add(&api.PlaylistAdd{Tracks: got.Skipped, Again: true}); len(again.Added) != 1 {
		t.Errorf("again = %+v, want song 1 a second time", again)
	}
	first := ts.playlist(t, sess.token, id).Entries[0].ID
	add(&api.PlaylistAdd{Albums: []string{albumULID(2)}, Before: first})
	want := []string{songAt(2, 1), songAt(2, 2), songAt(2, 3), songAt(1, 1), songAt(1, 2), songAt(1, 1)}
	if d := ts.playlist(t, sess.token, id); !slices.Equal(entryTracks(&d), want) {
		t.Errorf("order = %v, want %v", entryTracks(&d), want)
	}
	ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "tracks"), body: `{"tracks":["` + songAt(1, 3) + `"],"before":"nope"}`, header: jsonHeader(sess.token)}).
		problem(t, http.StatusNotFound)
}

// An add or a make naming what isn't in the library is a 422 naming it.
func TestAddToPlaylistNamesWhatIsNotLive(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	id := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName}).Playlist.ID
	for _, c := range []call{
		{method: http.MethodPost, path: playlistPath(id, "tracks"), body: `{"tracks":["` + songAt(1, 1) + `","nope"],"albums":["gone"]}`, header: jsonHeader(sess.token)},
		{method: http.MethodPost, path: playlistsPath, body: `{"name":"New","tracks":["nope"],"albums":["gone"]}`, header: jsonHeader(sess.token)},
	} {
		resp := ts.send(t, c)
		resp.problem(t, http.StatusUnprocessableEntity)
		var p api.PlaylistProblem
		if err := json.Unmarshal(resp.Body, &p); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(p.Tracks, []string{missingID}) || !slices.Equal(p.Albums, []string{"gone"}) {
			t.Errorf("%s %s: problem = %+v, want it to name nope and gone", c.method, c.path, p)
		}
	}
	var list api.Playlists
	ts.getJSON(t, sess.token, playlistsPath, &list)
	if len(list.Playlists) != 1 || list.Playlists[0].SongCount != 0 {
		t.Errorf("list = %+v, want the one empty playlist", list)
	}
}

// A take-out and a move change the order, and a reload shows it.
func TestTakeOutAndMoveInPlaylist(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	id := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{songAt(1, 1), songAt(1, 2), songAt(1, 3)}}).Playlist.ID
	e := entryIDs(new(ts.playlist(t, sess.token, id)))
	resp := ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"` + e[0] + `","after":"` + e[2] + `"}`, header: jsonHeader(sess.token)})
	if resp.Status != http.StatusNoContent {
		t.Fatalf("move = %d, want 204: %s", resp.Status, resp.Body)
	}
	var out api.PlaylistEntries
	ts.send(t, call{method: http.MethodDelete, path: playlistPath(id, "entries"), body: `{"entries":["` + e[1] + `","999999","x"]}`, header: jsonHeader(sess.token)}).decode(t, &out)
	if !slices.Equal(out.Entries, e[1:2]) {
		t.Errorf("taken out %v, want %v", out.Entries, e[1:2])
	}
	if d := ts.playlist(t, sess.token, id); !slices.Equal(entryIDs(&d), []string{e[2], e[0]}) {
		t.Errorf("order = %v, want %v", entryIDs(&d), []string{e[2], e[0]})
	}
	ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"` + e[1] + `","before":"` + e[0] + `"}`, header: jsonHeader(sess.token)}).
		problem(t, http.StatusNotFound)
}

// Every playlist route needs credentials.
func TestPlaylistsNeedCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	h := jsonHeader("nope")
	for _, c := range []call{
		{method: http.MethodGet, path: playlistsPath, header: bearer("nope")},
		{method: http.MethodPost, path: playlistsPath, body: `{"name":"Mix"}`, header: h},
		{method: http.MethodGet, path: playlistPath("x"), header: bearer("nope")},
		{method: http.MethodPatch, path: playlistPath("x"), body: `{"name":"Mix"}`, header: h},
		{method: http.MethodDelete, path: playlistPath("x"), header: bearer("nope")},
		{method: http.MethodPost, path: playlistPath("x", "tracks"), body: tracksBody("t"), header: h},
		{method: http.MethodDelete, path: playlistPath("x", "entries"), body: `{"entries":["1"]}`, header: h},
		{method: http.MethodPost, path: playlistPath("x", "move"), body: `{"entry":"1","after":"2"}`, header: h},
		{method: http.MethodGet, path: playlistPath("x", "m3u"), header: bearer("nope")},
		{method: http.MethodPost, path: savePath + "x/save", body: `{"root":"/r","folder":"","name":"n"}`, header: h},
	} {
		ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

// What a playlist request can't be is a 400.
func TestPlaylistBadRequests(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	id := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{songAt(1, 1)}}).Playlist.ID
	many := make([]string, mostHolding+1)
	for i := range many {
		many[i] = songAt(1, i)
	}
	h := jsonHeader(sess.token)
	for _, c := range []call{
		{method: http.MethodGet, path: playlistsPath + "?holding=" + strings.Join(many, ","), header: bearer(sess.token)},
		{method: http.MethodPost, path: playlistsPath, body: `{"name":"   "}`, header: h},
		{method: http.MethodPost, path: playlistsPath, body: `{"name":"` + strings.Repeat("x", 201) + `"}`, header: h},
		{method: http.MethodPatch, path: playlistPath(id), body: `{"name":""}`, header: h},
		{method: http.MethodPost, path: playlistPath(id, "tracks"), body: `{}`, header: h},
		{method: http.MethodDelete, path: playlistPath(id, "entries"), body: `{"entries":[]}`, header: h},
		{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"1"}`, header: h},
		{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"1","before":"2","after":"3"}`, header: h},
	} {
		ts.send(t, c).problem(t, http.StatusBadRequest)
	}
}

// A playlist past 10,000 songs is a 422, and nothing goes in.
func TestPlaylistLimitRoute(t *testing.T) {
	t.Parallel()
	ts, sess, _ := playlistsFixture(t)
	many := make([]string, store.MostPlaylistEntries)
	for i := range many {
		many[i] = songAt(1+i%2, 1+i%3)
	}
	id := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: "Long"}).Playlist.ID
	var full api.PlaylistAdded
	ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "tracks"), body: jsonBody(t, &api.PlaylistAdd{Tracks: many, Again: true}), header: jsonHeader(sess.token)}).decode(t, &full)
	if full.Playlist.SongCount != store.MostPlaylistEntries {
		t.Fatalf("count = %d, want %d", full.Playlist.SongCount, store.MostPlaylistEntries)
	}
	ts.send(t, call{method: http.MethodPost, path: playlistPath(id, "tracks"), body: `{"tracks":["` + songAt(1, 1) + `"],"again":true}`, header: jsonHeader(sess.token)}).
		problem(t, http.StatusUnprocessableEntity)
}

// A song in Trash leaves the playlist's page and comes back with Restore.
func TestTrashedSongLeavesPlaylistPage(t *testing.T) {
	t.Parallel()
	fx := trashFixture(t)
	song := fx.album.Tracks[1].ID
	id := fx.makePlaylist(t, fx.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{fx.album.Tracks[0].ID, song}}).Playlist.ID
	fx.trashed(t, trashPath, tracksBody(song))
	if d := fx.playlist(t, fx.token, id); !slices.Equal(entryTracks(&d), []string{fx.album.Tracks[0].ID}) || d.SongCount != 1 {
		t.Errorf("with a song in Trash the playlist shows %v (%d), want the other song alone", entryTracks(&d), d.SongCount)
	}
	fx.trashed(t, restorePath, tracksBody(song))
	if d := fx.playlist(t, fx.token, id); !slices.Equal(entryTracks(&d), []string{fx.album.Tracks[0].ID, song}) {
		t.Errorf("after Restore the playlist shows %v, want both songs", entryTracks(&d))
	}
}

// Every change to a playlist is a playlists event to its person's devices,
// naming it, and to nobody else.
func TestPlaylistEventsGoToTheirPerson(t *testing.T) {
	t.Parallel()
	ts, sess, other := playlistsFixture(t)
	mine := ts.openStream(t, sess.token, "")
	theirs := ts.openStream(t, other, "")
	mine.want(events.Ready)
	theirs.want(events.Ready)
	made := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{songAt(1, 1), songAt(1, 2)}})
	id, e := made.Playlist.ID, made.Added
	wantPlaylist := func() {
		t.Helper()
		var change api.EventPlaylists
		decodeEvent(t, mine.want(events.Playlists), &change)
		if !slices.Equal(change.IDs, []string{id}) {
			t.Errorf("event = %+v, want it to name %s", change, id)
		}
	}
	wantPlaylist()
	h := jsonHeader(sess.token)
	for _, c := range []call{
		{method: http.MethodPatch, path: playlistPath(id), body: `{"name":"Mixed"}`, header: h},
		{method: http.MethodPost, path: playlistPath(id, "tracks"), body: tracksBody(songAt(1, 3)), header: h},
		{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"` + e[0].ID + `","after":"` + e[1].ID + `"}`, header: h},
		{method: http.MethodDelete, path: playlistPath(id, "entries"), body: `{"entries":["` + e[0].ID + `"]}`, header: h},
		{method: http.MethodDelete, path: playlistPath(id), header: bearer(sess.token)},
	} {
		ts.send(t, c)
		wantPlaylist()
	}
	ts.ping()
	mine.want(events.Library)
	theirs.want(events.Library) // The other person heard none of it.
}
