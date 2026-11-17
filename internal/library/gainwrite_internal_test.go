package library

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	formatMP3 = "MP3"
	sinesFLAC = "Sines/1.flac"
	sinesMP3  = "Sines/2.mp3"
	sinesM4A  = "Sines/3.m4a"
	tones     = "Tones"
	tonesFLAC = "Tones/1.flac"
)

// gainLib is a library with tag writes on of the Sines album (a FLAC and an
// MP3 the loudness pass measures, an M4A it can't) and the Tones album (one
// FLAC), none with ReplayGain, scanned, fingerprinted and measured. prep, when
// not nil, changes a file before the scan.
func gainLib(t *testing.T, prep func(rel, path string)) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	for rel, sample := range map[string]string{sinesFLAC: sineFLAC, sinesMP3: sineMP3, sinesM4A: sampleM4A, tonesFLAC: sineFLAC} {
		l.copyTo(root, rel, sample)
		retag(t, filepath.Join(root, rel), filepath.Dir(rel))
		if prep != nil {
			prep(rel, filepath.Join(root, rel))
		}
	}
	l.scan(Quick)
	l.fingerprint()
	l.measureLoudness()
	return l
}

// gainRun writes every album it can, or those named, to the end, and returns
// the run.
func (l *testLib) gainRun(albums ...string) store.GainRunInfo {
	l.t.Helper()
	run, fn, err := l.scanner.StartGain(l.t.Context(), albums, 0)
	if err != nil {
		l.t.Fatalf("StartGain: %v", err)
	}
	fn(l.t.Context())
	return l.gainInfo(run.ID)
}

// gainUndo undoes the run id to the end, and returns the undo.
func (l *testLib) gainUndo(id string) store.GainRunInfo {
	l.t.Helper()
	run, fn, err := l.scanner.StartGainUndo(l.t.Context(), id, 0)
	if err != nil {
		l.t.Fatalf("StartGainUndo: %v", err)
	}
	fn(l.t.Context())
	return l.gainInfo(run.ID)
}

// gainInfo returns the run with the public ID id.
func (l *testLib) gainInfo(id string) store.GainRunInfo {
	l.t.Helper()
	info, err := l.st.GainRunByULID(l.t.Context(), id)
	if err != nil {
		l.t.Fatal(err)
	}
	return info
}

// fileGain reads the ReplayGain tags of the file at rel.
func (l *testLib) fileGain(rel string) gainTags {
	l.t.Helper()
	f, err := audiometa.Open(filepath.Join(l.roots[0], rel))
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	return gainTagsOf(f)
}

// plays returns what each track of Sines plays at, by format: the gains
// Kitsune sends, tagged or measured, rounded as a tag holds them.
func (l *testLib) plays() map[string]store.GainValues {
	l.t.Helper()
	d := l.albumNamed(sines)
	out := map[string]store.GainValues{}
	for i := range d.Tracks {
		tr := &d.Tracks[i]
		if rg := tr.ReplayGain; rg.TrackGain != 0 || rg.AlbumGain != 0 {
			out[tr.Format] = store.GainValues{
				TrackGain: roundTo(rg.TrackGain, gainStep), TrackPeak: roundTo(rg.TrackPeak, peakStep),
				AlbumGain: roundTo(rg.AlbumGain, gainStep), AlbumPeak: roundTo(rg.AlbumPeak, peakStep), HasAlbum: rg.AlbumGain != 0,
			}
			continue
		}
		if tr.Measured.HasTrack {
			out[tr.Format] = gainValuesOf(&store.GainMeasure{
				Track: tr.Measured.Track, TrackPeak: tr.Measured.TrackPeak,
				Album: tr.Measured.Album, AlbumPeak: tr.Measured.AlbumPeak, HasAlbum: tr.Measured.HasAlbum,
			})
		}
	}
	return out
}

// tagged returns, by format, whether each track of Sines reads a gain from
// its tags.
func (l *testLib) tagged() map[string]bool {
	l.t.Helper()
	d := l.albumNamed(sines)
	out := map[string]bool{}
	for i := range d.Tracks {
		out[d.Tracks[i].Format] = d.Tracks[i].ReplayGain.TrackGain != 0 || d.Tracks[i].ReplayGain.AlbumGain != 0
	}
	return out
}

// gainLeft returns why run left each song, by path.
func (l *testLib) gainLeft(run int64) map[string]string {
	l.t.Helper()
	left, err := l.st.LeftGain(l.t.Context(), run, 100)
	if err != nil {
		l.t.Fatal(err)
	}
	out := map[string]string{}
	for _, s := range left {
		out[s.RelPath] = s.Reason
	}
	return out
}

func TestGainRunWritesWhatKitsunePlays(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	before := l.plays()
	if got, want := l.gainCounts(), (gainCounts{songs: 3, albums: 2, unmeasurable: 1}); got != want {
		t.Fatalf("gain library = %+v, want %+v", got, want)
	}

	run := l.gainRun()

	if got, want := ranOf(&run), (ran{state: store.GainDone, written: 3, total: 3, albums: 2}); got != want {
		t.Errorf("run = %+v, want %+v", got, want)
	}
	l.wantWrittenAsPlayed(before)
	l.wantPassesIdle()
	if lib := l.gainLibrary(); lib.Songs() != 0 {
		t.Errorf("gain library after = %d songs, want nothing left to write", lib.Songs())
	}
}

// gainCounts is what a run may write now, counted.
type gainCounts struct{ songs, albums, waiting, unmeasurable int }

// gainCounts counts what a run may write now.
func (l *testLib) gainCounts() gainCounts {
	l.t.Helper()
	lib := l.gainLibrary()
	return gainCounts{songs: lib.Songs(), albums: len(lib.Albums), waiting: lib.Waiting, unmeasurable: lib.Unmeasurable}
}

// ran is what a run did, counted.
type ran struct {
	state                        string
	written, left, total, albums int
}

// ranOf counts what run did.
func ranOf(run *store.GainRunInfo) ran {
	return ran{state: run.State, written: run.Written, left: run.Left, total: run.Total, albums: run.Albums}
}

// gainLibrary returns what a run may write now.
func (l *testLib) gainLibrary() *store.GainLibrary {
	l.t.Helper()
	lib, err := l.scanner.GainLibrary(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	return lib
}

// wantWrittenAsPlayed fails unless Sines' FLAC and MP3 are tagged with the four
// values they played at before, and play at them still.
func (l *testLib) wantWrittenAsPlayed(before map[string]store.GainValues) {
	l.t.Helper()
	if got := l.tagged(); !got[formatFLAC] || !got[formatMP3] || got[formatM4A] {
		l.t.Errorf("tagged = %v, want the FLAC and the MP3", got)
	}
	if after := l.plays(); after[formatFLAC] != before[formatFLAC] || after[formatMP3] != before[formatMP3] || !after[formatFLAC].HasAlbum {
		l.t.Errorf("plays %+v after, %+v before: nothing heard may change", after, before)
	}
	for rel, format := range map[string]string{sinesFLAC: formatFLAC, sinesMP3: formatMP3} {
		v := before[format]
		if tags := l.fileGain(rel); !tags.reads(&v) || len(tags) != 4 {
			l.t.Errorf("%s holds %v, want the four tags of %+v", rel, tags, v)
		}
	}
}

// wantPassesIdle fails unless the fingerprints and the measures are current,
// so neither pass has work, every measured song keeps its measure, and a scan
// reads nothing.
func (l *testLib) wantPassesIdle() {
	l.t.Helper()
	if n, err := l.st.CountUndigested(l.t.Context()); err != nil || n != 0 {
		l.t.Errorf("undigested = %d, %v; want none", n, err)
	}
	if got := l.measureLoudness(); got != (LoudnessStats{}) {
		l.t.Errorf("loudness pass after = %+v, want nothing to do", got)
	}
	d := l.albumNamed(sines)
	for i := range d.Tracks {
		if tr := &d.Tracks[i]; tr.Format != formatM4A && !tr.Measured.HasTrack {
			l.t.Errorf("%s lost its measure", tr.Format)
		}
	}
	if st := l.scan(Quick); st.Read != 0 || st.Updated != 0 {
		l.t.Errorf("a scan after the run = %+v, want nothing read", st)
	}
}

func TestGainRunOfOneAlbum(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run := l.gainRun(l.albumNamed(tones).ID)
	if run.Written != 1 || run.Albums != 1 {
		t.Errorf("run = %+v, want Tones alone", run)
	}
	if got := l.tagged(); got[formatFLAC] {
		t.Error("the run wrote Sines too")
	}
	if _, _, err := l.scanner.StartGain(t.Context(), []string{"01HZZZZZZZZZZZZZZZZZZZZZZZ"}, 0); !errors.Is(err, ErrNothingToWrite) {
		t.Errorf("an unknown album = %v, want ErrNothingToWrite", err)
	}
	if _, _, err := l.scanner.StartGain(t.Context(), []string{l.albumNamed(tones).ID}, 0); !errors.Is(err, ErrNothingToWrite) {
		t.Errorf("an album written already = %v, want ErrNothingToWrite", err)
	}
}

// A song whose file holds a ReplayGain peak alone, which reads as no
// ReplayGain, is written over, and an undo puts the peak back as spelled.
func TestGainUndoPutsBackExactlyWhatWasThere(t *testing.T) {
	t.Parallel()
	l := gainLib(t, func(rel, path string) {
		if rel != sinesFLAC {
			return
		}
		err := audiometa.Write(path, func(f *audiometa.File) error {
			f.Tags.SetNamed("replaygain_track_peak", "0.500000")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	before := map[string]gainTags{sinesFLAC: l.fileGain(sinesFLAC), sinesMP3: l.fileGain(sinesMP3)}
	if len(before[sinesFLAC]) != 1 {
		t.Fatalf("before = %v, want the FLAC's peak", before)
	}
	played := l.plays()
	run := l.gainRun()

	undo := l.gainUndo(run.ULID)

	if got := ranOf(&undo); got.state != store.GainDone || got.written != 3 || undo.Kind != store.GainKindUndo {
		t.Errorf("undo = %+v, want 3 put back", undo)
	}
	for rel, want := range before {
		if got := l.fileGain(rel); !got.same(want) {
			t.Errorf("%s holds %v after the undo, want %v", rel, got, want)
		}
	}
	l.wantMeasuredAgain(played)
	l.wantUndone(run.ULID)
}

// wantUndone fails unless the run id is undone, and a second undo of it has
// nothing to do.
func (l *testLib) wantUndone(id string) {
	l.t.Helper()
	if got := l.gainInfo(id); got.State != store.GainUndone || got.StillWritten {
		l.t.Errorf("the run after its undo = %+v, want undone", got)
	}
	if _, _, err := l.scanner.StartGainUndo(l.t.Context(), id, 0); !errors.Is(err, ErrNothingToUndo) {
		l.t.Errorf("a second undo = %v, want ErrNothingToUndo", err)
	}
}

// wantMeasuredAgain fails unless Sines reads no gain from its tags, plays at
// its measure again, played, at once, and can be written again.
func (l *testLib) wantMeasuredAgain(played map[string]store.GainValues) {
	l.t.Helper()
	if got := l.tagged(); got[formatFLAC] || got[formatMP3] {
		l.t.Errorf("tagged after the undo = %v, want none", got)
	}
	if got := l.plays(); got[formatFLAC] != played[formatFLAC] || got[formatMP3] != played[formatMP3] {
		l.t.Errorf("plays %+v after the undo, want the measure again at once, %+v", got, played)
	}
	if got := l.measureLoudness(); got != (LoudnessStats{}) {
		l.t.Errorf("loudness pass after the undo = %+v, want nothing to do", got)
	}
	if lib := l.gainLibrary(); lib.Songs() != 3 {
		l.t.Errorf("gain library after the undo = %d songs, want all three to write again", lib.Songs())
	}
}

func TestGainUndoFindsASongTheOrganizerMoved(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run := l.gainRun()
	l.run(l.preview(OrganizeScope{}))
	wantFiles(t, l.roots[0], nil, []string{sinesFLAC})
	if t.Failed() {
		t.Fatal("the organizer moved nothing")
	}

	undo := l.gainUndo(run.ULID)

	if undo.Written != 3 {
		t.Errorf("undo = %+v, want 3 put back wherever they are", undo)
	}
	if got := l.tagged(); got[formatFLAC] || got[formatMP3] {
		t.Errorf("tagged after the undo = %v", got)
	}
}

func TestGainUndoLeavesASongRetaggedSince(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run := l.gainRun()
	err := audiometa.Write(filepath.Join(l.roots[0], sinesFLAC), func(f *audiometa.File) error {
		return f.SetTrackReplayGain(-3, 0.25)
	})
	if err != nil {
		t.Fatal(err)
	}
	l.scan(Quick)

	undo := l.gainUndo(run.ULID)

	if undo.Written != 2 || undo.Left != 1 {
		t.Errorf("undo = %+v, want two put back and one left", undo)
	}
	if got := l.gainLeft(undo.ID); got[sinesFLAC] != string(GainRetagged) {
		t.Errorf("left = %v, want the FLAC retagged", got)
	}
	if got := l.gainInfo(run.ULID); got.State == store.GainUndone {
		t.Error("a run with a write left in its file is undone")
	}
}

func TestGainUndoAfterATagEdit(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run := l.gainRun()
	d := l.albumNamed(tones)
	if _, err := l.scanner.EditAlbum(t.Context(), d.ID, d.ETag, &AlbumEdit{All: *yearEdit()}); err != nil {
		t.Fatal(err)
	}
	if undo := l.gainUndo(run.ULID); undo.Written != 3 {
		t.Errorf("undo = %+v, want every song put back", undo)
	}
}

func TestGainRunLeavesWhatChanged(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run, fn, err := l.scanner.StartGain(t.Context(), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	root := l.roots[0]
	bump(t, filepath.Join(root, sinesFLAC), 1)
	if err = os.Chmod(filepath.Join(root, sinesMP3), 0o400); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, tonesFLAC)); err != nil {
		t.Fatal(err)
	}
	fn(t.Context())

	info := l.gainInfo(run.ID)
	if info.State != store.GainDone || info.Written != 0 || info.Left != 3 {
		t.Errorf("run = %+v, want all three left", info)
	}
	want := map[string]string{sinesFLAC: string(GainChanged), sinesMP3: string(GainUnwritable), tonesFLAC: string(GainMissing)}
	if got := l.gainLeft(info.ID); !mapsEqual(got, want) {
		t.Errorf("left = %v, want %v", got, want)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestGainStopFinishesTheAlbumInHand(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	run, fn, err := l.scanner.StartGain(t.Context(), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	l.scanner.gainFault = func(step, _ string) {
		if step == gainStepWritten {
			if stopErr := l.scanner.StopGain(run.ID); stopErr != nil {
				t.Error(stopErr)
			}
		}
	}
	fn(t.Context())
	info := l.gainInfo(run.ID)
	if info.State != store.GainStopped || info.Written != 2 || info.AlbumsDone != 1 {
		t.Errorf("run = %+v, want Sines, its first album, written whole and then stopped", info)
	}
	if err = l.scanner.StopGain(run.ID); !errors.Is(err, ErrNotRunning) {
		t.Errorf("stop after the end = %v", err)
	}
}

// gainCrash stops a run where a server killed there would.
type gainCrash struct{}

// gainCrashAt runs every album, crashing at step's n-th call.
func (l *testLib) gainCrashAt(step string, n int) {
	l.t.Helper()
	_, fn, err := l.scanner.StartGain(l.t.Context(), nil, 0)
	if err != nil {
		l.t.Fatal(err)
	}
	calls := 0
	l.scanner.gainFault = func(at, _ string) {
		if at == step {
			calls++
			if calls == n {
				panic(gainCrash{})
			}
		}
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(gainCrash); !ok {
					panic(r)
				}
			}
		}()
		fn(l.t.Context())
	}()
}

// A run killed at each step is settled at the next start: nothing is planned,
// the run is stopped, the store reads what each file holds, and the songs it
// never wrote can be written again.
func TestGainRecoversFromACrashAtEachStep(t *testing.T) {
	t.Parallel()
	for _, step := range []struct {
		name string
		n    int
	}{{gainStepJournaled, 1}, {gainStepWritten, 1}, {gainStepWritten, 2}, {gainStepReindexed, 1}, {gainStepBatch, 1}} {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()
			l := gainLib(t, nil)
			played := l.plays()
			l.gainCrashAt(step.name, step.n)
			l.scanner = NewScanner(l.st, l.opts) // The next start.
			if err := l.scanner.RecoverGain(t.Context()); err != nil {
				t.Fatal(err)
			}
			l.wantGainRecovered(played)
		})
	}
}

// wantStoreReadsFiles fails unless the store says Sines' FLAC and MP3 carry
// ReplayGain exactly when their files do.
func (l *testLib) wantStoreReadsFiles() {
	l.t.Helper()
	tagged := l.tagged()
	for format, rel := range map[string]string{formatFLAC: sinesFLAC, formatMP3: sinesMP3} {
		if held := l.fileGain(rel); tagged[format] != (len(held) > 0) {
			l.t.Errorf("%s: the store says tagged %v, the file holds %v", format, tagged[format], held)
		}
	}
}

// wantGainRecovered fails unless nothing is planned, the run is stopped, the
// store reads what each file holds, Sines plays as it did, and a second run
// writes what the first didn't.
func (l *testLib) wantGainRecovered(played map[string]store.GainValues) {
	l.t.Helper()
	ctx := l.t.Context()
	if planned, err := l.st.PlannedGain(ctx); err != nil || len(planned) != 0 {
		l.t.Errorf("planned after recovery = %v, %v", planned, err)
	}
	runs, err := l.st.GainRuns(ctx, 10)
	if err != nil || len(runs) != 1 || runs[0].State != store.GainStopped {
		l.t.Fatalf("runs = %+v, %v; want one, stopped", runs, err)
	}
	l.wantStoreReadsFiles()
	if st := l.scan(Quick); st.Read != 0 {
		l.t.Errorf("a scan after recovery read %d files", st.Read)
	}
	if got := l.plays(); got[formatFLAC] != played[formatFLAC] || got[formatMP3] != played[formatMP3] {
		l.t.Errorf("plays %+v, want %+v", got, played)
	}
	if again := l.gainRun(); again.Written+runs[0].Written != 3 {
		l.t.Errorf("a second run wrote %d after %d, want 3 in all", again.Written, runs[0].Written)
	}
}

func TestGainRefusals(t *testing.T) {
	t.Parallel()
	t.Run("tag writes off", func(t *testing.T) {
		t.Parallel()
		l := gainLib(t, nil)
		l.opts.TagWrites = false
		l.scanner = NewScanner(l.st, l.opts)
		if _, _, err := l.scanner.StartGain(t.Context(), nil, 0); !errors.Is(err, ErrTagWritesOff) {
			t.Errorf("StartGain = %v, want ErrTagWritesOff", err)
		}
	})
	t.Run("busy", func(t *testing.T) {
		t.Parallel()
		l := gainLib(t, nil)
		_, fn, err := l.scanner.StartGain(t.Context(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = l.scanner.StartGain(t.Context(), nil, 0); !errors.Is(err, ErrBusy) {
			t.Errorf("a second run = %v, want ErrBusy", err)
		}
		if _, err = l.scanner.Claim(Quick); !errors.Is(err, ErrScanRunning) {
			t.Errorf("a scan during a run = %v, want ErrScanRunning", err)
		}
		fn(t.Context())
	})
	t.Run("root down", func(t *testing.T) {
		t.Parallel()
		l := gainLib(t, nil)
		for _, dir := range []string{sines, tones} {
			if err := os.RemoveAll(filepath.Join(l.roots[0], dir)); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := l.scanner.StartGain(t.Context(), nil, 0); !errors.Is(err, ErrRootDown) {
			t.Errorf("StartGain = %v, want ErrRootDown", err)
		}
	})
}

// openFiles says the files in it are open.
type openFiles struct {
	mu   sync.Mutex
	open map[string]bool
}

func (o *openFiles) Open(path string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.open[path]
}

// A file a stream holds open is rewritten atomically, so the stream reads on
// through the file it opened; one no stream has is written in place.
func TestGainWritesAStreamedFileAtomically(t *testing.T) {
	t.Parallel()
	l := gainLib(t, nil)
	root := l.roots[0]
	streamed := filepath.Join(root, sinesFLAC)
	l.opts.Streams = &openFiles{open: map[string]bool{streamed: true}}
	l.scanner = NewScanner(l.st, l.opts)
	modes := map[string]int{}
	var mu sync.Mutex
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error, opts ...audiometa.WriteOption) error {
		mu.Lock()
		modes[filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path)] = len(opts)
		mu.Unlock()
		return audiometa.WriteContext(ctx, path, edit, opts...)
	}
	stream, err := os.Open(streamed)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	original, err := os.ReadFile(streamed)
	if err != nil {
		t.Fatal(err)
	}

	l.gainRun()

	if modes[sinesFLAC] != 0 || modes[sinesMP3] != 1 || modes[tonesFLAC] != 1 {
		t.Errorf("write options by file = %v, want none (atomic) for the streamed FLAC, in place for the rest", modes)
	}
	read, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(read, original) {
		t.Error("the stream read a file that changed under it")
	}
	if now, _ := os.ReadFile(streamed); slices.Equal(now, original) {
		t.Error("the streamed file wasn't written")
	}
}

func TestGainValuesRoundAsTagsHoldThem(t *testing.T) {
	t.Parallel()
	got := gainValuesOf(&store.GainMeasure{Track: -20.004, TrackPeak: 0.98852549, Album: -17.996, AlbumPeak: 1.07, HasAlbum: true})
	want := store.GainValues{TrackGain: 2, TrackPeak: 0.988525, AlbumGain: 0, AlbumPeak: 1.07, HasAlbum: true}
	if !nearValues(got, want) {
		t.Errorf("values = %+v, want %+v", got, want)
	}
	if g := loudness.Gain(-18); g != 0 {
		t.Errorf("Gain(-18) = %v", g)
	}
}

func nearValues(a, b store.GainValues) bool {
	const eps = 1e-9
	d := func(x, y float64) bool { return x-y < eps && y-x < eps }
	return d(a.TrackGain, b.TrackGain) && d(a.TrackPeak, b.TrackPeak) && d(a.AlbumGain, b.AlbumGain) &&
		d(a.AlbumPeak, b.AlbumPeak) && a.HasAlbum == b.HasAlbum
}
