package server

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	fileOne    = "album/one.flac"
	fileTwo    = "album/two.flac"
	sampleFLAC = "sample.flac"
	sampleMP3  = "sample_v24.mp3"
	mp3File    = "album/one.mp3"
	yearBody   = `{"all":{"set":{"date":["1987"]}}}`
	newTitle   = "New Title"
	keyGenre   = "GENRE"
	ifMatch    = "If-Match"
	staleETag  = `"0123456789abcdef0123456789abcdef"`
)

// editFixture is a server over a scanned temp library of one album.
type editFixture struct {
	*testServer

	root  string
	token string
	album api.AlbumDetail
}

// newEditFixture copies samples (path under the root to testdata name) to a temp
// root, scans it and signs in an admin.
func newEditFixture(t *testing.T, opts *testServerOptions, samples map[string]string) *editFixture {
	t.Helper()
	root := t.TempDir()
	for rel, sample := range samples {
		b, err := os.ReadFile(filepath.Join("..", "library", "testdata", sample))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, rel)
		if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // G703: under the test's temp root.
			t.Fatal(err)
		}
	}
	opts.LibraryRoot = root
	ts := newTestServer(t, *opts)
	if _, err := ts.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	fx := &editFixture{testServer: ts, root: root, token: ts.tokenFor(t, store.RoleAdmin)}
	fx.reload(t)
	return fx
}

func twoFLACs() map[string]string { return map[string]string{fileOne: sampleFLAC, fileTwo: sampleFLAC} }

// reload reads the library's only album again.
func (fx *editFixture) reload(t *testing.T) {
	t.Helper()
	var page api.AlbumPage
	fx.getJSON(t, fx.token, albumsPath, &page)
	if len(page.Albums) != 1 {
		t.Fatalf("albums = %+v, want 1", page.Albums)
	}
	fx.album = api.AlbumDetail{}
	fx.getJSON(t, fx.token, albumsPath+"/"+page.Albums[0].ID, &fx.album)
}

func (fx *editFixture) albumPath() string { return albumsPath + "/" + fx.album.ID }

func (fx *editFixture) editsPath() string { return fx.albumPath() + "/edits" }

// trackBody is an edit of the album's track i, with set as its "set" member.
func (fx *editFixture) trackBody(i int, set string) string {
	return `{"tracks":[{"id":"` + fx.album.Tracks[i].ID + `","set":` + set + `}]}`
}

// post sends an authenticated edit with etag as If-Match, unless it is empty.
func (fx *editFixture) post(t *testing.T, etag, body string) response {
	t.Helper()
	return fx.send(t, fx.postCall(fx.editsPath(), etag, body))
}

func (fx *editFixture) postCall(path, etag, body string) call {
	h := bearer(fx.token)
	h.Set(contentType, api.JSONMediaType)
	if etag != "" {
		h.Set(ifMatch, etag)
	}
	return call{method: http.MethodPost, path: path, body: body, header: h}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tagsOf opens the file at path and returns its tags. It also proves it parses.
func tagsOf(t *testing.T, path string) audiometa.Tags {
	t.Helper()
	f, err := audiometa.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close() //nolint:errcheck // Read-only use is done; nothing to report.
	return f.Tags
}

// decodeEditProblem decodes r as a stopped edit with the given status.
func decodeEditProblem(t *testing.T, r response, status int) api.EditProblem {
	t.Helper()
	r.problem(t, status)
	var p api.EditProblem
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatalf("decode %q: %v", r.Body, err)
	}
	return p
}

func TestEditWithWritesOffChangesNothing(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	path := filepath.Join(fx.root, fileOne)
	before := readFile(t, path)
	fx.post(t, fx.album.Etag, yearBody).problem(t, http.StatusForbidden)
	if !bytes.Equal(before, readFile(t, path)) {
		t.Error("the file changed with tag writes off")
	}
}

func TestEditIsForAdmins(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	etag := fx.album.Etag
	fx.token = fx.tokenFor(t, store.RoleUser)
	fx.post(t, etag, yearBody).problem(t, http.StatusForbidden)
	fx.token = "not a token"
	fx.post(t, etag, yearBody).problem(t, http.StatusUnauthorized)
}

// decode fails the test unless r is a 200, and decodes its body into v.
func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatal(err)
	}
}

// wantStoreFollowed checks the album kept its identity, took got's ETag, and that
// a quick scan finds nothing to read.
func (fx *editFixture) wantStoreFollowed(t *testing.T, before, got *api.AlbumDetail) {
	t.Helper()
	fx.reload(t)
	if fx.album.ID != before.ID || !fx.album.AddedAt.Equal(before.AddedAt) || len(fx.album.Tracks) != 2 {
		t.Errorf("album = %+v, want it kept, with its 2 tracks", fx.album)
	}
	if fx.album.Etag != got.Etag || got.Etag == before.Etag {
		t.Errorf("album etag = %s, want the response's %s, not %s", fx.album.Etag, got.Etag, before.Etag)
	}
	stats, err := fx.Scanner.Scan(t.Context(), library.Quick)
	if err != nil || stats.Read != 0 {
		t.Errorf("quick scan read %d files, err %v: want none, the store already matches", stats.Read, err)
	}
}

func TestEditOfOneTrackSavesAndReturnsTheAlbum(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	before := fx.album
	var got api.AlbumDetail
	fx.post(t, before.Etag, fx.trackBody(0, `{"title":["  New Title "],"track":["7"],"disc":["2"]}`)).decode(t, &got)
	i := slices.IndexFunc(got.Tracks, func(tr api.Track) bool { return tr.ID == before.Tracks[0].ID })
	if tr := &got.Tracks[i]; tr.Title != newTitle || tr.TrackNo != 7 || tr.DiscNo != 2 {
		t.Errorf("track = %+v", tr)
	}
	if other := &got.Tracks[1-i]; other.Title == newTitle || other.Etag != before.Tracks[1].Etag {
		t.Errorf("the other track = %+v, want it left alone", other)
	}
	fx.wantStoreFollowed(t, &before, &got)
}

// wantGenres checks the album and each of its tracks hold genres.
func wantGenres(t *testing.T, album *api.AlbumDetail, genres []string) {
	t.Helper()
	if !slices.Equal(album.Genres, genres) {
		t.Errorf("album genres = %q, want %q", album.Genres, genres)
	}
	for i := range album.Tracks {
		if g := album.Tracks[i].Genres; !slices.Equal(g, genres) {
			t.Errorf("track genres = %q, want %q", g, genres)
		}
	}
}

func TestEditOfAllAndOneTrackInOneRequest(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	before := fx.album
	body := `{"all":{"set":{"date":["1987"],"genres":["Jazz"," Blues "]}},` +
		`"tracks":[{"id":"` + before.Tracks[1].ID + `","set":{"title":["New Title"]}}]}`
	var got api.AlbumDetail
	fx.post(t, before.Etag, body).decode(t, &got)
	genres := []string{"Jazz", "Blues"}
	if got.Year != 1987 {
		t.Errorf("album year = %d", got.Year)
	}
	wantGenres(t, &got, genres)
	for _, rel := range []string{fileOne, fileTwo} {
		if tags := tagsOf(t, filepath.Join(fx.root, rel)); tags.Year != 1987 || !slices.Equal(tags.Genres, genres) {
			t.Errorf("%s: year %d, genres %q", rel, tags.Year, tags.Genres)
		}
	}
	titled := 0
	for i := range got.Tracks {
		if got.Tracks[i].Title == newTitle {
			titled++
		}
	}
	if titled != 1 {
		t.Errorf("%d tracks took the title, want 1", titled)
	}
	fx.wantStoreFollowed(t, &before, &got)
	// The response's ETag is current: the next save goes straight on.
	if r := fx.post(t, got.Etag, yearBody); r.Status != http.StatusOK {
		t.Errorf("chained edit = %d: %s", r.Status, r.Body)
	}
}

func TestEditClearsFields(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	fx.retag(t, map[string][]string{"DATE": {"2001-05-12"}, "YEAR": {"2001"}, keyGenre: {"Rock"}, "COMPOSER": {"Someone"}})
	kept := fx.album.Tracks[1].ID
	body := `{"all":{"clear":["date","composers"]},"tracks":[{"id":"` + kept + `","set":{"composers":["Someone Else"]}}]}`
	var got api.AlbumDetail
	fx.post(t, fx.album.Etag, body).decode(t, &got)
	if got.Year != 0 {
		t.Errorf("album year = %d, want none", got.Year)
	}
	fx.reload(t)
	tags := fx.albumTags(t)
	for i := range tags.Tracks {
		tr := &tags.Tracks[i]
		want := []string(nil)
		if tr.ID == kept {
			want = []string{"Someone Else"}
		}
		if v := tr.Fields["composers"]; !slices.Equal(v, want) {
			t.Errorf("composers = %q, want %q", v, want)
		}
		if v := tr.Fields["date"]; v != nil {
			t.Errorf("date = %q, want none", v)
		}
		if v := tr.Fields["genres"]; !slices.Equal(v, []string{"Rock"}) {
			t.Errorf("genres = %q, want them left alone", v)
		}
	}
}

func TestEditPreconditions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		etag func(string) string
		want int
	}{
		{"no If-Match", func(string) string { return "" }, http.StatusPreconditionRequired},
		{"star", func(string) string { return "*" }, http.StatusPreconditionRequired},
		{"stale", func(string) string { return staleETag }, http.StatusPreconditionFailed},
		{"weak", func(s string) string { return "W/" + s }, http.StatusPreconditionFailed},
		{"a list", func(s string) string { return s + ", " + staleETag }, http.StatusPreconditionFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
			path := filepath.Join(fx.root, fileOne)
			before := readFile(t, path)
			fx.post(t, tt.etag(fx.album.Etag), yearBody).problem(t, tt.want)
			if !bytes.Equal(before, readFile(t, path)) {
				t.Error("a refused edit changed the file")
			}
		})
	}
}

func TestEditOfAFileThatDriftedIsStale(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	for _, rel := range []string{fileOne, fileTwo} {
		err := audiometa.Write(filepath.Join(fx.root, rel), func(f *audiometa.File) error {
			f.Tags.Comment = "changed behind the server's back"
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	fx.post(t, fx.album.Etag, yearBody).problem(t, http.StatusPreconditionFailed)
}

func TestEditOfSomethingUnknownIs404(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	fx.send(t, fx.postCall(albumsPath+"/"+missingID+"/edits", staleETag, yearBody)).problem(t, http.StatusNotFound)
}

func TestEditBadBodies(t *testing.T) {
	t.Parallel()
	twice := func(fx *editFixture) string {
		id := fx.album.Tracks[0].ID
		return `{"tracks":[{"id":"` + id + `","set":{"title":["a"]}},{"id":"` + id + `","set":{"title":["b"]}}]}`
	}
	tests := []struct {
		name    string
		body    func(*editFixture) string
		invalid bool // The contract refuses it too.
	}{
		{name: "empty edit", body: func(*editFixture) string { return `{}` }},
		{name: "nothing set", body: func(*editFixture) string { return `{"all":{"set":{}}}` }},
		{name: "a track with no change", body: func(fx *editFixture) string { return fx.trackBody(0, `{}`) }},
		{name: "a track named twice", body: twice},
		{name: "a field set and cleared", body: func(*editFixture) string { return `{"all":{"set":{"date":["1987"]},"clear":["date"]}}` }},
		{name: "a track's field set and cleared", body: func(fx *editFixture) string {
			return `{"tracks":[{"id":"` + fx.album.Tracks[0].ID + `","set":{"date":["1987"]},"clear":["date"]}]}`
		}},
		{name: "not JSON", body: func(*editFixture) string { return `title` }, invalid: true},
		{name: "an old edit", body: func(*editFixture) string { return `{"year":1987}` }, invalid: true},
		{name: "no track ID", body: func(*editFixture) string { return `{"tracks":[{"set":{"title":["x"]}}]}` }, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
			c := fx.postCall(fx.editsPath(), fx.album.Etag, tt.body(fx))
			c.invalid = tt.invalid
			fx.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
}

func TestEditCapsTheBody(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	big := `{"all":{"set":{"genres":["` + strings.Repeat("x", 1<<20) + `"]}}}`
	fx.post(t, fx.album.Etag, big).problem(t, http.StatusBadRequest)
}

func TestEditNeedsJSON(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	c := fx.postCall(fx.editsPath(), fx.album.Etag, yearBody)
	c.header.Set(contentType, "text/plain")
	c.invalid = true
	fx.send(t, c).problem(t, http.StatusBadRequest)
}

// holdingWrite returns a write that signals entered and waits for release
// before really writing.
func holdingWrite(entered chan<- struct{}, release <-chan struct{}) library.WriteFunc {
	return func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		entered <- struct{}{}
		<-release
		return audiometa.WriteContext(ctx, path, edit)
	}
}

func TestEditDuringAnEditIsBusy(t *testing.T) {
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
	resp := fx.post(t, fx.album.Etag, yearBody)
	resp.problem(t, http.StatusConflict)
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Error("no Retry-After")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestEditRefusedByTheFile(t *testing.T) {
	t.Parallel()
	mp3Album := map[string]string{mp3File: sampleMP3}
	tests := []struct {
		name    string
		samples map[string]string
		body    string
		prepare func(*testing.T, *editFixture)
		want    int
		invalid bool // The contract refuses it too.
	}{
		{name: "blank title", samples: twoFLACs(), body: `{"all":{"set":{"title":["  "]}}}`, want: http.StatusUnprocessableEntity},
		{name: "no such field", samples: twoFLACs(), body: `{"all":{"set":{"nope":["x"]}}}`, want: http.StatusUnprocessableEntity, invalid: true},
		{name: "a track number of zero", samples: twoFLACs(), body: `{"all":{"set":{"track":["0"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a track number past 16 bits", samples: twoFLACs(), body: `{"all":{"set":{"track":["65536"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a date that isn't one", samples: twoFLACs(), body: `{"all":{"set":{"date":["1987-13"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a year too small", samples: twoFLACs(), body: `{"all":{"set":{"date":["0999"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "two titles", samples: twoFLACs(), body: `{"all":{"set":{"title":["a","b"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a track of another album", samples: twoFLACs(), body: `{"tracks":[{"id":"` + missingID + `","set":{"title":["a"]}}]}`, want: http.StatusUnprocessableEntity},
		{name: "clearing the title", samples: twoFLACs(), body: `{"all":{"clear":["title"]}}`, want: http.StatusUnprocessableEntity},
		{name: "clearing an identity field", samples: twoFLACs(), body: `{"all":{"clear":["album"]}}`, want: http.StatusUnprocessableEntity},
		{name: "an identity field", samples: twoFLACs(), body: `{"all":{"set":{"album":["x"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a year MP3 can't hold", samples: mp3Album, body: `{"all":{"set":{"date":["1850"]}}}`, want: http.StatusUnprocessableEntity},
		{name: "a read-only file", samples: twoFLACs(), body: yearBody, want: http.StatusForbidden, prepare: readOnly(fileTwo)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newEditFixture(t, &testServerOptions{TagWrites: true}, tt.samples)
			if tt.prepare != nil {
				tt.prepare(t, fx)
			}
			c := fx.postCall(fx.editsPath(), fx.album.Etag, tt.body)
			c.invalid = tt.invalid
			p := decodeEditProblem(t, fx.send(t, c), tt.want)
			if len(p.Changed) != 0 || len(p.Remaining) == 0 {
				t.Errorf("changed %q, remaining %q: a preflight failure changes nothing", p.Changed, p.Remaining)
			}
			if tags := tagsOf(t, filepath.Join(fx.root, firstFile(tt.samples))); tags.Title == newTitle || tags.Year == 1987 {
				t.Errorf("a refused edit reached the file: %q %d", tags.Title, tags.Year)
			}
		})
	}
}

// firstFile returns the first path of samples, sorted.
func firstFile(samples map[string]string) string {
	return slices.Sorted(maps.Keys(samples))[0]
}

// readOnly makes the files read-only, keeping their times, so only the mode differs.
func readOnly(rels ...string) func(*testing.T, *editFixture) {
	return func(t *testing.T, fx *editFixture) {
		t.Helper()
		for _, rel := range rels {
			if err := os.Chmod(filepath.Join(fx.root, rel), 0o444); err != nil { //nolint:gosec // G302: read-only is the point.
				t.Fatal(err)
			}
		}
	}
}

// failingWrite really writes the first n files, then fails.
func failingWrite(n int32) library.WriteFunc {
	var writes atomic.Int32
	return func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		if writes.Add(1) > n {
			return errors.New("disk full")
		}
		return audiometa.WriteContext(ctx, path, edit)
	}
}

func TestAlbumEditStoppedPartwayReportsHowFarItGot(t *testing.T) {
	t.Parallel()
	samples := map[string]string{"album/1.flac": sampleFLAC, "album/2.flac": sampleFLAC, "album/3.flac": sampleFLAC}
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, Write: failingWrite(1)}, samples)
	p := decodeEditProblem(t, fx.post(t, fx.album.Etag, yearBody), http.StatusInternalServerError)
	if len(p.Changed) != 1 || len(p.Remaining) != 2 {
		t.Fatalf("changed %q, remaining %q: want 1 and 2", p.Changed, p.Remaining)
	}
	if strings.Contains(p.Detail, "disk full") {
		t.Errorf("detail = %q: an internal cause must stay in the log", p.Detail)
	}
	years := 0
	for rel := range samples {
		if tagsOf(t, filepath.Join(fx.root, rel)).Year == 1987 {
			years++
		}
	}
	if years != 1 {
		t.Errorf("%d files hold the edit, want 1", years)
	}
	fx.reload(t)
	for i := range fx.album.Tracks {
		tr := &fx.album.Tracks[i]
		if changed := slices.Contains(p.Changed, tr.ID); changed != (tr.Year == 1987) {
			t.Errorf("track year %d, reported changed = %t: the store must follow the files", tr.Year, changed)
		}
	}
}

func TestRetryAfterAStopFinishesTheEdit(t *testing.T) {
	t.Parallel()
	samples := map[string]string{"album/1.flac": sampleFLAC, "album/2.flac": sampleFLAC, "album/3.flac": sampleFLAC}
	var writes atomic.Int32
	write := func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		if writes.Add(1) == 2 {
			return errors.New("disk full")
		}
		return audiometa.WriteContext(ctx, path, edit)
	}
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, Write: write}, samples)
	stale := fx.album.Etag
	p := decodeEditProblem(t, fx.post(t, stale, yearBody), http.StatusInternalServerError)
	if len(p.Changed) != 1 || len(p.Remaining) != 2 {
		t.Fatalf("changed %q, remaining %q: want 1 and 2", p.Changed, p.Remaining)
	}
	fx.post(t, stale, yearBody).problem(t, http.StatusPreconditionFailed) // One file has moved on.
	fx.reload(t)
	var got api.AlbumDetail
	fx.post(t, fx.album.Etag, yearBody).decode(t, &got)
	for rel := range samples {
		if year := tagsOf(t, filepath.Join(fx.root, rel)).Year; year != 1987 {
			t.Errorf("%s: year %d after the retry", rel, year)
		}
	}
	if got.Year != 1987 {
		t.Errorf("album year = %d", got.Year)
	}
}

func TestClientDisconnectDoesNotStopAnEdit(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, Write: holdingWrite(entered, release)}, twoFLACs())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fx.URL+fx.editsPath(), strings.NewReader(yearBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = fx.postCall("", fx.album.Etag, "").header
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		if resp, doErr := http.DefaultClient.Do(req); doErr == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	cancel()
	<-gone
	close(release)
	if _, err = fx.Scanner.Scan(t.Context(), library.Quick); err != nil { // It waits for the edit.
		t.Fatal(err)
	}
	for _, rel := range []string{fileOne, fileTwo} {
		if got := tagsOf(t, filepath.Join(fx.root, rel)).Year; got != 1987 {
			t.Errorf("%s: year %d, want the edit finished", rel, got)
		}
	}
}

// retag sets raw tags in every file of the fixture, outside the server, and
// scans them in.
func (fx *editFixture) retag(t *testing.T, tags map[string][]string) {
	t.Helper()
	for _, rel := range []string{fileOne, fileTwo} {
		err := audiometa.Write(filepath.Join(fx.root, rel), func(f *audiometa.File) error {
			for key, values := range tags {
				f.Tags.Set(key, values...)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	fx.reload(t)
}

// albumTags reads what an editor shows of the album.
func (fx *editFixture) albumTags(t *testing.T) api.AlbumTags {
	t.Helper()
	var got api.AlbumTags
	fx.getJSON(t, fx.token, fx.albumPath()+"/tags", &got)
	return got
}

// wantRawTag checks tr's other tags hold key with values.
func wantRawTag(t *testing.T, tr *api.TrackTags, key string, values ...string) {
	t.Helper()
	i := slices.IndexFunc(tr.Tags, func(tag api.RawTag) bool { return tag.Key == key })
	if i < 0 || !slices.Equal(tr.Tags[i].Values, values) {
		t.Errorf("tags = %+v, want %s = %q", tr.Tags, key, values)
	}
}

func TestAlbumTagsShowTheFiles(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	genres := []string{"Jazz", "Blues"}
	fx.retag(t, map[string][]string{keyGenre: genres, "MOOD": {"Calm"}})
	got := fx.albumTags(t)
	if got.Etag != fx.album.Etag || len(got.Tracks) != 2 {
		t.Fatalf("etag %s, %d tracks: want %s and 2", got.Etag, len(got.Tracks), fx.album.Etag)
	}
	tr := &got.Tracks[0]
	if tr.ID != fx.album.Tracks[0].ID || filepath.Ext(tr.File) != ".flac" || strings.Contains(tr.File, "/") {
		t.Errorf("track %s from %q, want %s and its base name", tr.ID, tr.File, fx.album.Tracks[0].ID)
	}
	if v := tr.Fields["genres"]; !slices.Equal(v, genres) {
		t.Errorf("genres = %q", v)
	}
	wantRawTag(t, tr, "MOOD", "Calm")
	if slices.ContainsFunc(tr.Tags, func(tag api.RawTag) bool { return tag.Key == keyGenre || tag.Key == "TITLE" }) {
		t.Errorf("tags = %+v: a field's keys don't belong here", tr.Tags)
	}
}

func TestAlbumTagsCallOutReadOnlyFields(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	fx.retag(t, map[string][]string{"LYRICS": {"[00:12.34]One"}})
	if got := fx.albumTags(t).Tracks[0].ReadOnly; !slices.Equal(got, []string{"lyrics"}) {
		t.Errorf("read-only = %q", got)
	}
}

func TestAlbumTagsRefusals(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	path := fx.albumPath() + "/tags"
	get := func(token, path string) response {
		return fx.send(t, call{method: http.MethodGet, path: path, header: bearer(token)})
	}
	get("not a token", path).problem(t, http.StatusUnauthorized)
	get(fx.tokenFor(t, store.RoleUser), path).problem(t, http.StatusForbidden)
	get(fx.token, albumsPath+"/"+missingID+"/tags").problem(t, http.StatusNotFound)
	if err := os.Remove(filepath.Join(fx.root, fileTwo)); err != nil {
		t.Fatal(err)
	}
	get(fx.token, path).problem(t, http.StatusConflict)
}

func TestGetTrack(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	want := fx.album.Tracks[0]
	var got api.TrackDetail
	path := "/tracks/" + want.ID
	fx.getJSON(t, fx.token, path, &got)
	if got.ID != want.ID || got.Etag != want.Etag || got.AlbumID != fx.album.ID || got.AlbumEtag != fx.album.Etag {
		t.Errorf("GET = %+v, want track %+v of album %s", got, want, fx.album.ID)
	}
	fx.send(t, call{method: http.MethodGet, path: path, header: bearer("not a token")}).problem(t, http.StatusUnauthorized)
	fx.send(t, call{method: http.MethodGet, path: "/tracks/" + missingID, header: bearer(fx.token)}).problem(t, http.StatusNotFound)
}
