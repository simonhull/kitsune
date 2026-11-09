package library

import (
	"context"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/store"
)

// The pcm package's fixtures: a 1 kHz sine at -20 dBFS, measured by ffmpeg's
// ebur128 at -20.0 LUFS as FLAC and -20.4 as MP3, which keeps the encoder's
// silence at its ends.
const (
	sineFLAC = "../../pcm/testdata/sine_24.flac"
	sineMP3  = "../../pcm/testdata/sine_stereo.mp3"
	sines    = "Sines"
)

// measureLoudness runs a loudness pass, as the server does after a scan, and
// fails the test on error.
func (l *testLib) measureLoudness() LoudnessStats {
	l.t.Helper()
	stats, err := l.scanner.MeasureLoudness(l.t.Context())
	if err != nil {
		l.t.Fatalf("MeasureLoudness: %v", err)
	}
	return stats
}

// sineAlbum is a library of one album, Sines: a FLAC and an MP3 the pass
// measures and an M4A it can't, none of them with ReplayGain. It is scanned.
func sineAlbum(t *testing.T) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	root := l.roots[0]
	for rel, sample := range map[string]string{"Sines/1.flac": sineFLAC, "Sines/2.mp3": sineMP3, "Sines/3.m4a": sampleM4A} {
		l.copyTo(root, rel, sample)
		retag(t, filepath.Join(root, rel), sines)
	}
	l.scan(Quick)
	return l
}

// byFormat returns the album's track of format.
func byFormat(t *testing.T, d *store.AlbumDetail, format string) store.Track {
	t.Helper()
	i := slices.IndexFunc(d.Tracks, func(tr store.Track) bool { return tr.Format == format })
	if i < 0 {
		t.Fatalf("no %s track on %s", format, d.Title)
	}
	return d.Tracks[i]
}

func near(t *testing.T, what string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %.3f, want %.2f ±%.2f", what, got, want, tolerance)
	}
}

func TestLoudnessPassMeasuresAlbumsWithoutGain(t *testing.T) {
	t.Parallel()
	l := sineAlbum(t)

	if got := l.measureLoudness(); got != (LoudnessStats{Measured: 2, Unmeasurable: 1, Albums: 1}) {
		t.Errorf("pass = %+v, want 2 measured, the M4A unmeasurable, and the album", got)
	}
	d := l.albumNamed(sines)
	wantMeasures(t, &d)
	if got := l.scanner.Loudness(); got != (LoudnessProgress{Counts: store.LoudnessCounts{Ungained: 3, Measured: 2, Unmeasurable: 1}}) {
		t.Errorf("progress = %+v", got)
	}
	if got := l.measureLoudness(); got != (LoudnessStats{}) {
		t.Errorf("second pass = %+v, want nothing to do", got)
	}
}

// wantMeasures checks the Sines album holds the measures its files should.
func wantMeasures(t *testing.T, d *store.AlbumDetail) {
	t.Helper()
	flacTrack, mp3Track := byFormat(t, d, "FLAC").Measured, byFormat(t, d, "MP3").Measured
	if !flacTrack.HasTrack || !mp3Track.HasTrack || !flacTrack.HasAlbum {
		t.Fatalf("measured = %+v and %+v", flacTrack, mp3Track)
	}
	near(t, "FLAC", flacTrack.Track, -20.0, 0.1)
	near(t, "MP3", mp3Track.Track, -20.4, 0.1)
	near(t, "FLAC's true peak", 20*math.Log10(flacTrack.TrackPeak), -20, 0.05)
	if flacTrack.Album < mp3Track.Track || flacTrack.Album > flacTrack.Track || mp3Track.Album != flacTrack.Album {
		t.Errorf("album = %.3f and %.3f, want one value between its tracks'", flacTrack.Album, mp3Track.Album)
	}
	if got := byFormat(t, d, "M4A").Measured; got != (store.MeasuredLoudness{}) {
		t.Errorf("the M4A shows a measure: %+v", got)
	}
}

func TestLoudnessPassMeasuresAChangedFileAgain(t *testing.T) {
	t.Parallel()
	l := sineAlbum(t)
	l.measureLoudness()
	bump(t, filepath.Join(l.roots[0], sines, "1.flac"), 1)
	l.scan(Quick)
	if got := l.measureLoudness(); got != (LoudnessStats{Measured: 1, Albums: 1}) {
		t.Errorf("pass after a change = %+v, want the file and its album", got)
	}
}

func TestLoudnessPassLeavesGainedAlbumsAlone(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	path := filepath.Join(l.roots[0], "Gained", "1.flac")
	l.copyTo(l.roots[0], "Gained/1.flac", sineFLAC)
	err := audiometa.Write(path, func(f *audiometa.File) error {
		f.Tags.Album = "Gained"
		f.Tags.Set("REPLAYGAIN_TRACK_GAIN", "+2.00 dB")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	l.scan(Quick)
	if got := l.measureLoudness(); got != (LoudnessStats{}) {
		t.Errorf("pass = %+v, want nothing: its file has gain", got)
	}
	if got := l.scanner.Loudness().Counts; got != (store.LoudnessCounts{}) {
		t.Errorf("counts = %+v, want none without gain", got)
	}
}

// TestLoudnessPassGivesWayToAnEdit holds an edit waiting while a file is being
// measured: the measurement stops for it, and the file is measured after.
func TestLoudnessPassGivesWayToAnEdit(t *testing.T) {
	t.Parallel()
	l := sineAlbum(t)
	log := &passLog{}
	read := l.scanner.measure
	first := true
	l.scanner.measure = func(ctx context.Context, row *store.LoudnessWaiting, yield func() bool) (loudness.Result, error) {
		if first {
			first = false
			go func() {
				if err := l.scanner.exclusive(ctx, func() error { log.add("edit"); return nil }); err != nil {
					log.add("edit failed: " + err.Error())
				}
			}()
			for !yield() {
				time.Sleep(time.Millisecond)
			}
			log.add("gave way")
			return read(ctx, row, yield)
		}
		log.add("measured " + filepath.Base(row.RelPath))
		return read(ctx, row, yield)
	}
	if got := l.measureLoudness(); got != (LoudnessStats{Measured: 2, Unmeasurable: 1, Albums: 1}) {
		t.Errorf("pass = %+v", got)
	}
	want := []string{"gave way", "edit", "measured 1.flac", "measured 2.mp3", "measured 3.m4a"}
	if got := log.all(); !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}
