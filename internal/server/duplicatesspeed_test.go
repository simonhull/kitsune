package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// The duplicates speed shape: 50,000 songs in albums of ten, one in fifty a
// copy of a song on the album before it (2% in groups), and one in three
// tagged with a MusicBrainz recording of its own.
const (
	dupesSpeedSongs = 50_000
	dupesEvery      = 50
	dupesTagged     = 3
)

// TestDuplicatesFindSpeed times GET /admin/duplicates at 50,000 songs, each
// time after a fingerprint has landed, so the find is worked out again from
// the rows the store keeps for the index, as when the page opens after the
// fingerprint pass moved on.
func TestDuplicatesFindSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "duplicates-find")
	ts := newTestServer(t, testServerOptions{TagWrites: true})
	tws := dupesSpeedLibrary()
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
	token := ts.tokenFor(t, store.RoleAdmin)
	var l api.DuplicateList
	ts.getJSON(t, token, duplicatesPath, &l)
	if c := l.Counts; c.Library != dupesSpeedSongs || c.Songs != 2*(dupesSpeedSongs/dupesEvery-1) {
		t.Fatalf("counts = %+v, want %d songs in pairs of copies", c, 2*(dupesSpeedSongs/dupesEvery-1))
	}
	first := ts.firstTrack(t)
	took := make([]time.Duration, speedRuns+1)
	for i := range took {
		// A fingerprint landing makes the next find work it out again.
		if _, err := ts.Store.SetDigest(t.Context(), first, 1, time.UnixMilli(1), [32]byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		resp := ts.send(t, call{method: http.MethodGet, path: duplicatesPath, header: bearer(token)})
		took[i] = time.Since(start)
		if resp.Status != http.StatusOK {
			t.Fatalf("GET %s = %d", duplicatesPath, resp.Status)
		}
	}
	holdSpeed(t, b, limit, took[1:]) // The first warms up.
}

// dupesSpeedLibrary is the speed shape's songs.
func dupesSpeedLibrary() []store.TrackWrite {
	tws := make([]store.TrackWrite, 0, dupesSpeedSongs)
	for a := range dupesSpeedSongs / organizeSpeedPerDisc {
		artist := person(a%4000+1, fmt.Sprintf("Artist %d", a%4000))
		for n := 1; n <= organizeSpeedPerDisc; n++ {
			tw := organizeSong(musicBase, fmt.Sprintf("a%d/%d.flac", a+1, n), a+1, n, &artist, 1, time.UnixMilli(1))
			tw.Title = fmt.Sprintf("Song %d of %d", n, a+1)
			i := len(tws)
			if i%dupesEvery == 0 && a > 0 {
				tw.Title = fmt.Sprintf("Song %d of %d", n, a) // The same song as on the album before, by the same artist.
				tw.Artists = tws[i-organizeSpeedPerDisc].Artists
			}
			if i%dupesTagged == 0 {
				tw.Tags = []store.Tag{{Key: "MUSICBRAINZ_TRACKID", Value: fmt.Sprintf("rec-%d", i)}}
			}
			tws = append(tws, tw)
		}
	}
	return tws
}

// firstTrack returns the row of the library's first song.
func (ts *testServer) firstTrack(t *testing.T) int64 {
	t.Helper()
	lib, err := ts.Store.OrganizeLibrary(t.Context())
	if err != nil || len(lib.Songs) == 0 {
		t.Fatalf("library: %v", err)
	}
	return lib.Songs[0].ID
}
