package server

import (
	"fmt"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	heartsPath  = "/me/hearts/"
	ratingsPath = "/me/ratings/tracks/"
	secondTrack = "01J00000000000000000001002"
	kindTracks  = "tracks"
	kindAlbums  = "albums"
	kindArtists = "artists"
)

// heartTarget is something markFixture has to heart, as the path names it.
type heartTarget struct{ kind, id string }

// everyKind is one of each kind of thing markFixture has.
func everyKind() []heartTarget {
	return []heartTarget{{kindTracks, firstTrack}, {kindAlbums, albumULID(1)}, {kindArtists, annID}}
}

// markFixture is a server with one two-track album by Ann and a signed-in admin.
func markFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2})
	sess := ts.sessionFor(t)
	return ts, sess
}

// heartCall is a heart change as the holder of token.
func heartCall(method, token, kind, id string) call {
	return call{method: method, path: heartsPath + kind + "/" + id, header: bearer(token)}
}

// rateCall rates a track as the holder of token.
func rateCall(token, id, body string) call {
	return call{method: http.MethodPut, path: ratingsPath + id, body: body, header: jsonHeader(token)}
}

func stars(n int) string { return fmt.Sprintf(`{"stars":%d}`, n) }

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

// trackMarks returns each track's marks on album 1, by ULID, as the album page
// and the index both say them. It fails if the two disagree.
func (ts *testServer) trackMarks(t *testing.T, token string) map[string]store.Marks {
	t.Helper()
	page := map[string]store.Marks{}
	album := ts.albumAs(t, token)
	for i := range album.Tracks {
		tr := &album.Tracks[i]
		page[tr.ID] = store.Marks{Hearted: tr.Hearted, Stars: tr.Stars}
	}
	index := map[string]store.Marks{}
	for _, a := range ts.indexAs(t, token).Artists {
		for _, al := range a.Albums {
			for _, tr := range al.Tracks {
				index[tr.ID] = store.Marks{Hearted: tr.Hearted, Stars: tr.Stars}
			}
		}
	}
	if !maps.Equal(page, index) {
		t.Errorf("the album page says %v and the index %v", page, index)
	}
	return page
}

// heartsSeen is what the holder of a token sees hearted, in the index and on
// the album page, which must agree on the album.
type heartsSeen struct{ artist, album, first, second bool }

func (ts *testServer) heartsSeen(t *testing.T, token string) heartsSeen {
	t.Helper()
	a := ts.indexAs(t, token).Artists[0]
	if page := ts.albumAs(t, token); page.Hearted != a.Albums[0].Hearted {
		t.Errorf("the album page says hearted %t, the index %t", page.Hearted, a.Albums[0].Hearted)
	}
	marks := ts.trackMarks(t, token)
	return heartsSeen{a.Hearted, a.Albums[0].Hearted, marks[firstTrack].Hearted, marks[secondTrack].Hearted}
}

func TestHeartsAreIdempotent(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		for _, target := range everyKind() {
			for range 2 {
				wantNoContent(t, ts.send(t, heartCall(method, sess.token, target.kind, target.id)))
			}
		}
		want := heartsSeen{artist: true, album: true, first: true}
		if method == http.MethodDelete {
			want = heartsSeen{}
		}
		if got := ts.heartsSeen(t, sess.token); got != want {
			t.Errorf("after %s twice: hearted %+v, want %+v", method, got, want)
		}
	}
}

func TestRatingsReplaceAndClear(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, rateCall(sess.token, firstTrack, stars(2))))
	wantNoContent(t, ts.send(t, rateCall(sess.token, firstTrack, stars(4))))
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(5))))
	if got := ts.trackMarks(t, sess.token); got[firstTrack].Stars != 4 || got[secondTrack].Stars != 5 {
		t.Errorf("marks = %v, want 4 and 5 stars", got)
	}
	for range 2 {
		wantNoContent(t, ts.send(t, call{method: http.MethodDelete, path: ratingsPath + firstTrack, header: bearer(sess.token)}))
	}
	if got := ts.trackMarks(t, sess.token); got[firstTrack].Stars != 0 || got[secondTrack].Stars != 5 {
		t.Errorf("marks = %v, want the first unrated and the second at 5", got)
	}
}

func TestMarksOnUnknownItems(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	for _, target := range everyKind() {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			ts.send(t, heartCall(method, sess.token, target.kind, missingID)).problem(t, http.StatusNotFound)
		}
	}
	ts.send(t, rateCall(sess.token, missingID, stars(3))).problem(t, http.StatusNotFound)
	ts.send(t, call{method: http.MethodDelete, path: ratingsPath + missingID, header: bearer(sess.token)}).problem(t, http.StatusNotFound)
	// An album's ULID is no track's.
	ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, albumULID(1))).problem(t, http.StatusNotFound)
}

func TestMarksRefuseBadRequests(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		c := heartCall(method, sess.token, "songs", firstTrack)
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusBadRequest)
	}
	for name, body := range map[string]string{
		"zero":     stars(0),
		"six":      stars(6),
		"none":     `{}`,
		"text":     `{"stars":"4"}`,
		"fraction": `{"stars":3.5}`,
		"extra":    `{"stars":3,"album":true}`,
		"garbage":  "{",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := rateCall(sess.token, firstTrack, body)
			c.invalid = true
			ts.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
	if got := ts.trackMarks(t, sess.token); got[firstTrack] != (store.Marks{}) {
		t.Errorf("marks = %v, want none", got)
	}
}

func TestMarksAreTheCallersOwn(t *testing.T) {
	t.Parallel()
	ts, admin := markFixture(t)
	other := ts.tokenFor(t, store.RoleUser)
	for _, target := range everyKind() {
		wantNoContent(t, ts.send(t, heartCall(http.MethodPut, admin.token, target.kind, target.id)))
	}
	wantNoContent(t, ts.send(t, rateCall(admin.token, firstTrack, stars(5))))
	wantNoContent(t, ts.send(t, rateCall(other, secondTrack, stars(1))))

	if got := ts.trackMarks(t, other); got[firstTrack] != (store.Marks{}) || got[secondTrack] != (store.Marks{Stars: 1}) {
		t.Errorf("the user's marks = %v, want only their own rating", got)
	}
	if got := ts.heartsSeen(t, other); got != (heartsSeen{}) {
		t.Errorf("the user sees the admin's hearts: %+v", got)
	}
	if got := ts.trackMarks(t, admin.token); got[firstTrack] != (store.Marks{Hearted: true, Stars: 5}) || got[secondTrack] != (store.Marks{}) {
		t.Errorf("the admin's marks = %v", got)
	}
}

func TestTrackDetailCarriesMarks(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, secondTrack)))
	wantNoContent(t, ts.send(t, rateCall(sess.token, secondTrack, stars(3))))
	var out api.TrackDetail
	ts.getJSON(t, sess.token, "/tracks/"+secondTrack, &out)
	if !out.Hearted || out.Stars != 3 {
		t.Errorf("track = hearted %t, %d stars, want hearted at 3", out.Hearted, out.Stars)
	}
}

func TestDeletedTrackTakesItsMarks(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	wantNoContent(t, ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, firstTrack)))
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
	ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, firstTrack)).problem(t, http.StatusNotFound)

	// The same file back, under the same ULID, is a new row with no marks.
	back := (&browseAlbum{n: 1, sortName: "a", tracks: 2}).writes()[:1]
	if err = ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: back}); err != nil {
		t.Fatal(err)
	}
	if got := ts.trackMarks(t, sess.token); got[firstTrack] != (store.Marks{}) || got[secondTrack] != (store.Marks{Stars: 2}) {
		t.Errorf("marks = %v, want the deleted track's gone and the other's kept", got)
	}
}

func TestMarksNeedASession(t *testing.T) {
	t.Parallel()
	ts, _ := markFixture(t)
	for _, c := range []call{
		heartCall(http.MethodPut, "", kindTracks, firstTrack),
		heartCall(http.MethodDelete, "", kindTracks, firstTrack),
		rateCall("", firstTrack, stars(3)),
		{method: http.MethodDelete, path: ratingsPath + firstTrack},
	} {
		c.header.Del(authorizationHeader)
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

func TestMarksByCookieNeedTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	calls := []call{
		{method: http.MethodPut, path: heartsPath + "tracks/" + firstTrack},
		{method: http.MethodDelete, path: heartsPath + "tracks/" + firstTrack},
		{method: http.MethodPut, path: ratingsPath + firstTrack, body: stars(3)},
		{method: http.MethodDelete, path: ratingsPath + firstTrack},
	}
	for _, c := range calls {
		c.header = cookieOnly(sess.token)
		if c.body != "" {
			c.header.Set(contentType, api.JSONMediaType)
		}
		ts.send(t, c).problem(t, http.StatusForbidden)
		c.header.Set(requestHeader, "1")
		wantNoContent(t, ts.send(t, c))
	}
}
