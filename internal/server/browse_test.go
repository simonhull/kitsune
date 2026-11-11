package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	albumsPath  = "/albums"
	libraryPath = "/library"
	flacFormat  = "flac"
	annName     = "Ann"
	annID       = "03J00000000000000000000000"
	missingID   = "nope"
	lowerRock   = "rock"
)

// browseAlbum is an album the browse tests seed: tracks track rows, an optional cover.
type browseAlbum struct {
	n        int
	sortName string
	tracks   int
	cover    *store.ArtworkWrite
	genres   []string // Raw values, on every track.
}

func albumULID(n int) string { return fmt.Sprintf("02J%023d", n) }

// genreTags returns the raw tags that hold genres.
func genreTags(genres []string) []store.Tag {
	tags := make([]store.Tag, len(genres))
	for i, genre := range genres {
		tags[i] = store.Tag{Key: library.GenreKeys()[0], Value: genre}
	}
	return tags
}

// seedAlbums writes the albums, each track 1 minute long.
func (ts *testServer) seedAlbums(tb testing.TB, albums ...browseAlbum) {
	tb.Helper()
	var tws []store.TrackWrite
	for _, a := range albums {
		tws = append(tws, a.writes()...)
	}
	if err := ts.Store.WriteBatch(tb.Context(), &store.Batch{Tracks: tws}); err != nil {
		tb.Fatal(err)
	}
}

// writes returns the track writes of a.
func (a *browseAlbum) writes() []store.TrackWrite {
	tws := make([]store.TrackWrite, 0, a.tracks)
	for t := 1; t <= a.tracks; t++ {
		tws = append(tws, store.TrackWrite{
			ULID:    fmt.Sprintf("01J%020d%03d", a.n, t),
			Root:    "/music",
			RelPath: fmt.Sprintf("a%d/%d.flac", a.n, t),
			Digest:  new(sha256.Sum256(fmt.Appendf(nil, "%d/%d", a.n, t))),
			Format:  flacFormat, Codec: flacFormat,
			Duration: time.Minute,
			Title:    fmt.Sprintf("Track %d", t),
			TrackNo:  t,
			Year:     2001,
			Tags:     genreTags(a.genres),
			Artists:  []store.ArtistWrite{{ULID: annID, MatchKey: "ann", Name: annName, SortName: annName}},
			Album: store.AlbumWrite{
				GroupKey: fmt.Sprintf("g%d", a.n), ULID: albumULID(a.n), Name: fmt.Sprintf("Album %d", a.n),
				SortName: a.sortName, Artist: annName, Year: 2001, Artwork: a.cover,
			},
		})
	}
	return tws
}

// getJSON sends an authenticated GET and decodes a 200 response into v.
func (ts *testServer) getJSON(t *testing.T, token, path string, v any) {
	t.Helper()
	resp := ts.send(t, call{method: http.MethodGet, path: path, header: bearer(token)})
	if resp.Status != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200: %s", path, resp.Status, resp.Body)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		t.Fatalf("decode %s: %v", resp.Body, err)
	}
}

// checkEntries checks the two entries of the name-sorted first page.
func checkEntries(t *testing.T, albums []api.AlbumEntry, cover *store.ArtworkWrite) {
	t.Helper()
	art := &api.Artwork{Hash: hex.EncodeToString(cover.Hash[:]), Accent: "#aabbcc", Ground: "#112233", Width: 600, Height: 500}
	want := []api.AlbumEntry{
		{ID: albumULID(2), Title: "Album 2", SortName: "A", Artist: annName, SortArtist: annName, Year: 2001, TrackCount: 1, DisplayGenres: []string{}},
		{ID: albumULID(1), Title: "Album 1", SortName: "b", Artist: annName, SortArtist: annName, Year: 2001, TrackCount: 2, DisplayGenres: []string{}, Artwork: art},
	}
	for i := range albums {
		if albums[i].AddedAt.IsZero() {
			t.Errorf("album %d has no addedAt", i)
		}
		albums[i].AddedAt = time.Time{}
	}
	if !reflect.DeepEqual(albums, want) {
		t.Errorf("albums = %+v, want %+v", albums, want)
	}
}

func TestAlbumsListsAndPages(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	cover := &store.ArtworkWrite{
		Hash: sha256.Sum256([]byte("c")), Width: 600, Height: 500, Accent: "#aabbcc", Ground: "#112233", Source: store.ArtworkEmbedded,
	}
	ts.seedAlbums(t,
		browseAlbum{n: 1, sortName: "b", tracks: 2, cover: cover},
		browseAlbum{n: 2, sortName: "A", tracks: 1},
		browseAlbum{n: 3, sortName: "c", tracks: 1},
	)

	var page api.AlbumPage
	ts.getJSON(t, token, albumsPath+"?sort=name&limit=2", &page)
	if len(page.Albums) != 2 || page.NextCursor == "" {
		t.Fatalf("page 1 = %+v, want 2 albums and a cursor", page)
	}
	checkEntries(t, page.Albums, cover)

	var rest api.AlbumPage
	ts.getJSON(t, token, albumsPath+"?sort=name&limit=2&cursor="+url.QueryEscape(page.NextCursor), &rest)
	if len(rest.Albums) != 1 || rest.Albums[0].ID != albumULID(3) || rest.NextCursor != "" {
		t.Errorf("page 2 = %+v, want album 3 and no cursor", rest)
	}
}

func TestAlbumsDefaults(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "b", tracks: 1}, browseAlbum{n: 2, sortName: "a", tracks: 1})
	var def api.AlbumPage // Defaults: added, 60.
	ts.getJSON(t, token, albumsPath, &def)
	if len(def.Albums) != 2 || def.NextCursor != "" {
		t.Errorf("default page = %+v, want both albums", def)
	}
	if def.Albums[0].ID != albumULID(2) {
		t.Errorf("first = %q, want the newest album, %q", def.Albums[0].ID, albumULID(2))
	}
}

func TestAlbumsBadRequests(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1}, browseAlbum{n: 2, sortName: "b", tracks: 1})

	var byName, byAdded api.AlbumPage
	ts.getJSON(t, token, albumsPath+"?sort=name&limit=1", &byName)
	ts.getJSON(t, token, albumsPath+"?limit=1", &byAdded)

	tests := []struct {
		name, query string
		invalid     bool // The request itself breaks the contract.
	}{
		{"garbage cursor", "?cursor=!!!", false},
		{"cursor of the other sort", "?sort=added&cursor=" + url.QueryEscape(byName.NextCursor), false},
		{"name sort with an added cursor", "?sort=name&cursor=" + url.QueryEscape(byAdded.NextCursor), false},
		{"limit 0", "?limit=0", true},
		{"limit 201", "?limit=201", true},
		{"limit not a number", "?limit=ten", true},
		{"unknown sort", "?sort=year", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := ts.send(t, call{method: http.MethodGet, path: albumsPath + tc.query, header: bearer(token), invalid: tc.invalid})
			if p := resp.problem(t, http.StatusBadRequest); p.Detail == "" {
				t.Error("problem has no detail")
			}
		})
	}
}

func TestAlbumDetail(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 3})

	var d api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &d)
	if d.ID != albumULID(1) || d.TrackCount != 3 || len(d.Tracks) != 3 || d.DurationMs != 3*60_000 || d.Artwork != nil {
		t.Errorf("detail = %+v", d)
	}
	if !slices.Equal(d.Formats, []string{flacFormat}) || d.AddedAt.IsZero() {
		t.Errorf("formats = %v, addedAt = %v", d.Formats, d.AddedAt)
	}
	checkDetailTrack(t, &d.Tracks[1])
}

func checkDetailTrack(t *testing.T, tr *api.Track) {
	t.Helper()
	if tr.Title != "Track 2" || tr.TrackNo != 2 || tr.DurationMs != 60_000 || tr.Format != flacFormat ||
		!slices.Equal(tr.Artists, []string{annName}) || tr.ReplayGain != nil || tr.SampleRate != 0 {
		t.Errorf("track = %+v", tr)
	}
	if tr.ID != fmt.Sprintf("01J%020d%03d", 1, 2) {
		t.Errorf("track ID = %q, want the track's ULID", tr.ID)
	}
}

func TestAlbumsShowDisplayGenres(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t,
		browseAlbum{n: 1, sortName: "a", tracks: 1, genres: []string{"Pop, Rock", "(12)Spiritual"}},
		browseAlbum{n: 2, sortName: "b", tracks: 2, genres: []string{lowerRock}},
		browseAlbum{n: 3, sortName: "c", tracks: 1},
	)
	if err := ts.Handle.genres.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	var page api.AlbumPage
	ts.getJSON(t, token, albumsPath+"?sort=name", &page)
	want := [][]string{{"Pop", lowerRock, "Spiritual"}, {lowerRock}, {}}
	for i := range page.Albums {
		if got := page.Albums[i].DisplayGenres; !slices.Equal(got, want[i]) {
			t.Errorf("album %d: displayGenres = %q, want %q", i, got, want[i])
		}
	}
	var album api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &album)
	if want := []string{"Pop, Rock", "(12)Spiritual"}; !slices.Equal(album.Genres, want) {
		t.Errorf("genres = %q, want raw %q", album.Genres, want)
	}
	if !slices.Equal(album.DisplayGenres, page.Albums[0].DisplayGenres) {
		t.Errorf("detail displayGenres = %q, list %q", album.DisplayGenres, page.Albums[0].DisplayGenres)
	}
}

func TestAlbumDetailNotFound(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1})
	states, err := ts.Store.FileStates(t.Context(), "/music")
	if err != nil {
		t.Fatal(err)
	}
	if err = ts.Store.MarkMissing(t.Context(), []int64{states["a1/1.flac"].TrackID}, ts.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{albumULID(1), albumULID(99), missingID} {
		resp := ts.send(t, call{method: http.MethodGet, path: albumsPath + "/" + id, header: bearer(token)})
		resp.problem(t, http.StatusNotFound)
	}
}

func TestLibraryFacts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Roots: 2})
	token := ts.sessionFor(t).token
	var empty api.LibraryFacts
	ts.getJSON(t, token, libraryPath, &empty)
	if empty != (api.LibraryFacts{Roots: 2, Scan: api.ScanStatus{State: api.ScanIdle}}) {
		t.Errorf("empty library = %+v, want only roots: 2", empty)
	}
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2}, browseAlbum{n: 2, sortName: "b", tracks: 1})
	var got api.LibraryFacts
	ts.getJSON(t, token, libraryPath, &got)
	if want := (api.LibraryFacts{Albums: 2, Tracks: 3, DurationMs: 3 * 60_000, Roots: 2, Scan: api.ScanStatus{State: api.ScanIdle}}); got != want {
		t.Errorf("facts = %+v, want %+v", got, want)
	}
}

func TestBrowseRequiresAuth(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	for _, path := range []string{albumsPath, albumsPath + "/" + albumULID(1), libraryPath} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodGet, path: path, invalid: true}).problem(t, http.StatusUnauthorized)
		})
	}
}

func TestNewPanicsWithoutLibrary(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	defer func() {
		if recover() == nil {
			t.Error("New without Library did not panic")
		}
	}()
	New(Config{Auth: ts.Auth})
}

func TestToAPIAlbumKeepsEmptyListsNonNull(t *testing.T) {
	t.Parallel()
	d := toAPIAlbumDetail(&store.AlbumDetail{Tracks: []store.Track{{}}}, nil)
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	tracks, _ := m["tracks"].([]any)
	if len(tracks) != 1 {
		t.Fatalf("tracks = %v", m["tracks"])
	}
	if artists, ok := tracks[0].(map[string]any)["artists"].([]any); !ok || len(artists) != 0 {
		t.Errorf("artists = %v, want []", tracks[0])
	}
	if f, ok := m["formats"].([]any); !ok || len(f) != 0 {
		t.Errorf("formats = %v, want []", m["formats"])
	}
	if g, ok := m["genres"].([]any); !ok || len(g) != 0 {
		t.Errorf("genres = %v, want []", m["genres"])
	}
}
