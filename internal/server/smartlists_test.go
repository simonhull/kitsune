package server

import (
	"encoding/json/v2"
	"net/http"
	"slices"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/shuffler"
	"github.com/simonhull/kitsune/internal/store"
)

const previewPath = playlistsPath + "/preview"

// rockGenre is album 1's genre, and the name a smart playlist of it takes.
const rockGenre = "Rock"

// Rules the tests use: songs at 4★ and up, and the first song of each album by
// least recently played.
const (
	ratedRules  = `{"match":"all","rules":[{"field":"rating","op":"atLeast","value":8}]}`
	limitRules  = `{"match":"all","rules":[{"field":"format","op":"is","value":"flac"}],"limit":{"count":2,"unit":"songs","by":"leastRecent"},"order":"chosen"}`
	genreRules  = `{"match":"any","rules":[{"field":"genre","op":"is","value":"ROCK"}]}`
	brokenRules = `{"match":"all","rules":[{"field":"rating","op":"contains","value":8}]}`
)

// smartFixture is playlistsFixture with album 1's songs tagged Rock and two of
// the admin's songs rated 4★.
func smartFixture(t *testing.T) (*testServer, session, string) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 3, genres: []string{rockGenre}}, browseAlbum{n: 2, sortName: "b", tracks: 3})
	if err := ts.Handle.genres.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	sess := ts.sessionFor(t)
	for _, id := range []string{songAt(2, 2), songAt(1, 3)} {
		if err := ts.Store.Rate(t.Context(), sess.user.ID, id, rating.Rating(8)); err != nil {
			t.Fatal(err)
		}
	}
	other := ts.tokenFor(t, store.RoleUser)
	return ts, sess, other
}

// makeSmart makes a smart playlist of rules and returns it.
func (ts *testServer) makeSmart(t *testing.T, token, name, rules string) api.Playlist {
	t.Helper()
	return ts.makePlaylist(t, token, &api.PlaylistCreate{Name: name, Rules: []byte(rules)}).Playlist
}

func (ts *testServer) preview(t *testing.T, token, body string) api.SmartPreview {
	t.Helper()
	resp := ts.send(t, call{method: http.MethodPost, path: previewPath, body: body, header: jsonHeader(token)})
	if resp.Status != http.StatusOK {
		t.Fatalf("POST %s = %d: %s", previewPath, resp.Status, resp.Body)
	}
	var p api.SmartPreview
	if err := json.Unmarshal(resp.Body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func previewIDs(p *api.SmartPreview) []string {
	out := make([]string, len(p.Songs))
	for i := range p.Songs {
		out[i] = p.Songs[i].ID
	}
	return out
}

// wantSmart fails t unless p is a smart playlist with its rules, named name,
// holding the given number of songs from the given number of albums, each in
// its mosaic.
func wantSmart(t *testing.T, p *api.Playlist, name string, songs, albums int) {
	t.Helper()
	if !p.Smart || p.Name != name || p.SongCount != songs || p.AlbumCount != albums || len(p.Mosaic) != albums || string(p.Rules) == "" {
		t.Fatalf("playlist = %+v", p)
	}
}

// A smart playlist is made from rules and listed as what they choose, and
// asking what playlists hold leaves it out.
func TestSmartPlaylistMadeAndListed(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	sub := ts.openStream(t, sess.token, "")
	sub.want(events.Ready)
	p := ts.makeSmart(t, sess.token, " Rated ", ratedRules)
	wantSmart(t, &p, "Rated", 2, 2)
	sub.want(events.Playlists)
	var list api.Playlists
	ts.getJSON(t, sess.token, playlistsPath, &list)
	if len(list.Playlists) != 1 || !list.Playlists[0].Smart || list.Playlists[0].SongCount != 2 || list.Playlists[0].Holds != nil {
		t.Fatalf("list = %+v", list.Playlists)
	}
	ts.getJSON(t, sess.token, playlistsPath+"?holding="+songAt(2, 2), &list)
	if len(list.Playlists) != 0 {
		t.Errorf("holding lists %+v, want smart playlists left out", list.Playlists)
	}
}

// A smart playlist reads as what its rules choose, its entry IDs its places,
// with what the rules read of each song.
func TestSmartPlaylistReads(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	p := ts.makeSmart(t, sess.token, "Rated", ratedRules)
	d := ts.playlist(t, sess.token, p.ID)
	if !d.Smart || !slices.Equal(entryTracks(&d), []string{songAt(1, 3), songAt(2, 2)}) ||
		!slices.Equal(entryIDs(&d), []string{"1", "2"}) || d.Entries[0].Track.Stars != 4 || d.Entries[0].Track.TrackFields == nil {
		t.Fatalf("detail = %+v", d)
	}
}

// A smart playlist's rules change, and so does what it holds; then a rename on
// its own. Nothing of it is stored as entries.
func TestSmartPlaylistChanges(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	p := ts.makeSmart(t, sess.token, "Rated", ratedRules)
	sub := ts.openStream(t, sess.token, "")
	sub.want(events.Ready)
	changed := ts.send(t, call{method: http.MethodPatch, path: playlistPath(p.ID), body: `{"name":"Rock","rules":` + genreRules + `}`, header: jsonHeader(sess.token)})
	var after api.Playlist
	if changed.Status != http.StatusOK || json.Unmarshal(changed.Body, &after) != nil || after.Name != rockGenre || after.SongCount != 3 {
		t.Fatalf("PATCH = %d %s", changed.Status, changed.Body)
	}
	sub.want(events.Playlists)
	renamed := ts.send(t, call{method: http.MethodPatch, path: playlistPath(p.ID), body: `{"name":"Rocks"}`, header: jsonHeader(sess.token)})
	if renamed.Status != http.StatusOK {
		t.Fatalf("rename = %d", renamed.Status)
	}
	d := ts.playlist(t, sess.token, p.ID)
	if d.Name != "Rocks" || !slices.Equal(entryTracks(&d), []string{songAt(1, 1), songAt(1, 2), songAt(1, 3)}) {
		t.Errorf("after the change = %s %v", d.Name, entryTracks(&d))
	}
}

// Starring a song brings it into a smart playlist, with nothing to refresh.
func TestSmartPlaylistFollowsStars(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	if err := ts.Store.Rate(t.Context(), sess.user.ID, songAt(2, 1), rating.Rating(10)); err != nil {
		t.Fatal(err)
	}
	ratedNow := ts.makeSmart(t, sess.token, "Rated again", ratedRules)
	if ratedNow.SongCount != 3 {
		t.Errorf("after a new star = %d songs", ratedNow.SongCount)
	}
}

// The preview counts, times and lists what rules would choose, the limit
// applied, and follows the seed it is given or the one being edited.
func TestSmartPreview(t *testing.T) {
	t.Parallel()
	ts, sess, other := smartFixture(t)
	p := ts.preview(t, sess.token, `{"rules":`+limitRules+`}`)
	if p.SongCount != 2 || p.MatchCount != 6 || len(p.Songs) != 2 || p.Songs[0].AlbumID == "" || p.Songs[0].Title == "" {
		t.Fatalf("preview = %+v", p)
	}
	random := `{"match":"all","rules":[],"order":"random"}`
	seeded := ts.preview(t, sess.token, `{"rules":`+random+`,"seed":7}`)
	if again := ts.preview(t, sess.token, `{"rules":`+random+`,"seed":7}`); !slices.Equal(previewIDs(&seeded), previewIDs(&again)) {
		t.Error("the same seed shuffled differently")
	}
	made := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: "Shuffled", Rules: []byte(random), Seed: new(int64(7))})
	editing := ts.preview(t, sess.token, `{"rules":`+random+`,"id":"`+made.Playlist.ID+`"}`)
	d := ts.playlist(t, sess.token, made.Playlist.ID)
	if !slices.Equal(previewIDs(&editing), entryTracks(&d)) || !slices.Equal(previewIDs(&editing), previewIDs(&seeded)) {
		t.Errorf("editing follows the playlist's seed: %v, playlist %v", previewIDs(&editing), entryTracks(&d))
	}
	h := jsonHeader(sess.token)
	ts.send(t, call{method: http.MethodPost, path: previewPath, body: `{"rules":` + brokenRules + `}`, header: h}).
		problem(t, http.StatusUnprocessableEntity)
	ts.send(t, call{method: http.MethodPost, path: previewPath, body: `{"rules":` + ratedRules + `,"id":"` + made.Playlist.ID + `"}`, header: jsonHeader(other)}).
		problem(t, http.StatusNotFound)
	ts.send(t, call{method: http.MethodPost, path: previewPath, body: `{"rules":`, header: h, invalid: true}).
		problem(t, http.StatusBadRequest)
	ts.send(t, call{method: http.MethodPost, path: previewPath, body: `{"rules":{}}`, header: bearer("nope"), invalid: true}).
		problem(t, http.StatusUnauthorized)
}

// What a smart playlist can't be, or do, is refused.
func TestSmartPlaylistRefusals(t *testing.T) {
	t.Parallel()
	ts, sess, other := smartFixture(t)
	h := jsonHeader(sess.token)
	smart := ts.makeSmart(t, sess.token, "Rated", ratedRules)
	regular := ts.makePlaylist(t, sess.token, &api.PlaylistCreate{Name: mixName, Tracks: []string{songAt(1, 1)}}).Playlist
	// Rules it can't have, and rules with songs.
	ts.send(t, call{method: http.MethodPost, path: playlistsPath, body: `{"name":"x","rules":` + brokenRules + `}`, header: h}).
		problem(t, http.StatusUnprocessableEntity)
	ts.send(t, call{method: http.MethodPost, path: playlistsPath, body: `{"name":"x","rules":` + ratedRules + `,"tracks":["` + songAt(1, 1) + `"]}`, header: h}).
		problem(t, http.StatusUnprocessableEntity)
	ts.send(t, call{method: http.MethodPatch, path: playlistPath(smart.ID), body: `{"rules":` + brokenRules + `}`, header: h}).
		problem(t, http.StatusUnprocessableEntity)
	// Rules on a regular playlist, and a seed.
	ts.send(t, call{method: http.MethodPatch, path: playlistPath(regular.ID), body: `{"rules":` + ratedRules + `}`, header: h}).
		problem(t, http.StatusConflict)
	ts.send(t, call{method: http.MethodPost, path: playlistPath(regular.ID, "seed"), header: bearer(sess.token)}).
		problem(t, http.StatusConflict)
	// Its entries, which it doesn't have.
	ts.send(t, call{method: http.MethodPost, path: playlistPath(smart.ID, "tracks"), body: tracksBody(songAt(1, 1)), header: h}).
		problem(t, http.StatusConflict)
	ts.send(t, call{method: http.MethodDelete, path: playlistPath(smart.ID, "entries"), body: `{"entries":["1"]}`, header: h}).
		problem(t, http.StatusConflict)
	ts.send(t, call{method: http.MethodPost, path: playlistPath(smart.ID, "move"), body: `{"entry":"1","after":"2"}`, header: h}).
		problem(t, http.StatusConflict)
	// A cycle, which the problem names.
	inSmart := func(id string) string {
		return `{"match":"all","rules":[{"field":"playlist","op":"is","value":"` + id + `"}]}`
	}
	second := ts.makeSmart(t, sess.token, "In Rated", inSmart(smart.ID))
	resp := ts.send(t, call{method: http.MethodPatch, path: playlistPath(smart.ID), body: `{"rules":` + inSmart(second.ID) + `}`, header: h})
	var cycle api.SmartProblem
	if resp.Status != http.StatusUnprocessableEntity || json.Unmarshal(resp.Body, &cycle) != nil ||
		!slices.Equal(cycle.Cycle, []string{smart.ID, second.ID, smart.ID}) {
		t.Errorf("a cycle = %d %s", resp.Status, resp.Body)
	}
	// Another person's is a 404 to every one of them.
	oh := jsonHeader(other)
	for _, c := range []call{
		{method: http.MethodGet, path: playlistPath(smart.ID), header: bearer(other)},
		{method: http.MethodPatch, path: playlistPath(smart.ID), body: `{"rules":` + ratedRules + `}`, header: oh},
		{method: http.MethodPost, path: playlistPath(smart.ID, "seed"), header: bearer(other)},
		{method: http.MethodPost, path: playlistPath(smart.ID, "copy"), header: bearer(other)},
		{method: http.MethodPost, path: playlistPath(smart.ID, "tracks"), body: tracksBody(songAt(1, 1)), header: oh},
	} {
		ts.send(t, c).problem(t, http.StatusNotFound)
	}
	for _, path := range []string{playlistPath("x", "seed"), playlistPath("x", "copy")} {
		ts.send(t, call{method: http.MethodPost, path: path, header: bearer("nope")}).problem(t, http.StatusUnauthorized)
	}
}

// Pick again draws a new seed; Copy to a playlist makes a regular one holding
// the songs as they are now.
func TestSmartSeedAndCopy(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	smart := ts.makeSmart(t, sess.token, "Rated", ratedRules)
	sub := ts.openStream(t, sess.token, "")
	sub.want(events.Ready)
	seeded := ts.send(t, call{method: http.MethodPost, path: playlistPath(smart.ID, "seed"), header: bearer(sess.token)})
	if seeded.Status != http.StatusOK {
		t.Fatalf("seed = %d", seeded.Status)
	}
	sub.want(events.Playlists)
	resp := ts.send(t, call{method: http.MethodPost, path: playlistPath(smart.ID, "copy"), header: bearer(sess.token)})
	var c api.Playlist
	if resp.Status != http.StatusCreated || json.Unmarshal(resp.Body, &c) != nil {
		t.Fatalf("copy = %d %s", resp.Status, resp.Body)
	}
	sub.want(events.Playlists)
	if c.Smart || c.Name != "Rated (copy)" || c.SongCount != 2 {
		t.Errorf("copy = %+v", c)
	}
	d := ts.playlist(t, sess.token, c.ID)
	if got := entryTracks(&d); !slices.Equal(got, []string{songAt(1, 3), songAt(2, 2)}) {
		t.Errorf("the copy holds %v", got)
	}
	// It is a regular playlist now: songs go in.
	ts.send(t, call{method: http.MethodPost, path: playlistPath(c.ID, "tracks"), body: tracksBody(songAt(1, 1)), header: jsonHeader(sess.token)})
	if d = ts.playlist(t, sess.token, c.ID); len(d.Entries) != 3 {
		t.Errorf("after an add the copy holds %d", len(d.Entries))
	}
}

// A copy past what a playlist holds is a 422, and nothing is made.
func TestSmartCopyTooLarge(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.seedBench(t, store.MostPlaylistEntries+1, 100, 50)
	smart := ts.makeSmart(t, token, "Everything", `{"match":"any","rules":[]}`)
	ts.send(t, call{method: http.MethodPost, path: playlistPath(smart.ID, "copy"), header: bearer(token)}).
		problem(t, http.StatusUnprocessableEntity)
	var list api.Playlists
	ts.getJSON(t, token, playlistsPath, &list)
	if len(list.Playlists) != 1 {
		t.Errorf("%d playlists, want only the smart one", len(list.Playlists))
	}
}

// The shuffle scope of a smart playlist is the songs its rules choose.
func TestShuffleASmartPlaylist(t *testing.T) {
	t.Parallel()
	ts, sess, _ := smartFixture(t)
	smart := ts.makeSmart(t, sess.token, "Rated", ratedRules)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: string(shuffler.Playlist), ID: smart.ID}, Mode: modeFinite})
	got := songsOf(&page)
	slices.Sort(got)
	if !slices.Equal(got, []string{songAt(1, 3), songAt(2, 2)}) {
		t.Errorf("shuffled %v, want the two rated songs", got)
	}
}
