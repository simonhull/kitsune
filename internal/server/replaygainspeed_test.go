package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

// The ReplayGain speed shapes: a run of 1,000 songs in 100 albums, and the
// status of 50,000 songs with 2,000 of them to write.
const (
	gainRunAlbums   = 100
	gainRunPerAlbum = 10
	gainPlanSongs   = 50_000
	gainPlanToWrite = 2_000
	gainPlanPerDisc = 10
	// loudnessVersion is the library's measurement version, which a stored
	// measure must have to count.
	loudnessVersion = 1
)

// TestReplayGainRunSpeed times a run that writes 1,000 real FLACs in 100
// albums, in place as no stream has them, reads each back and journals it, as
// milliseconds per song. Runs and undos take turns, so each writes them all.
func TestReplayGainRunSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "gain-run")
	root := t.TempDir()
	gainSines(t, root)
	ts := newTestServer(t, testServerOptions{LibraryRoot: root, TagWrites: true})
	if _, err := ts.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	const songs = gainRunAlbums * gainRunPerAlbum
	took := make([]time.Duration, speedRuns+1)
	last := ""
	for i := range took {
		start := time.Now()
		run, err := ts.startGainOrUndo(t, last)
		if err != nil {
			t.Fatal(err)
		}
		run.fn(t.Context())
		took[i] = time.Since(start) / songs
		info, err := ts.Store.GainRunByULID(t.Context(), run.id)
		if err != nil || info.Written != songs {
			t.Fatalf("run %d = %+v, %v; want all %d written", i, info, err, songs)
		}
		last = ""
		if info.Kind == store.GainKindRun {
			last = run.id
		}
	}
	holdSpeed(t, b, limit, took[1:]) // The first warms up.
}

// gainSines writes the speed test's library under root: 100 albums of ten
// copies of the sine, each album tagged its own.
func gainSines(t *testing.T, root string) {
	t.Helper()
	sine, err := os.ReadFile(filepath.Join("..", "pcm", "testdata", "sine_24.flac"))
	if err != nil {
		t.Fatal(err)
	}
	for a := range gainRunAlbums {
		first := filepath.Join(root, fmt.Sprintf("a%03d", a), "01.flac")
		writeFile(t, first, sine)
		err = audiometa.Write(first, func(f *audiometa.File) error {
			f.Tags.Album, f.Tags.AlbumArtist = fmt.Sprintf("Album %d", a), fmt.Sprintf("Artist %d", a)
			f.Tags.Artists = []string{f.Tags.AlbumArtist}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		tagged, readErr := os.ReadFile(first)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for n := 2; n <= gainRunPerAlbum; n++ {
			writeFile(t, filepath.Join(root, fmt.Sprintf("a%03d", a), fmt.Sprintf("%02d.flac", n)), tagged)
		}
	}
}

// startGainOrUndo starts a run of every album, or with run, an undo of it.
func (ts *testServer) startGainOrUndo(t *testing.T, run string) (startedGain, error) {
	t.Helper()
	var r library.GainRun
	var out startedGain
	var err error
	if run == "" {
		r, out.fn, err = ts.Scanner.StartGain(t.Context(), nil, 0)
	} else {
		r, out.fn, err = ts.Scanner.StartGainUndo(t.Context(), run, 0)
	}
	out.id = r.ID
	return out, err //nolint:wrapcheck // A test's: it fails on any.
}

// startedGain is a run started: its public ID, and the func that runs it.
type startedGain struct {
	id string
	fn func(context.Context)
}

// writeFile writes data at path, making its folders.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // G703: under the test's temp root.
		t.Fatal(err)
	}
}

// TestReplayGainStatusSpeed times GET /admin/replaygain at 50,000 songs, 2,000
// of them measured and without ReplayGain, on 200 albums: one statement over
// the songs with no ReplayGain, and no file read.
func TestReplayGainStatusSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "gain-status")
	ts := newTestServer(t, testServerOptions{TagWrites: true})
	at := time.UnixMilli(1_700_000_000_000)
	tws := make([]store.TrackWrite, 0, gainPlanSongs)
	gain := store.ReplayGain{TrackGain: -7, TrackPeak: 0.9, AlbumGain: -7, AlbumPeak: 0.9}
	for a := range gainPlanSongs / gainPlanPerDisc {
		artist := person(a%4000+1, fmt.Sprintf("Artist %d", a%4000))
		for n := 1; n <= gainPlanPerDisc; n++ {
			tw := organizeSong(musicBase, fmt.Sprintf("a%d/%d.flac", a, n), a+1, n, &artist, 4096, at)
			if a >= gainPlanToWrite/gainPlanPerDisc {
				tw.ReplayGain = gain
			}
			tws = append(tws, tw)
		}
	}
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
	ts.measureUngained(t)
	token := ts.tokenFor(t, store.RoleAdmin)
	var st api.ReplayGainStatus
	ts.getJSON(t, token, gainPath, &st)
	if st.Songs != gainPlanToWrite {
		t.Fatalf("status = %+v, want %d to write", st, gainPlanToWrite)
	}
	holdSpeed(t, b, limit, ts.timeCalls(t, token, http.MethodGet, gainPath, ""))
}

// measureUngained stores a measure for every song with no ReplayGain, as the
// loudness pass would, and pools each album.
func (ts *testServer) measureUngained(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	waiting, err := ts.Store.LoudnessWaiting(ctx, store.FileKey{}, loudnessVersion, gainPlanSongs)
	if err != nil {
		t.Fatal(err)
	}
	m := &store.TrackLoudness{Outcome: store.LoudnessMeasured, Integrated: -20, TruePeak: 0.5, SamplePeak: 0.5}
	albums := map[int64]bool{}
	for i := range waiting {
		w := &waiting[i]
		if _, err = ts.Store.SetTrackLoudness(ctx, w.ID, w.Size, w.MTime, loudnessVersion, m); err != nil {
			t.Fatal(err)
		}
		albums[w.AlbumID] = true
	}
	for album := range albums {
		_, err = ts.Store.PoolAlbum(ctx, album, func([]store.PooledTrack) (store.AlbumLoudness, error) {
			return store.AlbumLoudness{Integrated: -20, TruePeak: 0.5}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
