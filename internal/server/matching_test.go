package server

import (
	"encoding/json/v2"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	matchingQueuePath = "/matching/queue"
	testAlbum         = "Test Album"
	testArtist        = "Test Artist"
)

// songTitles are the titles of a test release's songs, in order.
var songTitles = []string{"Breakwater", "Fogbound", "Neap"} //nolint:gochecknoglobals // A fixed fixture, never written.

// testRelease is the stand-in's release of the sample album: two songs of two
// seconds, as the sample FLACs are.
func testRelease(name string, songs int) matching.Release {
	artist := []matching.Credit{{Name: testArtist, ID: musicbrainztest.ID("artist")}}
	m := matching.Medium{Position: 1, Format: "CD", TrackCount: songs}
	for i := range songs {
		m.Tracks = append(m.Tracks, matching.ReleaseTrack{
			ID: musicbrainztest.ID(name + " track " + string(rune('a'+i))), RecordingID: musicbrainztest.ID("recording " + string(rune('a'+i))),
			Position: i + 1, Title: songTitles[i%len(songTitles)], Artists: artist, Length: 2 * time.Second,
		})
	}
	return matching.Release{
		Source: matching.SourceMusicBrainz, ID: musicbrainztest.ID(name), GroupID: musicbrainztest.ID("group"),
		Title: testAlbum, Artists: artist, Date: "2019-04-05", Labels: []matching.Label{{Name: "Label", CatalogNumber: "CAT 1"}},
		Media: []matching.Medium{m}, Cover: true,
	}
}

// matchFixture is an album of two sample FLACs, with tag writes on, and a
// stand-in MusicBrainz holding its release and a three-song one.
type matchFixture struct {
	*editFixture

	mb      *musicbrainztest.Server
	release matching.Release
}

func newMatchFixture(t *testing.T) *matchFixture {
	t.Helper()
	release := testRelease("two", 2)
	mb := musicbrainztest.NewInMemory(release, testRelease("three", 3))
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, MusicBrainz: mb}, twoFLACs())
	return &matchFixture{editFixture: fx, mb: mb, release: release}
}

// send sends an authenticated request with a JSON body, if any.
func (fx *matchFixture) call(method, path, body string) call {
	h := bearer(fx.token)
	if body != "" {
		h.Set(contentType, api.JSONMediaType)
	}
	return call{method: method, path: path, body: body, header: h}
}

// find asks for the album's matches with body.
func (fx *matchFixture) find(t *testing.T, body string) api.MatchList {
	t.Helper()
	resp := fx.send(t, fx.call(http.MethodPost, fx.albumPath()+"/matches", body))
	if resp.Status != http.StatusOK {
		t.Fatalf("POST matches = %d %s", resp.Status, resp.Body)
	}
	var list api.MatchList
	resp.decode(t, &list)
	return list
}

func TestMatchesFindARelease(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	list := fx.find(t, `{}`)
	if list.Source != matching.SourceMusicBrainz || list.Asked.Album != testAlbum || list.Asked.Artist != testArtist {
		t.Errorf("asked %+v of %s", list.Asked, list.Source)
	}
	if len(list.Candidates) != 2 || list.Candidates[0].Release.ID != fx.release.ID {
		t.Fatalf("candidates = %+v, want the two-song release first", list.Candidates)
	}
	best := list.Candidates[0]
	if best.Score < 90 || best.Release.TrackCount != 2 || best.Cover == "" || best.Release.Labels[0].CatalogNumber != "CAT 1" {
		t.Errorf("best = %+v", best)
	}
}

func TestReviewingAMatch(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	best := fx.find(t, `{}`).Candidates[0]
	var review api.MatchReview
	fx.getJSON(t, fx.token, "/matches/"+best.ID, &review)
	if review.Candidate.ID != best.ID || len(review.Tracks) != 2 || len(review.Pairs) != 2 {
		t.Fatalf("review = %+v", review)
	}
	for i, p := range review.Pairs {
		if p.Track != fx.album.Tracks[i].ID || p.Release == nil || *p.Release != i {
			t.Errorf("pair %d = %+v", i, p)
		}
	}
	if got := review.AlbumIDs[string(library.FieldMusicBrainzAlbumID)]; !slices.Equal(got, []string{fx.release.ID}) {
		t.Errorf("album IDs = %q", review.AlbumIDs)
	}
	if got := review.Tracks[1].Proposed[string(library.FieldTitle)]; !slices.Equal(got, songTitles[1:2]) {
		t.Errorf("second track proposes %q", got)
	}
}

func TestMatchesAreKeptForADay(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	best := fx.find(t, `{}`).Candidates[0]
	before := len(fx.mb.Calls())
	var review api.MatchReview
	fx.getJSON(t, fx.token, "/matches/"+best.ID, &review)
	fx.find(t, `{}`)
	if after := len(fx.mb.Calls()); after != before {
		t.Errorf("asking again asked MusicBrainz %d more times", after-before)
	}
}

func TestApplyingAMatchWritesItsIDs(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	var review api.MatchReview
	fx.getJSON(t, fx.token, "/matches/"+fx.find(t, `{}`).Candidates[0].ID, &review)
	// What a client sends: the accepted values and the IDs, for the first track only.
	set := maps.Clone(review.AlbumIDs)
	maps.Copy(set, review.Tracks[0].IDs)
	set[string(library.FieldTitle)] = review.Tracks[0].Proposed[string(library.FieldTitle)]
	body, err := json.Marshal(api.AlbumEdit{Tracks: []api.TrackTagEdit{{ID: fx.album.Tracks[0].ID, Set: set}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp := fx.post(t, fx.album.Etag, string(body)); resp.Status != http.StatusOK {
		t.Fatalf("apply = %d %s", resp.Status, resp.Body)
	}
	// The two files are alike, so which is the album's first is the library's to say: one takes it.
	var took []string
	for _, file := range []string{fileOne, fileTwo} {
		tags := tagsOf(t, filepath.Join(fx.root, file))
		if tags.Get("MUSICBRAINZ_ALBUMID") == nil {
			continue
		}
		took = append(took, file)
		if got := tags.Get("MUSICBRAINZ_ALBUMID"); !slices.Equal(got, []string{fx.release.ID}) {
			t.Errorf("%s album MBID = %q", file, got)
		}
		if got := tags.Get("MUSICBRAINZ_RELEASETRACKID"); !slices.Equal(got, []string{fx.release.Media[0].Tracks[0].ID}) {
			t.Errorf("%s track MBID = %q", file, got)
		}
	}
	if len(took) != 1 {
		t.Errorf("files with the release's ID: %q, want just the song applied to", took)
	}
	// Now the files name the release, a search counts it.
	fx.reload(t)
	list := fx.find(t, `{}`)
	if !slices.ContainsFunc(list.Candidates[0].Evidence, func(e api.MatchEvidence) bool { return e.Kind == "tagged" && e.Count == 1 }) {
		t.Errorf("evidence = %+v, want one song tagged", list.Candidates[0].Evidence)
	}
}

func TestMatchesForOtherTermsAndAhead(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	list := fx.find(t, `{"album":"Nothing Like It","code":"5060000000417","prefetch":true}`)
	if len(list.Candidates) != 0 || list.Asked.Album != "Nothing Like It" || list.Asked.Barcode != "5060000000417" {
		t.Errorf("list = %+v, want nothing found for the terms asked", list)
	}
	if calls := fx.mb.Calls(); !strings.Contains(calls[0].Query, `barcode:"5060000000417"`) {
		t.Errorf("asked %q", calls[0].Query)
	}
}

func TestMatchesRefuse(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	parts := strings.Split(fx.find(t, `{}`).Candidates[0].ID, ".")
	album, source, release := parts[0], parts[1], parts[2]
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"a body that isn't a request", http.MethodPost, fx.albumPath() + "/matches", `{"artist":1}`, http.StatusBadRequest},
		{"an unknown album", http.MethodPost, albumsPath + "/01JNOTANALBUM0000000000000/matches", `{}`, http.StatusNotFound},
		{"a candidate that isn't one", http.MethodGet, "/matches/nonsense", "", http.StatusNotFound},
		{"a candidate of another source", http.MethodGet, "/matches/" + album + ".discogs." + release, "", http.StatusNotFound},
		{"a release the source lacks", http.MethodGet, "/matches/" + album + "." + source + "." + musicbrainztest.ID("gone"), "", http.StatusNotFound},
		{"a candidate of an unknown album", http.MethodGet, "/matches/01JNOTANALBUM0000000000000." + source + "." + release, "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := fx.call(tt.method, tt.path, tt.body)
			c.invalid = tt.status == http.StatusBadRequest
			fx.send(t, c).problem(t, tt.status)
		})
	}
}

func TestMatchesWhenMusicBrainzIsDown(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	fx.mb.Fail("", http.StatusInternalServerError)
	resp := fx.send(t, fx.call(http.MethodPost, fx.albumPath()+"/matches", `{}`))
	resp.problem(t, http.StatusServiceUnavailable)
	if resp.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	if !strings.Contains(fx.Log.String(), "find matches") {
		t.Errorf("log = %s, want the failure noted", fx.Log)
	}
}

func TestMatchesWithoutAMatcher(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	for _, c := range []call{
		{method: http.MethodPost, path: fx.albumPath() + "/matches", body: `{}`},
		{method: http.MethodGet, path: "/matches/" + fx.album.ID + ".musicbrainz." + musicbrainztest.ID("x")},
	} {
		c.header = bearer(fx.token)
		c.header.Set(contentType, api.JSONMediaType)
		fx.send(t, c).problem(t, http.StatusServiceUnavailable)
	}
}

func TestMatchingQueue(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	queue := func() api.MatchQueue {
		t.Helper()
		var q api.MatchQueue
		fx.getJSON(t, fx.token, matchingQueuePath, &q)
		return q
	}
	want := api.QueuedAlbum{ID: fx.album.ID, Title: testAlbum, Artist: testArtist, Songs: 2, State: string(store.MatchToDo)}
	if q := queue(); q.Counts != (api.MatchCounts{ToDo: 1}) || !slices.Equal(q.Albums, []api.QueuedAlbum{want}) {
		t.Fatalf("queue = %+v", q)
	}
	path := "/matching/albums/" + fx.album.ID
	if resp := fx.send(t, fx.call(http.MethodPut, path, `{"state":"skipped"}`)); resp.Status != http.StatusNoContent {
		t.Fatalf("PUT = %d %s", resp.Status, resp.Body)
	}
	if q := queue(); q.Counts != (api.MatchCounts{Skipped: 1}) || q.Albums[0].State != string(store.MatchSkipped) {
		t.Errorf("after a skip, queue = %+v", q)
	}
	bad := fx.call(http.MethodPut, path, `{"state":"done"}`)
	bad.invalid = true
	fx.send(t, bad).problem(t, http.StatusBadRequest)
	fx.send(t, fx.call(http.MethodPut, "/matching/albums/01JNOTANALBUM0000000000000", `{"state":"matched"}`)).problem(t, http.StatusNotFound)
}

func TestMatchingIsForAdmins(t *testing.T) {
	t.Parallel()
	fx := newMatchFixture(t)
	user := fx.tokenFor(t, store.RoleUser)
	routes := []call{
		{method: http.MethodPost, path: fx.albumPath() + "/matches", body: `{}`},
		{method: http.MethodGet, path: "/matches/" + fx.album.ID + ".musicbrainz." + fx.release.ID},
		{method: http.MethodGet, path: matchingQueuePath},
		{method: http.MethodPut, path: "/matching/albums/" + fx.album.ID, body: `{"state":"skipped"}`},
	}
	for _, c := range routes {
		for token, status := range map[string]int{"": http.StatusUnauthorized, user: http.StatusForbidden} {
			c.header, c.invalid = http.Header{}, token == "" // The contract asks for credentials.
			if token != "" {
				c.header = bearer(token)
			}
			if c.body != "" {
				c.header.Set(contentType, api.JSONMediaType)
			}
			fx.send(t, c).problem(t, status)
		}
	}
	if n := len(fx.mb.Calls()); n != 0 {
		t.Errorf("refused requests asked MusicBrainz %d times", n)
	}
}
