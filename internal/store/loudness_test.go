package store_test

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const loudnessVersion = 1

// onAlbum is newWrite(i) on newWrite(album)'s album.
func onAlbum(i, album int) store.TrackWrite {
	w := newWrite(i)
	w.Album = newWrite(album).Album
	return w
}

func waitingPaths(t *testing.T, s *store.Store, version int) []string {
	t.Helper()
	rows, err := s.LoudnessWaiting(t.Context(), store.FileKey{}, version, 100)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.CountLoudnessWaiting(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(rows) {
		t.Errorf("CountLoudnessWaiting = %d, LoudnessWaiting gives %d", n, len(rows))
	}
	paths := make([]string, len(rows))
	for i := range rows {
		paths[i] = rows[i].RelPath
	}
	return paths
}

// measured is a measure of a file of loudness and true peak.
func measured(loudness, peak float64) store.TrackLoudness {
	return store.TrackLoudness{Outcome: store.LoudnessMeasured, Integrated: loudness, TruePeak: peak, SamplePeak: peak, Blocks: []byte{1}}
}

// measure stores m as the measure of w's file as it was written.
func measure(t *testing.T, s *store.Store, w *store.TrackWrite, m store.TrackLoudness) {
	t.Helper()
	ok, err := s.SetTrackLoudness(t.Context(), idOf(t, s, w), w.Size, w.MTime, loudnessVersion, &m)
	if err != nil || !ok {
		t.Fatalf("SetTrackLoudness = %v, %v", ok, err)
	}
}

func toPool(t *testing.T, s *store.Store) []int64 {
	t.Helper()
	ids, err := s.AlbumsToPool(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// pooling is what came of pooling an album: whether it was ready, and how
// many measured tracks the pool saw.
type pooling struct {
	ready bool
	seen  int
}

// pool pools album at -9 LUFS and the largest of its tracks' peaks.
func pool(t *testing.T, s *store.Store, album int64) pooling {
	t.Helper()
	var p pooling
	var err error
	p.ready, err = s.PoolAlbum(t.Context(), album, func(tracks []store.PooledTrack) (store.AlbumLoudness, error) {
		p.seen = len(tracks)
		peak := 0.0
		for _, tr := range tracks {
			peak = max(peak, tr.TruePeak)
		}
		return store.AlbumLoudness{Integrated: -9, TruePeak: peak}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// shownTrack is w as its album page shows it.
func shownTrack(t *testing.T, s *store.Store, w *store.TrackWrite) store.Track {
	t.Helper()
	d, err := s.Album(t.Context(), w.Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(d.Tracks, func(tr store.Track) bool { return tr.ID == w.ULID })
	if i < 0 {
		t.Fatalf("%s is not on its album", w.ULID)
	}
	return d.Tracks[i]
}

// halfGained is an album of two songs, the second with ReplayGain, and an
// album of one song with it.
type halfGained struct {
	s                   *store.Store
	bare, tagged, alone store.TrackWrite
	album               int64 // The first album's row ID.
}

func newHalfGained(t *testing.T) *halfGained {
	t.Helper()
	f := &halfGained{s: openUsers(t), bare: onAlbum(0, 0), tagged: onAlbum(1, 0), alone: newWrite(2)}
	f.tagged.ReplayGain = store.ReplayGain{TrackGain: -7, TrackPeak: 0.9}
	f.alone.ReplayGain = f.tagged.ReplayGain
	writeAll(t, f.s, f.bare, f.tagged, f.alone)
	rows, err := f.s.LoudnessWaiting(t.Context(), store.FileKey{}, loudnessVersion, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	f.album = rows[0].AlbumID
	return f
}

// TestLoudnessWaitsForWholeAlbums: the whole album waits, its tagged song too,
// so its album measure pools all of it; an album with gain on every song
// doesn't.
func TestLoudnessWaitsForWholeAlbums(t *testing.T) {
	t.Parallel()
	f := newHalfGained(t)
	if got, want := waitingPaths(t, f.s, loudnessVersion), []string{f.bare.RelPath, f.tagged.RelPath}; !slices.Equal(got, want) {
		t.Fatalf("waiting = %q, want %q", got, want)
	}
	stale := measured(-6, 1.1)
	ok, err := f.s.SetTrackLoudness(t.Context(), idOf(t, f.s, &f.bare), f.bare.Size+1, f.bare.MTime, loudnessVersion, &stale)
	if err != nil || ok {
		t.Errorf("a measure of another file = %v, %v; want it refused", ok, err)
	}
	measure(t, f.s, &f.bare, measured(-6, 1.1))
	if p := pool(t, f.s, f.album); p.ready {
		t.Error("an album pooled while a song waits")
	}
	if got := toPool(t, f.s); len(got) != 0 {
		t.Errorf("AlbumsToPool = %v while a song waits", got)
	}
}

func TestLoudnessPoolsAMeasuredAlbum(t *testing.T) {
	t.Parallel()
	f := newHalfGained(t)
	measure(t, f.s, &f.bare, measured(-6, 1.1))
	measure(t, f.s, &f.tagged, store.TrackLoudness{Outcome: store.LoudnessUnsupported})
	if got := toPool(t, f.s); !slices.Equal(got, []int64{f.album}) {
		t.Fatalf("AlbumsToPool = %v, want [%d]", got, f.album)
	}
	if p := pool(t, f.s, f.album); p != (pooling{ready: true, seen: 1}) {
		t.Fatalf("pool = %+v, want the one measured", p)
	}
	if got := toPool(t, f.s); len(got) != 0 {
		t.Errorf("AlbumsToPool = %v after pooling", got)
	}
	if got := waitingPaths(t, f.s, loudnessVersion); len(got) != 0 {
		t.Errorf("waiting = %q after every file was measured", got)
	}
	want := store.MeasuredLoudness{Track: -6, TrackPeak: 1.1, HasTrack: true, Album: -9, AlbumPeak: 1.1, HasAlbum: true}
	if got := shownTrack(t, f.s, &f.bare).Measured; got != want {
		t.Errorf("measured = %+v, want %+v", got, want)
	}
	if got := shownTrack(t, f.s, &f.tagged).Measured; got != (store.MeasuredLoudness{}) {
		t.Errorf("an unmeasurable song shows %+v", got)
	}
	counts, err := f.s.CountLoudness(t.Context())
	if err != nil || counts != (store.LoudnessCounts{Ungained: 1, Measured: 1}) {
		t.Errorf("counts = %+v, %v", counts, err)
	}
	// A new version of the measurement waits for every file again.
	if got := waitingPaths(t, f.s, loudnessVersion+1); len(got) != 2 {
		t.Errorf("waiting at a new version = %q", got)
	}
}

func TestLoudnessFollowsAlbumChanges(t *testing.T) {
	t.Parallel()
	f := newHalfGained(t)
	measure(t, f.s, &f.bare, measured(-6, 1.1))
	measure(t, f.s, &f.tagged, measured(-8, 0.9))
	pool(t, f.s, f.album)

	// A song joining the album makes it pool again once measured.
	joined := onAlbum(3, 0)
	writeAll(t, f.s, joined)
	if got := waitingPaths(t, f.s, loudnessVersion); !slices.Equal(got, []string{joined.RelPath}) {
		t.Fatalf("waiting = %q, want the new song", got)
	}
	measure(t, f.s, &joined, measured(-10, 0.5))
	if got := toPool(t, f.s); !slices.Equal(got, []int64{f.album}) {
		t.Errorf("AlbumsToPool = %v after a song joined, want [%d]", got, f.album)
	}
	if p := pool(t, f.s, f.album); p != (pooling{ready: true, seen: 3}) {
		t.Errorf("pool = %+v, want all three", p)
	}

	// A changed file's measure no longer shows, and it waits again.
	changed := f.bare
	changed.ID = idOf(t, f.s, &f.bare)
	changed.MTime = changed.MTime.Add(time.Minute)
	writeAll(t, f.s, changed)
	if got := shownTrack(t, f.s, &f.bare).Measured; got.HasTrack || got.HasAlbum {
		t.Errorf("a changed file shows its old measure: %+v", got)
	}
	if got := waitingPaths(t, f.s, loudnessVersion); !slices.Equal(got, []string{f.bare.RelPath}) {
		t.Errorf("waiting = %q, want the changed file", got)
	}
}

func TestSilentMeasureHasNoLoudness(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	writeAll(t, s, w)
	measure(t, s, &w, store.TrackLoudness{Outcome: store.LoudnessMeasured, Silent: true, Blocks: []byte{1}})
	albums := toPool(t, s)
	if len(albums) != 1 {
		t.Fatalf("AlbumsToPool = %v", albums)
	}
	ok, err := s.PoolAlbum(t.Context(), albums[0], func([]store.PooledTrack) (store.AlbumLoudness, error) {
		return store.AlbumLoudness{Silent: true}, nil
	})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if got := shownTrack(t, s, &w).Measured; got.HasTrack || got.HasAlbum {
		t.Errorf("silence shows a loudness: %+v", got)
	}
}
