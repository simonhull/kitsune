package server

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	editsPath = "/edits"
	oneFile   = "one/a.flac"
	twoFile   = "two/b.flac"
)

// crossFixture is a server over a library of two albums, one track each.
type crossFixture struct {
	*editFixture

	albums [2]api.AlbumDetail // By file: one, then two.
}

func newCrossFixture(t *testing.T, opts *testServerOptions) *crossFixture {
	t.Helper()
	fx := &crossFixture{editFixture: newEditServer(t, opts, map[string]string{oneFile: sampleFLAC, twoFile: sampleFLAC})}
	fx.reloadAll(t)
	return fx
}

// reloadAll reads both albums again.
func (fx *crossFixture) reloadAll(t *testing.T) {
	t.Helper()
	var page api.AlbumPage
	fx.getJSON(t, fx.token, albumsPath, &page)
	if len(page.Albums) != 2 {
		t.Fatalf("albums = %+v, want 2", page.Albums)
	}
	for i := range page.Albums {
		var d api.AlbumDetail
		fx.getJSON(t, fx.token, albumsPath+"/"+page.Albums[i].ID, &d)
		slot := 0
		if fx.fileOf(t, d.Tracks[0].ID) == twoFile {
			slot = 1
		}
		fx.albums[slot] = d
	}
}

// fileOf returns where the track id's file lies under the root.
func (fx *crossFixture) fileOf(t *testing.T, id string) string {
	t.Helper()
	f, err := fx.Store.TrackFile(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return f.RelPath
}

// body is an edit of both tracks, with all as its "all" member.
func (fx *crossFixture) body(all string) string {
	return `{"albums":[` + fx.albumRef(0, fx.albums[0].Etag) + `,` + fx.albumRef(1, fx.albums[1].Etag) + `],` +
		`"all":` + all + `,"tracks":[{"id":"` + fx.albums[0].Tracks[0].ID + `"},{"id":"` + fx.albums[1].Tracks[0].ID + `"}]}`
}

func (fx *crossFixture) albumRef(i int, etag string) string {
	return `{"id":"` + fx.albums[i].ID + `","etag":` + jsonString(etag) + `}`
}

// jsonString quotes s for a JSON body.
func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func (fx *crossFixture) postEdits(t *testing.T, body string) response {
	t.Helper()
	return fx.send(t, fx.postCall(editsPath, "", body))
}

func TestEditsAcrossAlbumsWriteEveryTrack(t *testing.T) {
	t.Parallel()
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true})
	var got api.EditedTracks
	fx.postEdits(t, fx.body(`{"set":{"date":["1987"]}}`)).decode(t, &got)
	if len(got.Tracks) != 2 || got.Tracks[0].AlbumID != fx.albums[0].ID || got.Tracks[1].AlbumID != fx.albums[1].ID {
		t.Errorf("tracks = %+v, want each on its own album", got.Tracks)
	}
	for _, rel := range []string{oneFile, twoFile} {
		if year := tagsOf(t, filepath.Join(fx.root, rel)).Year; year != 1987 {
			t.Errorf("%s: year %d", rel, year)
		}
	}
}

func TestEditsWithAStaleAlbumWriteNothing(t *testing.T) {
	t.Parallel()
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true})
	before := readFile(t, filepath.Join(fx.root, oneFile))
	body := `{"albums":[` + fx.albumRef(0, fx.albums[0].Etag) + `,` + fx.albumRef(1, staleETag) + `],` +
		`"all":{"set":{"date":["1987"]}},"tracks":[{"id":"` + fx.albums[0].Tracks[0].ID + `"},{"id":"` + fx.albums[1].Tracks[0].ID + `"}]}`
	p := decodeEditProblem(t, fx.postEdits(t, body), http.StatusPreconditionFailed)
	if p.Album != fx.albums[1].ID || !strings.Contains(p.Detail, fx.albums[1].ID) {
		t.Errorf("problem = %+v, want it to name %s", p, fx.albums[1].ID)
	}
	if !bytes.Equal(before, readFile(t, filepath.Join(fx.root, oneFile))) {
		t.Error("the fresh album's file changed")
	}
}

func TestEditsMovingTracksIntoOneAlbumKeepTheirIDs(t *testing.T) {
	t.Parallel()
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true})
	var got api.EditedTracks
	fx.postEdits(t, fx.body(`{"set":{"album":["Together"],"albumArtists":["Both"]}}`)).decode(t, &got)
	for i, tr := range got.Tracks {
		if tr.ID != fx.albums[i].Tracks[0].ID {
			t.Errorf("track %d = %s, want %s kept", i, tr.ID, fx.albums[i].Tracks[0].ID)
		}
		var d api.AlbumDetail
		fx.getJSON(t, fx.token, albumsPath+"/"+tr.AlbumID, &d)
		if d.Title != "Together" {
			t.Errorf("track %s is on %q", tr.ID, d.Title)
		}
	}
}

func TestEditsMergeAnAlbumIntoAnother(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	original := fx.album
	var split api.AlbumDetail
	fx.post(t, original.Etag, fx.trackBody(1, `{"album":["Split Off"]}`)).decode(t, &split)
	var moved api.TrackDetail
	fx.getJSON(t, fx.token, "/tracks/"+original.Tracks[1].ID, &moved)
	body := `{"albums":[{"id":"` + split.ID + `","etag":` + jsonString(split.Etag) + `},{"id":"` + moved.AlbumID + `","etag":` + jsonString(moved.AlbumEtag) + `}],` +
		`"all":{"set":{"album":[` + jsonString(original.Title) + `]}},"tracks":[{"id":"` + original.Tracks[1].ID + `"}]}`
	var got api.EditedTracks
	fx.send(t, fx.postCall(editsPath, "", body)).decode(t, &got)
	if got.Tracks[0].AlbumID != original.ID {
		t.Errorf("the track landed on %s, want %s, merging the split-off album back", got.Tracks[0].AlbumID, original.ID)
	}
	fx.send(t, call{method: http.MethodGet, path: albumsPath + "/" + moved.AlbumID, header: bearer(fx.token)}).problem(t, http.StatusNotFound)
}

func TestEditsStoppedPartwayReportHowFar(t *testing.T) {
	t.Parallel()
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true, Write: failingWrite(1)})
	p := decodeEditProblem(t, fx.postEdits(t, fx.body(`{"set":{"date":["1987"]}}`)), http.StatusInternalServerError)
	if len(p.Changed) != 1 || len(p.Remaining) != 1 || p.Changed[0] != fx.albums[0].Tracks[0].ID {
		t.Errorf("changed %q, remaining %q: want the first album's track changed", p.Changed, p.Remaining)
	}
}

func TestEditsRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    testServerOptions
		role    store.Role
		body    func(*crossFixture) string
		want    int
		invalid bool
	}{
		{name: "writes off", role: store.RoleAdmin, body: yearAcross, want: http.StatusForbidden},
		{name: "not an admin", opts: testServerOptions{TagWrites: true}, role: store.RoleUser, body: yearAcross, want: http.StatusForbidden},
		{name: "an unknown album", opts: testServerOptions{TagWrites: true}, role: store.RoleAdmin, want: http.StatusNotFound, body: func(fx *crossFixture) string {
			return `{"albums":[{"id":"` + missingID + `","etag":"x"}],"all":{"set":{"date":["1987"]}},"tracks":[{"id":"` + fx.albums[0].Tracks[0].ID + `"}]}`
		}},
		{name: "a track of an album not named", opts: testServerOptions{TagWrites: true}, role: store.RoleAdmin, want: http.StatusUnprocessableEntity, body: func(fx *crossFixture) string {
			return `{"albums":[` + fx.albumRef(0, fx.albums[0].Etag) + `],"all":{"set":{"date":["1987"]}},"tracks":[{"id":"` + fx.albums[1].Tracks[0].ID + `"}]}`
		}},
		{name: "an album named twice", opts: testServerOptions{TagWrites: true}, role: store.RoleAdmin, want: http.StatusBadRequest, body: func(fx *crossFixture) string {
			return `{"albums":[` + fx.albumRef(0, fx.albums[0].Etag) + `,` + fx.albumRef(0, fx.albums[0].Etag) + `],"all":{"set":{"date":["1987"]}},"tracks":[{"id":"` + fx.albums[0].Tracks[0].ID + `"}]}`
		}},
		{name: "nothing to change", opts: testServerOptions{TagWrites: true}, role: store.RoleAdmin, want: http.StatusBadRequest, body: func(fx *crossFixture) string {
			return fx.body(`{}`)
		}},
		{name: "no albums", opts: testServerOptions{TagWrites: true}, role: store.RoleAdmin, want: http.StatusBadRequest, invalid: true, body: func(*crossFixture) string {
			return `{"tracks":[{"id":"x"}]}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newCrossFixture(t, &tt.opts)
			if tt.role != store.RoleAdmin {
				fx.token = fx.tokenFor(t, tt.role)
			}
			c := fx.postCall(editsPath, "", tt.body(fx))
			c.invalid = tt.invalid
			fx.send(t, c).problem(t, tt.want)
		})
	}
}

func yearAcross(fx *crossFixture) string { return fx.body(`{"set":{"date":["1987"]}}`) }

func TestEditsNeedCredentials(t *testing.T) {
	t.Parallel()
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true})
	fx.token = "not a token"
	fx.postEdits(t, yearAcross(fx)).problem(t, http.StatusUnauthorized)
}

func TestEditsDuringAnEditAreBusy(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	fx := newCrossFixture(t, &testServerOptions{TagWrites: true, Write: holdingWrite(entered, release)})
	first := make(chan error, 1)
	c := fx.postCall(editsPath, "", yearAcross(fx))
	go func() {
		_, err := fx.trySend(t, c)
		first <- err
	}()
	<-entered
	fx.postEdits(t, yearAcross(fx)).problem(t, http.StatusConflict)
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}
