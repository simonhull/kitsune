package server

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	ratingsPath       = "/me/ratings/tracks/"
	heartsSummaryPath = "/me/hearts/summary"
	heartsConvertPath = "/me/hearts/convert"
	secondTrack       = "01J00000000000000000001002"
	kindTracks        = "tracks"
)

// markFixture is a server with one two-track album by Ann and a signed-in admin.
func markFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2})
	sess := ts.sessionFor(t)
	return ts, sess
}

// heart gives userID a heart of kind on id, as one kept from before stars were
// the only like: no route makes one any more.
func (ts *testServer) heart(t *testing.T, userID int64, kind store.Kind, id string) {
	t.Helper()
	if err := ts.Store.Heart(t.Context(), userID, kind, id); err != nil {
		t.Fatal(err)
	}
}

// heartEverything hearts both tracks, album 1 and Ann for userID.
func (ts *testServer) heartEverything(t *testing.T, userID int64) {
	t.Helper()
	ts.heart(t, userID, store.KindTrack, firstTrack)
	ts.heart(t, userID, store.KindTrack, secondTrack)
	ts.heart(t, userID, store.KindAlbum, albumULID(1))
	ts.heart(t, userID, store.KindArtist, annID)
}

// rateCall rates a track as the holder of token.
func rateCall(token, id, body string) call {
	return call{method: http.MethodPut, path: ratingsPath + id, body: body, header: jsonHeader(token)}
}

// convertCall turns the hearts of the holder of token into stars.
func convertCall(token, body string) call {
	return call{method: http.MethodPost, path: heartsConvertPath, body: body, header: jsonHeader(token)}
}

// stars is a rating body of n stars.
func stars(n float64) string { return fmt.Sprintf(`{"stars":%v}`, n) }

// albumAs reads album 1 as the holder of token.
func (ts *testServer) albumAs(t *testing.T, token string) api.AlbumDetail {
	t.Helper()
	var out api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &out)
	return out
}

// indexAs reads the library index as the holder of token.
func (ts *testServer) indexAs(t *testing.T, token string) api.LibraryIndex {
	t.Helper()
	var out api.LibraryIndex
	ts.getJSON(t, token, indexPath, &out)
	return out
}

// trackStars returns each track's stars on album 1, by ULID, as the album page
// and the index both say them. It fails if the two disagree.
func (ts *testServer) trackStars(t *testing.T, token string) map[string]float64 {
	t.Helper()
	page := map[string]float64{}
	album := ts.albumAs(t, token)
	for i := range album.Tracks {
		page[album.Tracks[i].ID] = album.Tracks[i].Stars
	}
	index := map[string]float64{}
	for _, a := range ts.indexAs(t, token).Artists {
		for i := range a.Albums {
			for _, tr := range a.Albums[i].Tracks {
				index[tr.ID] = tr.Stars
			}
		}
	}
	if !maps.Equal(page, index) {
		t.Errorf("the album page says %v and the index %v", page, index)
	}
	return page
}

// heartsSummary reads the hearts the holder of token kept.
func (ts *testServer) heartsSummary(t *testing.T, token string) api.HeartsSummary {
	t.Helper()
	var out api.HeartsSummary
	ts.getJSON(t, token, heartsSummaryPath, &out)
	return out
}

// convert turns the hearts of the holder of token into stars, and returns
// what it wrote.
func (ts *testServer) convert(t *testing.T, token, body string) api.HeartsConverted {
	t.Helper()
	resp := ts.send(t, convertCall(token, body))
	if resp.Status != http.StatusOK {
		t.Fatalf("convert = %d %s", resp.Status, resp.Body)
	}
	var out api.HeartsConverted
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRatingsReplaceAndClear(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, rateCall(sess.token, firstTrack, stars(2))))
	wantNoContent(t, ts.send(t, rateCall(sess.token, firstTrack, stars(4.5))))
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(5))))
	if got := ts.trackStars(t, sess.token); got[firstTrack] != 4.5 || got[secondTrack] != 5 {
		t.Errorf("stars = %v, want 4½ and 5", got)
	}
	for range 2 {
		wantNoContent(t, ts.send(t, call{method: http.MethodDelete, path: ratingsPath + firstTrack, header: bearer(sess.token)}))
	}
	if got := ts.trackStars(t, sess.token); got[firstTrack] != 0 || got[secondTrack] != 5 {
		t.Errorf("stars = %v, want the first unrated and the second at 5", got)
	}
}

func TestRatingsOfUnknownTracks(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	ts.send(t, rateCall(sess.token, missingID, stars(3))).problem(t, http.StatusNotFound)
	ts.send(t, call{method: http.MethodDelete, path: ratingsPath + missingID, header: bearer(sess.token)}).problem(t, http.StatusNotFound)
	// An album's ULID is no track's.
	ts.send(t, rateCall(sess.token, albumULID(1), stars(3))).problem(t, http.StatusNotFound)
}

func TestMarksRefuseBadRequests(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	for name, body := range map[string]string{
		"zero":      stars(0),
		"over five": stars(5.5),
		"quarter":   stars(4.25),
		"negative":  stars(-0.5),
		"none":      `{}`,
		"text":      `{"stars":"4"}`,
		"extra":     `{"stars":3,"album":true}`,
		"garbage":   "{",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := rateCall(sess.token, firstTrack, body)
			c.invalid = true
			ts.send(t, c).problem(t, http.StatusBadRequest)
			c = convertCall(sess.token, body)
			c.invalid = true
			ts.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
	if got := ts.trackStars(t, sess.token); got[firstTrack] != 0 {
		t.Errorf("stars = %v, want none", got)
	}
}

func TestRatingsAreTheCallersOwn(t *testing.T) {
	t.Parallel()
	ts, admin := markFixture(t)
	other := ts.tokenFor(t, store.RoleUser)
	wantNoContent(t, ts.send(t, rateCall(admin.token, firstTrack, stars(5))))
	wantNoContent(t, ts.send(t, rateCall(other, secondTrack, stars(0.5))))

	if got, want := ts.trackStars(t, other), map[string]float64{firstTrack: 0, secondTrack: 0.5}; !maps.Equal(got, want) {
		t.Errorf("the user's stars = %v, want %v: only their own rating", got, want)
	}
	if got, want := ts.trackStars(t, admin.token), map[string]float64{firstTrack: 5, secondTrack: 0}; !maps.Equal(got, want) {
		t.Errorf("the admin's stars = %v, want %v", got, want)
	}
}

func TestTrackDetailCarriesStars(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(3.5))))
	var out api.TrackDetail
	ts.getJSON(t, sess.token, "/tracks/"+secondTrack, &out)
	if out.Stars != 3.5 {
		t.Errorf("track = %v stars, want 3½", out.Stars)
	}
}

func TestDeletedTrackTakesItsMarks(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, rateCall(sess.token, firstTrack, stars(5))))
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(2))))

	// What a scan does with a file that's gone: mark it missing, and delete it once
	// it has been missing long enough.
	files, err := ts.Store.FileStates(t.Context(), "/music")
	if err != nil {
		t.Fatal(err)
	}
	gone, ok := files["a1/1.flac"]
	if !ok {
		t.Fatalf("files = %v, want a1/1.flac", files)
	}
	if err = ts.Store.MarkMissing(t.Context(), []int64{gone.TrackID}, ts.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = ts.Store.DeleteMissingBefore(t.Context(), ts.Clock.Now().Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	ts.send(t, rateCall(sess.token, firstTrack, stars(5))).problem(t, http.StatusNotFound)

	// The same file back, under the same ULID, is a new row with no marks.
	back := (&browseAlbum{n: 1, sortName: "a", tracks: 2}).writes()[:1]
	if err = ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: back}); err != nil {
		t.Fatal(err)
	}
	if got := ts.trackStars(t, sess.token); got[firstTrack] != 0 || got[secondTrack] != 2 {
		t.Errorf("stars = %v, want the deleted track's gone and the other's kept", got)
	}
}

func TestMarksNeedASession(t *testing.T) {
	t.Parallel()
	ts, _ := markFixture(t)
	for _, c := range []call{
		rateCall("", firstTrack, stars(3)),
		{method: http.MethodDelete, path: ratingsPath + firstTrack},
		{method: http.MethodGet, path: heartsSummaryPath},
		convertCall("", stars(4)),
	} {
		if c.header == nil {
			c.header = http.Header{}
		}
		c.header.Del(authorizationHeader)
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

func TestMarksByCookieNeedTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	calls := []struct {
		c    call
		want int
	}{
		{call{method: http.MethodPut, path: ratingsPath + firstTrack, body: stars(3)}, http.StatusNoContent},
		{call{method: http.MethodDelete, path: ratingsPath + firstTrack}, http.StatusNoContent},
		{call{method: http.MethodPost, path: heartsConvertPath, body: `{"stars":null}`}, http.StatusOK},
	}
	for _, tc := range calls {
		c := tc.c
		c.header = cookieOnly(sess.token)
		if c.body != "" {
			c.header.Set(contentType, api.JSONMediaType)
		}
		ts.send(t, c).problem(t, http.StatusForbidden)
		c.header.Set(requestHeader, "1")
		if got := ts.send(t, c); got.Status != tc.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, got.Status, tc.want)
		}
	}
}

func TestHeartsSummaryCountsWhatWasKept(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	if got := ts.heartsSummary(t, sess.token); got.TracksRated+got.TracksUnrated+got.Albums+got.Artists != 0 || len(got.Preview) != 0 {
		t.Errorf("summary with no hearts = %+v, want all zeros", got)
	}
	ts.heartEverything(t, sess.user.ID)
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(4.5))))

	got := ts.heartsSummary(t, sess.token)
	want := api.HeartsSummary{
		TracksRated: 1, TracksUnrated: 1, Albums: 1, Artists: 1, ByStars: []int{0, 0, 0, 0, 1},
		Preview: []api.HeartedSong{{ID: firstTrack, Title: ts.albumAs(t, sess.token).Tracks[0].Title, Artist: annName}},
	}
	if got.TracksRated != want.TracksRated || got.TracksUnrated != want.TracksUnrated || got.Albums != want.Albums ||
		got.Artists != want.Artists || !slices.Equal(got.ByStars, want.ByStars) || !slices.Equal(got.Preview, want.Preview) {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
}

func TestConvertHeartsRatesTheUnratedOnceAndTakesEveryHeart(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	other := ts.seedUser(t, "kim", testPassword, store.RoleUser)
	ts.heartEverything(t, sess.user.ID)
	ts.heartEverything(t, other.ID)
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(2))))
	stream := ts.openStream(t, sess.token, "")
	stream.want(events.Ready)

	got := ts.convert(t, sess.token, stars(4))
	if want := (api.HeartsConverted{Rated: 1, Tracks: 2, Albums: 1, Artists: 1}); got != want {
		t.Errorf("converted = %+v, want %+v", got, want)
	}
	stream.wantMark(&api.EventMarks{Kind: kindTracks, ID: firstTrack, Stars: 4})
	if got, want := ts.trackStars(t, sess.token), map[string]float64{firstTrack: 4, secondTrack: 2}; !maps.Equal(got, want) {
		t.Errorf("stars = %v, want %v: only the unrated heart takes stars", got, want)
	}
	if got := ts.heartsSummary(t, sess.token); got.TracksRated+got.TracksUnrated+got.Albums+got.Artists != 0 {
		t.Errorf("summary after = %+v, want all zeros: the card is gone for good", got)
	}
	if again := ts.convert(t, sess.token, stars(1)); again != (api.HeartsConverted{}) {
		t.Errorf("a second conversion = %+v, want nothing done", again)
	}
	if theirs, err := ts.Store.HeartsSummary(t.Context(), other.ID); err != nil || theirs.Unrated != 2 || theirs.Albums != 1 || theirs.Artists != 1 {
		t.Errorf("the other user's hearts = %+v, %v, want them all still there", theirs, err)
	}
}

func TestConvertHeartsCanLeaveThemUnrated(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	ts.heartEverything(t, sess.user.ID)
	if got := ts.convert(t, sess.token, `{"stars":null}`); got != (api.HeartsConverted{Tracks: 2, Albums: 1, Artists: 1}) {
		t.Errorf("converted = %+v, want every heart gone and nothing rated", got)
	}
	if got := ts.trackStars(t, sess.token); got[firstTrack] != 0 || got[secondTrack] != 0 {
		t.Errorf("stars = %v, want none", got)
	}
}
