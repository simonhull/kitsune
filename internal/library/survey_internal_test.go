package library

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	testGroupKey = "album"
	fileMode     = 0o600
	dirMode      = 0o750
	sampleFLAC   = "sample.flac"
	sampleMP3    = "sample_v24.mp3"
	sampleM4A    = "sample.m4a"
	bystander    = "bystander.flac"
	sizeFLAC     = "size.flac"
	mtimeFLAC    = "mtime.flac"
	backFLAC     = "back.flac"
	issueChanged = "issue-changed.wav"
	issueSame    = "issue-same.wav"
	aFLAC        = "a.flac"
)

// fakeClock is a settable clock. The scanner reads it from its writer
// goroutine and tests move it between scans.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// testLib is a store, a clock and some library roots.
type testLib struct {
	t       *testing.T
	st      *store.Store
	clock   *fakeClock
	roots   []string
	scanner *Scanner
	opts    Options
	dbPath  string
}

const testGrace = 24 * time.Hour

// newTestLib returns a library with n empty roots.
func newTestLib(t *testing.T, n int) *testLib {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "kitsune.db")
	st, err := store.Open(t.Context(), dbPath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := st.Close(); cerr != nil {
			t.Error(cerr)
		}
	})
	l := &testLib{t: t, st: st, dbPath: dbPath, clock: &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}}
	for range n {
		l.roots = append(l.roots, t.TempDir())
	}
	l.use(l.roots...)
	return l
}

// use makes roots the configured roots, keeping the store and the clock.
func (l *testLib) use(roots ...string) {
	l.t.Helper()
	l.opts = Options{Roots: roots, MissingGrace: testGrace, Workers: 2, Now: l.clock.Now}
	l.scanner = NewScanner(l.st, l.opts)
}

// write creates root/rel with data, making its folders.
func (*testLib) write(t *testing.T, root, rel string, data []byte) string {
	t.Helper()
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // The write is done; nothing to report.
	if err = r.MkdirAll(filepath.Dir(rel), dirMode); err != nil {
		t.Fatal(err)
	}
	if err = r.WriteFile(rel, data, fileMode); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, rel)
}

// touch creates the empty file root/rel.
func (l *testLib) touch(root, rel string) string {
	l.t.Helper()
	return l.write(l.t, root, rel, nil)
}

// copyTo copies testdata/sample to root/rel.
func (l *testLib) copyTo(root, rel, sample string) {
	l.t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", sample))
	if err != nil {
		l.t.Fatal(err)
	}
	l.write(l.t, root, rel, b)
}

// trackAt returns a minimal valid track write.
func (l *testLib) trackAt(root, rel string, size int64, mtime time.Time) store.TrackWrite {
	l.t.Helper()
	id, err := newULID(l.clock.Now(), rand.Reader)
	if err != nil {
		l.t.Fatal(err)
	}
	return store.TrackWrite{
		ULID: id, Root: root, RelPath: rel, Size: size, MTime: mtime, Title: rel,
		Album: store.AlbumWrite{GroupKey: testGroupKey, Name: testGroupKey},
	}
}

// seed stores empty tracks at rels under root, each with an old mtime.
func (l *testLib) seed(root string, rels ...string) {
	l.t.Helper()
	var b store.Batch
	for _, rel := range rels {
		b.Tracks = append(b.Tracks, l.trackAt(root, rel, 0, time.UnixMilli(1)))
	}
	if err := l.st.WriteBatch(l.t.Context(), &b); err != nil {
		l.t.Fatal(err)
	}
}

// seedFile stores a track whose size and mtime are those of the file at root/rel.
func (l *testLib) seedFile(root, rel string) {
	l.t.Helper()
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		l.t.Fatal(err)
	}
	b := store.Batch{Tracks: []store.TrackWrite{l.trackAt(root, rel, info.Size(), info.ModTime())}}
	if err = l.st.WriteBatch(l.t.Context(), &b); err != nil {
		l.t.Fatal(err)
	}
}

// seedIssue stores a scan issue at root/rel with the given size and the file's mtime.
func (l *testLib) seedIssue(root, rel string, size int64, mtime time.Time) {
	l.t.Helper()
	b := store.Batch{Issues: []store.ScanIssue{{
		Root: root, RelPath: rel, Kind: store.IssueUnsupported, Message: "bad", Size: size, MTime: mtime,
	}}}
	if err := l.st.WriteBatch(l.t.Context(), &b); err != nil {
		l.t.Fatal(err)
	}
}

// id returns the track ID stored at root/rel, or fails.
func (l *testLib) id(root, rel string) int64 {
	l.t.Helper()
	st, err := l.st.FileStates(l.t.Context(), root)
	if err != nil {
		l.t.Fatal(err)
	}
	s, ok := st[rel]
	if !ok || s.TrackID == 0 {
		l.t.Fatalf("no track at %s/%s; states: %v", root, rel, slices.Sorted(maps.Keys(st)))
	}
	return s.TrackID
}

// states returns FileStates for root.
func (l *testLib) states(root string) map[string]store.FileState {
	l.t.Helper()
	st, err := l.st.FileStates(l.t.Context(), root)
	if err != nil {
		l.t.Fatal(err)
	}
	return st
}

// markMissing marks the tracks at rels under root missing as of at.
func (l *testLib) markMissing(root string, at time.Time, rels ...string) {
	l.t.Helper()
	ids := make([]int64, len(rels))
	for i, rel := range rels {
		ids[i] = l.id(root, rel)
	}
	if err := l.st.MarkMissing(l.t.Context(), ids, at); err != nil {
		l.t.Fatal(err)
	}
}

// survey runs the survey and fails the test on error.
func (l *testLib) survey(mode Mode) *surveyed {
	l.t.Helper()
	sv, err := l.scanner.survey(l.t.Context(), mode)
	if err != nil {
		l.t.Fatalf("survey: %v", err)
	}
	return sv
}

// jobPaths returns the sorted relative paths of the jobs.
func (l *testLib) jobPaths(sv *surveyed) []string {
	out := make([]string, 0, len(sv.Jobs))
	for i := range sv.Jobs {
		out = append(out, sv.Jobs[i].RelPath)
	}
	slices.Sort(out)
	return out
}

// skipIfRoot skips tests that rely on permissions root ignores.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
}

// denyDir removes all access to dir until the test ends.
func denyDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, dirMode); err != nil {
			t.Error(err)
		}
	})
}

func wantPaths(t *testing.T, got, want []string) {
	t.Helper()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}

func TestSurveyFindsAudio(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	audio := []string{
		aFLAC, "B.MP3", "c.M4A", "d.m4b", "e.ogg", "f.oga", "g.Opus", "h.wav", "i.aif", "j.AIFF",
		"deep/er/k.flac", "@music.flac", "x.flac.mp3",
	}
	other := []string{
		coverJPG, ".hidden.flac", "._x.flac", "@eaDir/x.flac", ".cache/x.flac", "notes.txt", "m.mp4", "n.m4p",
		"sub/@eaDir/y.flac", "flac", "sub/.hidden/z.flac",
	}
	for _, rel := range slices.Concat(audio, other) {
		l.touch(root, rel)
	}
	sv := l.survey(Full)
	wantPaths(t, l.jobPaths(sv), audio)
	if sv.Seen != len(audio) {
		t.Errorf("Seen = %d, want %d", sv.Seen, len(audio))
	}
	for i := range sv.Jobs {
		if sv.Jobs[i].Root != root {
			t.Errorf("job root = %q, want %q", sv.Jobs[i].Root, root)
		}
	}
}

func TestSurveySymlinks(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	outside := t.TempDir()
	target := l.touch(outside, "target.flac")
	l.touch(outside, "in-linked-dir/x.flac")
	if err := os.Symlink(target, filepath.Join(root, "link.flac")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "dangling.flac")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.flac"), fileMode); err != nil {
		t.Fatal(err)
	}
	l.touch(root, "x.flac/inner.mp3")
	sv := l.survey(Full)
	wantPaths(t, l.jobPaths(sv), []string{"link.flac", "x.flac/inner.mp3"})
}

func TestSurveyDanglingSymlinkKeepsTrack(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.touch(root, bystander)
	l.seed(root, "dangling.flac")
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "dangling.flac")); err != nil {
		t.Fatal(err)
	}
	sv := l.survey(Quick)
	if len(sv.Mark) != 0 {
		t.Errorf("Mark = %v, want none: a dangling link is an unreadable entry", sv.Mark)
	}
}

func TestSurveySymlinkRoot(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	target := l.roots[0]
	l.touch(target, "a/x.flac")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	l.use(link)
	sv := l.survey(Quick)
	if len(sv.Jobs) != 1 || sv.Jobs[0].Root != link || sv.Jobs[0].RelPath != filepath.Join("a", "x.flac") {
		t.Errorf("Jobs = %+v, want a/x.flac under %s", sv.Jobs, link)
	}
}

func TestSurveyRootNameDoesNotMatter(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 0)
	root := filepath.Join(t.TempDir(), "@music")
	if err := os.Mkdir(root, dirMode); err != nil {
		t.Fatal(err)
	}
	l.touch(root, "x.flac")
	l.use(root)
	if sv := l.survey(Quick); len(sv.Jobs) != 1 {
		t.Errorf("Jobs = %+v, want 1", sv.Jobs)
	}
}

func TestSurveyCancelled(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	for i := range 5 {
		l.touch(l.roots[0], fmt.Sprintf("%d.flac", i))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sv, err := l.scanner.survey(ctx, Quick)
	if !errors.Is(err, context.Canceled) || sv != nil {
		t.Errorf("survey = %+v, %v; want nil and context.Canceled", sv, err)
	}
}

// seedDiffFixture stores a library whose files differ from the store in the
// ways a diff must tell apart.
func (l *testLib) seedDiffFixture(root string) {
	l.t.Helper()
	for _, rel := range []string{"unchanged.flac", sizeFLAC, mtimeFLAC, backFLAC} {
		l.copyTo(root, rel, sampleFLAC)
		l.seedFile(root, rel)
	}
	// Rewrite the stored size and mtime, so those files look changed.
	b := store.Batch{}
	for _, rel := range []string{sizeFLAC, mtimeFLAC} {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			l.t.Fatal(err)
		}
		tw := l.trackAt(root, rel, info.Size(), info.ModTime())
		tw.ID = l.id(root, rel)
		if rel == sizeFLAC {
			tw.Size++
		} else {
			tw.MTime = tw.MTime.Add(-time.Hour)
		}
		b.Tracks = append(b.Tracks, tw)
	}
	if err := l.st.WriteBatch(l.t.Context(), &b); err != nil {
		l.t.Fatal(err)
	}
	l.markMissing(root, l.clock.Now(), backFLAC)
	for _, rel := range []string{issueSame, issueChanged} {
		l.touch(root, rel)
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			l.t.Fatal(err)
		}
		size := info.Size()
		if rel == issueChanged {
			size++
		}
		l.seedIssue(root, rel, size, info.ModTime())
	}
}

func TestSurveyDiff(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.seedDiffFixture(root)

	quick := l.survey(Quick)
	wantPaths(t, l.jobPaths(quick), []string{sizeFLAC, mtimeFLAC, backFLAC, issueChanged})
	for i := range quick.Jobs {
		j := &quick.Jobs[i]
		if wantID := l.states(root)[j.RelPath].TrackID; j.TrackID != wantID {
			t.Errorf("%s: TrackID = %d, want %d", j.RelPath, j.TrackID, wantID)
		}
		if j.Missing != (j.RelPath == backFLAC) {
			t.Errorf("%s: Missing = %t", j.RelPath, j.Missing)
		}
	}
	full := l.survey(Full)
	wantPaths(t, l.jobPaths(full), []string{"unchanged.flac", sizeFLAC, mtimeFLAC, backFLAC, issueSame, issueChanged})
}

func TestSurveyAbsent(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.touch(root, bystander)
	l.seed(root, "gone.flac", "already.flac", "stays.flac")
	l.touch(root, "stays.flac")
	l.markMissing(root, l.clock.Now(), "already.flac")
	l.seedIssue(root, "gone.wav", 5, time.UnixMilli(1))
	sv := l.survey(Quick)
	if want := []int64{l.id(root, "gone.flac")}; !slices.Equal(sv.Mark, want) {
		t.Errorf("Mark = %v, want %v", sv.Mark, want)
	}
	if got := sv.IssueDeletes[root]; !slices.Equal(got, []string{"gone.wav"}) {
		t.Errorf("IssueDeletes = %v, want gone.wav", sv.IssueDeletes)
	}
}

func TestSurveyMissingRootIsSkipped(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 0)
	gone := filepath.Join(t.TempDir(), "unmounted")
	l.seed(gone, aFLAC, "b.flac")
	l.use(gone)
	sv := l.survey(Quick)
	if sv.SkippedRoots != 1 || len(sv.Mark) != 0 || len(sv.Jobs) != 0 || len(sv.IssueDeletes) != 0 {
		t.Errorf("survey = %+v, want one skipped root and nothing else", sv)
	}
}

// A root that can't be read is an outage even with nothing stored under it.
func TestSurveyMissingRootWithNothingStoredIsSkipped(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 0)
	l.use(filepath.Join(t.TempDir(), "unmounted"))
	if sv := l.survey(Quick); sv.SkippedRoots != 1 {
		t.Errorf("SkippedRoots = %d, want 1", sv.SkippedRoots)
	}
}

func TestSurveyEmptyRootIsSkipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		missing     bool
		stored      bool
		wantSkipped int
	}{
		{"live tracks", false, true, 1},
		{"missing tracks", true, true, 1},
		{"nothing stored", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			if tt.stored {
				l.seed(root, aFLAC)
				if tt.missing {
					l.markMissing(root, l.clock.Now(), aFLAC)
				}
			}
			sv := l.survey(Quick)
			if sv.SkippedRoots != tt.wantSkipped || len(sv.Mark) != 0 {
				t.Errorf("SkippedRoots = %d, Mark = %v; want %d and none", sv.SkippedRoots, sv.Mark, tt.wantSkipped)
			}
		})
	}
}

func TestSurveyMassDisappearance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		stored, kept, newOne int
		wantSkipped          bool
	}{
		{"two thirds vanish", 90, 30, 0, true},
		{"vanished files reappear elsewhere", 90, 30, 60, false},
		{"under the floor", 49, 0, 0, false},
		{"a quarter vanishes above the floor", 200, 150, 0, false},
		{"all of 40 vanish", 40, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			rels := make([]string, tt.stored)
			for i := range rels {
				rels[i] = fmt.Sprintf("old/%03d.flac", i)
			}
			l.seed(root, rels...)
			for _, rel := range rels[:tt.kept] {
				l.touch(root, rel)
			}
			for i := range tt.newOne {
				l.touch(root, fmt.Sprintf("new/%03d.flac", i))
			}
			l.touch(root, bystander)
			checkMass(t, l.survey(Quick), tt.wantSkipped, tt.stored-tt.kept)
		})
	}
}

// checkMass checks that a root was skipped or that wantMark tracks were marked.
func checkMass(t *testing.T, sv *surveyed, wantSkipped bool, wantMark int) {
	t.Helper()
	switch {
	case (sv.SkippedRoots == 1) != wantSkipped:
		t.Errorf("SkippedRoots = %d, want skipped = %t", sv.SkippedRoots, wantSkipped)
	case wantSkipped && (len(sv.Mark) != 0 || len(sv.Jobs) != 0):
		t.Errorf("a skipped root left Mark = %d, Jobs = %d", len(sv.Mark), len(sv.Jobs))
	case !wantSkipped && len(sv.Mark) != wantMark:
		t.Errorf("Mark = %d, want %d", len(sv.Mark), wantMark)
	}
}

func TestSurveyHoldsDeletes(t *testing.T) {
	t.Parallel()
	t.Run("missing track under a skipped root", func(t *testing.T) {
		t.Parallel()
		l := newTestLib(t, 0)
		gone := filepath.Join(t.TempDir(), "unmounted")
		l.seed(gone, aFLAC)
		l.markMissing(gone, l.clock.Now(), aFLAC)
		l.use(gone)
		if sv := l.survey(Quick); !sv.HoldDeletes {
			t.Error("HoldDeletes = false, want true")
		}
	})
	t.Run("live track under a skipped root", func(t *testing.T) {
		t.Parallel()
		l := newTestLib(t, 0)
		gone := filepath.Join(t.TempDir(), "unmounted")
		l.seed(gone, aFLAC)
		l.use(gone)
		if sv := l.survey(Quick); sv.HoldDeletes || sv.SkippedRoots != 1 {
			t.Errorf("HoldDeletes = %t, SkippedRoots = %d; want false and 1", sv.HoldDeletes, sv.SkippedRoots)
		}
	})
	t.Run("missing track under an unreadable folder", func(t *testing.T) {
		t.Parallel()
		skipIfRoot(t)
		l := newTestLib(t, 1)
		root := l.roots[0]
		l.touch(root, bystander)
		l.touch(root, "sub/x.flac")
		l.seed(root, "sub/x.flac")
		l.markMissing(root, l.clock.Now(), "sub/x.flac")
		denyDir(t, filepath.Join(root, "sub"))
		if sv := l.survey(Quick); !sv.HoldDeletes {
			t.Error("HoldDeletes = false, want true")
		}
	})
	t.Run("missing track at an unreadable entry", func(t *testing.T) {
		t.Parallel()
		l := newTestLib(t, 1)
		root := l.roots[0]
		l.touch(root, bystander)
		l.seed(root, "dangling.flac")
		l.markMissing(root, l.clock.Now(), "dangling.flac")
		if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "dangling.flac")); err != nil {
			t.Fatal(err)
		}
		if sv := l.survey(Quick); !sv.HoldDeletes {
			t.Error("HoldDeletes = false, want true")
		}
	})
	t.Run("unreadable folder without tracks", func(t *testing.T) {
		t.Parallel()
		skipIfRoot(t)
		l := newTestLib(t, 1)
		root := l.roots[0]
		l.touch(root, bystander)
		l.seed(root, "gone.flac")
		l.markMissing(root, l.clock.Now(), "gone.flac")
		if err := os.Mkdir(filepath.Join(root, "lost+found"), dirMode); err != nil {
			t.Fatal(err)
		}
		denyDir(t, filepath.Join(root, "lost+found"))
		if sv := l.survey(Quick); sv.HoldDeletes {
			t.Error("HoldDeletes = true, want false: lost+found must not block deletion forever")
		}
	})
}

func TestSurveyUnreadableSubdirKeepsTracks(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.touch(root, bystander)
	l.touch(root, "sub/a.flac")
	l.seed(root, "sub/a.flac", "sub/deeper/b.flac", "gone.flac")
	denyDir(t, filepath.Join(root, "sub"))
	sv := l.survey(Quick)
	if want := []int64{l.id(root, "gone.flac")}; !slices.Equal(sv.Mark, want) || sv.SkippedRoots != 0 {
		t.Errorf("Mark = %v, SkippedRoots = %d; want %v and 0", sv.Mark, sv.SkippedRoots, want)
	}
}

func TestSurveyNoRootsTouchesNothing(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 0)
	root := t.TempDir()
	l.seed(root, aFLAC, "b.flac")
	l.markMissing(root, l.clock.Now().Add(-10*testGrace), "b.flac")
	l.use()
	stats, err := l.scanner.Scan(t.Context(), Quick)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stats, Stats{}) {
		t.Errorf("stats = %+v, want zero", stats)
	}
	st := l.states(root)
	if len(st) != 2 || st[aFLAC].Missing || !st["b.flac"].Missing {
		t.Errorf("states = %+v, want both tracks untouched", st)
	}
}

func TestSurveyRemovedRoot(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 2)
	kept, removed := l.roots[0], l.roots[1]
	l.copyTo(kept, "k.flac", sampleFLAC)
	l.seedFile(kept, "k.flac")
	l.seed(removed, "r1.flac", "r2.flac", "r3.flac")
	l.markMissing(removed, l.clock.Now(), "r3.flac")
	l.seedIssue(removed, "bad.wav", 1, time.UnixMilli(1))
	l.use(kept)
	sv := l.survey(Quick)
	want := []int64{l.id(removed, "r1.flac"), l.id(removed, "r2.flac")}
	slices.Sort(want)
	slices.Sort(sv.Mark)
	if !slices.Equal(sv.Mark, want) {
		t.Errorf("Mark = %v, want %v", sv.Mark, want)
	}
	if got := sv.IssueDeletes[removed]; !slices.Equal(got, []string{"bad.wav"}) {
		t.Errorf("IssueDeletes = %v, want bad.wav under the removed root", sv.IssueDeletes)
	}
	if _, ok := sv.IssueDeletes[kept]; ok || len(sv.Jobs) != 0 {
		t.Errorf("the configured root changed: %+v", sv)
	}
}

func TestSurveyConfigTypoKeepsLibrary(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing folder", "empty folder"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			old := l.roots[0]
			l.seed(old, aFLAC, "b.flac")
			next := filepath.Join(t.TempDir(), "typo")
			if name == "empty folder" {
				if err := os.Mkdir(next, dirMode); err != nil {
					t.Fatal(err)
				}
			}
			l.use(next)
			sv := l.survey(Quick)
			if len(sv.Mark) != 0 || len(sv.IssueDeletes) != 0 {
				t.Errorf("survey = %+v, want nothing under the old root touched", sv)
			}
		})
	}
}
