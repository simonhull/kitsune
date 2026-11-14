package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const homePath = "/me/home"

// recordPlay records a play of album n's track t by the user, at the time.
func (ts *testServer) recordPlay(tb testing.TB, userID int64, n, t int, at time.Time) {
	tb.Helper()
	if err := ts.Store.RecordPlay(tb.Context(), &store.NewPlay{
		UserID: userID, TrackID: fmt.Sprintf("01J%020d%03d", n, t), PlayedAt: at, Source: playSource,
		ClientID: fmt.Sprintf("%d-%d-%d", n, t, at.UnixMilli()),
	}); err != nil {
		tb.Fatal(err)
	}
}

func TestHomeListsRecentAndOnRepeat(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2}, browseAlbum{n: 2, sortName: "b", tracks: 1},
		browseAlbum{n: 3, sortName: "c", tracks: 1})
	sess := ts.sessionFor(t)
	other := ts.tokenFor(t, store.RoleUser)
	now := ts.Clock.Now()
	ts.recordPlay(t, sess.user.ID, 2, 1, now.Add(-120*24*time.Hour)) // Outside the window.
	ts.recordPlay(t, sess.user.ID, 1, 1, now.Add(-2*time.Hour))
	ts.recordPlay(t, sess.user.ID, 1, 2, now.Add(-90*time.Minute))
	ts.recordPlay(t, sess.user.ID, 3, 1, now.Add(-time.Hour))

	var got api.Home
	ts.getJSON(t, sess.token, homePath, &got)
	recent := make([]string, len(got.Recent))
	for i := range got.Recent {
		recent[i] = got.Recent[i].ID
	}
	if want := []string{albumULID(3), albumULID(1), albumULID(2)}; !slices.Equal(recent, want) {
		t.Errorf("recent = %v, want %v", recent, want)
	}
	onRepeat := make([]string, len(got.OnRepeat))
	for i := range got.OnRepeat {
		onRepeat[i] = got.OnRepeat[i].Album.ID + "×" + strconv.Itoa(got.OnRepeat[i].Plays)
	}
	if want := []string{albumULID(1) + "×2", albumULID(3) + "×1"}; !slices.Equal(onRepeat, want) {
		t.Errorf("on repeat = %v, want %v: album 2's play is over 90 days old", onRepeat, want)
	}
	if a := got.Recent[1]; a.Title != albumOne || a.Artist != annName || a.TrackCount != 2 {
		t.Errorf("album = %+v, want album 1 as GET /albums lists it", a)
	}

	ts.getJSON(t, other, homePath, &got)
	if len(got.Recent) != 0 || len(got.OnRepeat) != 0 {
		t.Errorf("another person's home = %+v, want nothing played", got)
	}
}

// historyPlays is how many plays the history budget imports for one person, on
// top of seedBench's.
const historyPlays = 200_000

// seedHistory imports historyPlays plays by the person seedBench signed in,
// one every 5 minutes back from now, over the shape's songs: far denser than
// anyone listens, so On repeat's 90 days hold about 26,000 of them.
func (ts *testServer) seedHistory(tb testing.TB, tracks, albums int) {
	tb.Helper()
	user, err := ts.Store.UserByName(tb.Context(), simonName)
	if err != nil {
		tb.Fatal(err)
	}
	perAlbum := tracks / albums
	imp := store.Import{UserID: user.ID, Plays: make([]store.NewPlay, historyPlays)}
	for i := range imp.Plays {
		n := i * 7919 % albums // A prime stride: plays wander over the albums.
		imp.Plays[i] = store.NewPlay{
			TrackID:  fmt.Sprintf("01J%020d%03d", n+1, i%perAlbum),
			PlayedAt: ts.Clock.Now().Add(-time.Duration(i) * 5 * time.Minute), Source: "navidrome", ClientID: "h" + strconv.Itoa(i),
		}
	}
	if _, err = ts.Store.ApplyImport(tb.Context(), &imp); err != nil {
		tb.Fatal(err)
	}
}

// TestHomeHistorySpeed times GET /me/home for a person with historyPlays
// plays, of the large library.
func TestHomeHistorySpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "home-history")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	token := ts.seedBench(t, shape.tracks, shape.albums, shape.artists)
	ts.seedHistory(t, shape.tracks, shape.albums)
	holdSpeed(t, b, limit, ts.timeGets(t, token, homePath))
}

func TestHomeNeedsASession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: homePath, invalid: true}).problem(t, http.StatusUnauthorized)
}
