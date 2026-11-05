package library

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	keepMP3   = "keep.mp3"
	xFLAC     = "x.flac"
	otherName = "Other"
)

// scan runs a scan and fails the test on error.
func (l *testLib) scan(mode Mode) *Stats {
	l.t.Helper()
	stats, err := l.scanner.Scan(l.t.Context(), mode)
	if err != nil {
		l.t.Fatalf("Scan: %v", err)
	}
	return &stats
}

// albumDetail returns the library's only album.
func (l *testLib) albumDetail() store.AlbumDetail {
	l.t.Helper()
	page, err := l.st.Albums(l.t.Context(), store.SortAdded, 2, "")
	if err != nil {
		l.t.Fatal(err)
	}
	if len(page.Albums) != 1 {
		l.t.Fatalf("albums = %+v, want 1", page.Albums)
	}
	d, err := l.st.Album(l.t.Context(), page.Albums[0].ID)
	if err != nil {
		l.t.Fatal(err)
	}
	return d
}

// bump moves the file's mtime forward by n minutes from a fixed point, so a
// rewrite is always a change at millisecond precision.
func bump(t *testing.T, path string, n int) {
	t.Helper()
	at := time.Date(2030, 1, 1, 0, n, 0, 0, time.UTC)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// retag changes the album and artist of the file at path, keeping its audio.
func retag(t *testing.T, path, name string) {
	t.Helper()
	err := audiometa.Write(path, func(f *audiometa.File) error {
		f.Tags.Album = name
		f.Tags.Artists = []string{name}
		f.Tags.AlbumArtist = name
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func garbage() []byte { return bytes.Repeat([]byte("garbage!"), 512) }

func wantStats(t *testing.T, got, want *Stats) {
	t.Helper()
	if !reflect.DeepEqual(*got, *want) {
		t.Errorf("stats = %+v, want %+v", *got, *want)
	}
}

// remove deletes root/rel.
func remove(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
}

func TestScanAddsTracks(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.copyTo(root, "a/two.mp3", sampleMP3)
	l.copyTo(root, "b/three.m4a", sampleM4A)
	stats := l.scan(Quick)
	wantStats(t, stats, &Stats{Seen: 3, Read: 3, Added: 3})
	if st := l.states(root); len(st) != 3 {
		t.Errorf("states = %+v, want 3 tracks", st)
	}
}

func TestQuickScanReadsNothingWhenUnchanged(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "one.flac", sampleFLAC)
	l.copyTo(root, "two.mp3", sampleMP3)
	l.copyTo(root, "three.m4a", sampleM4A)
	l.scan(Quick)
	wantStats(t, l.scan(Quick), &Stats{Seen: 3})
	wantStats(t, l.scan(Full), &Stats{Seen: 3, Read: 3, Updated: 3})
}

func TestScanChangedFileKeepsID(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.scan(Quick)
	id := l.id(root, "one.flac")
	path := filepath.Join(root, "one.flac")
	retag(t, path, "Changed")
	bump(t, path, 1)
	wantStats(t, l.scan(Quick), &Stats{Seen: 2, Read: 1, Updated: 1})
	if got := l.id(root, "one.flac"); got != id {
		t.Errorf("ID = %d, want %d", got, id)
	}
}

func TestScanMoveKeepsID(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "old/one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.scan(Quick)
	id := l.id(root, "old/one.flac")
	if err := os.MkdirAll(filepath.Join(root, "new"), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "old", "one.flac"), filepath.Join(root, "new", "one.flac")); err != nil {
		t.Fatal(err)
	}
	stats := l.scan(Quick)
	if stats.Moved != 1 || stats.Added != 0 || stats.Read != 1 {
		t.Errorf("stats = %+v, want Moved 1, Added 0, Read 1", stats)
	}
	if got := l.id(root, "new/one.flac"); got != id {
		t.Errorf("ID at new path = %d, want %d", got, id)
	}
	if _, ok := l.states(root)["old/one.flac"]; ok {
		t.Error("the old path still has a state")
	}
}

func TestScanMoveBetweenRoots(t *testing.T) {
	t.Parallel()
	for _, from := range []int{0, 1} {
		t.Run(fmt.Sprintf("from root %d", from), func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 2)
			l.copyTo(l.roots[0], keepMP3, sampleMP3)
			l.copyTo(l.roots[1], "keep.m4a", sampleM4A)
			l.copyTo(l.roots[from], xFLAC, sampleFLAC)
			l.scan(Quick)
			id := l.id(l.roots[from], xFLAC)
			to := 1 - from
			if err := os.Rename(filepath.Join(l.roots[from], xFLAC), filepath.Join(l.roots[to], xFLAC)); err != nil {
				t.Fatal(err)
			}
			stats := l.scan(Quick)
			if stats.Moved != 1 || stats.Added != 0 {
				t.Errorf("stats = %+v, want Moved 1, Added 0", stats)
			}
			if got := l.id(l.roots[to], xFLAC); got != id {
				t.Errorf("ID in the new root = %d, want %d", got, id)
			}
		})
	}
}

func TestScanMoveTieBreaksOnBaseName(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, "a/x.flac", sampleFLAC)
	l.copyTo(root, "b/y.flac", sampleFLAC)
	l.scan(Quick)
	idY := l.id(root, "b/y.flac")
	remove(t, root, "a/x.flac")
	remove(t, root, "b/y.flac")
	l.copyTo(root, "c/y.flac", sampleFLAC)
	stats := l.scan(Quick)
	if stats.Moved != 1 || stats.Added != 0 {
		t.Errorf("stats = %+v, want Moved 1, Added 0", stats)
	}
	if got := l.id(root, "c/y.flac"); got != idY {
		t.Errorf("c/y.flac took ID %d, want %d (the one with the same base name)", got, idY)
	}
}

func TestScanAmbiguousMoveIsNew(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"c/x.flac", "c/z.flac"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			l.copyTo(root, keepMP3, sampleMP3)
			l.copyTo(root, "a/x.flac", sampleFLAC)
			l.copyTo(root, "b/x.flac", sampleFLAC)
			l.scan(Quick)
			old := []int64{l.id(root, "a/x.flac"), l.id(root, "b/x.flac")}
			remove(t, root, "a/x.flac")
			remove(t, root, "b/x.flac")
			l.copyTo(root, name, sampleFLAC)
			stats := l.scan(Quick)
			if stats.Moved != 0 || stats.Added != 1 {
				t.Errorf("stats = %+v, want Moved 0, Added 1", stats)
			}
			if got := l.id(root, name); slices.Contains(old, got) {
				t.Errorf("%s took the ID %d of one of two candidates", name, got)
			}
		})
	}
}

func TestScanRootRenameKeepsDuplicateIDs(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 2)
	first, second := l.roots[0], l.roots[1]
	populate := func(root string) {
		l.copyTo(root, "a/x.flac", sampleFLAC)
		l.copyTo(root, "b/x.flac", sampleFLAC)
		l.copyTo(root, keepMP3, sampleMP3)
	}
	populate(first)
	populate(second)
	l.use(first)
	l.scan(Quick)
	idA, idB := l.id(first, "a/x.flac"), l.id(first, "b/x.flac")
	l.use(second)
	stats := l.scan(Quick)
	if stats.Moved != 3 || stats.Added != 0 {
		t.Errorf("stats = %+v, want Moved 3, Added 0", stats)
	}
	if a, b := l.id(second, "a/x.flac"), l.id(second, "b/x.flac"); a != idA || b != idB {
		t.Errorf("IDs = %d, %d, want %d, %d", a, b, idA, idB)
	}
}

func TestScanTwoNewFilesOneMissing(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, "old.flac", sampleFLAC)
	l.scan(Quick)
	id := l.id(root, "old.flac")
	remove(t, root, "old.flac")
	l.copyTo(root, "n1.flac", sampleFLAC)
	l.copyTo(root, "n2.flac", sampleFLAC)
	stats := l.scan(Quick)
	if stats.Moved != 1 || stats.Added != 1 {
		t.Errorf("stats = %+v, want Moved 1, Added 1", stats)
	}
	got1, got2 := l.id(root, "n1.flac"), l.id(root, "n2.flac")
	if (got1 == id) == (got2 == id) {
		t.Errorf("IDs = %d, %d: exactly one should be %d", got1, got2, id)
	}
}

func TestScanReappearanceKeepsIDOverCopy(t *testing.T) {
	t.Parallel()
	for _, names := range [][2]string{{aFLAC, "b.flac"}, {"b.flac", aFLAC}} {
		original, dup := names[0], names[1]
		t.Run(original, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			l.copyTo(root, keepMP3, sampleMP3)
			l.copyTo(root, original, sampleFLAC)
			l.scan(Quick)
			id := l.id(root, original)
			remove(t, root, original)
			wantStats(t, l.scan(Quick), &Stats{Seen: 1, Missing: 1})
			l.copyTo(root, original, sampleFLAC)
			l.copyTo(root, dup, sampleFLAC)
			stats := l.scan(Quick)
			if stats.Updated != 1 || stats.Added != 1 || stats.Moved != 0 {
				t.Errorf("stats = %+v, want Updated 1, Added 1, Moved 0", stats)
			}
			if got := l.id(root, original); got != id {
				t.Errorf("%s has ID %d, want its own %d", original, got, id)
			}
			if got := l.id(root, dup); got == id {
				t.Errorf("%s took the ID of %s", dup, original)
			}
		})
	}
}

func TestScanIdenticalFilesStayTwoTracks(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, aFLAC, sampleFLAC)
	l.copyTo(root, "b.flac", sampleFLAC)
	wantStats(t, l.scan(Quick), &Stats{Seen: 2, Read: 2, Added: 2})
	if l.id(root, aFLAC) == l.id(root, "b.flac") {
		t.Error("two identical files share a track")
	}
}

func TestScanVanishedThenGraceDeletes(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	retag(t, filepath.Join(root, keepMP3), otherName)
	l.copyTo(root, "gone.flac", sampleFLAC)
	l.scan(Quick)
	remove(t, root, "gone.flac")
	wantStats(t, l.scan(Quick), &Stats{Seen: 1, Missing: 1})
	l.clock.Advance(testGrace + time.Minute)
	stats := l.scan(Quick)
	wantStats(t, stats, &Stats{Seen: 1, Deleted: 1, Pruned: store.Pruned{Albums: 1, Artists: 1}})
	if _, ok := l.states(root)["gone.flac"]; ok {
		t.Error("the track survived the grace period")
	}
}

func TestScanBrokenFileIsAnIssue(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "good.flac", sampleFLAC)
	bad := l.write(t, root, "bad.flac", garbage())
	stats := l.scan(Quick)
	if stats.Issues != 1 || stats.Added != 1 || stats.Read != 2 {
		t.Errorf("stats = %+v, want Issues 1, Added 1, Read 2", stats)
	}
	if st := l.states(root)["bad.flac"]; !st.Issue || st.TrackID != 0 {
		t.Errorf("state of bad.flac = %+v, want an issue", st)
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
	b, err := os.ReadFile(filepath.Join("testdata", sampleFLAC))
	if err != nil {
		t.Fatal(err)
	}
	l.write(t, root, "bad.flac", b)
	bump(t, bad, 1)
	wantStats(t, l.scan(Quick), &Stats{Seen: 2, Read: 1, Added: 1})
	if st := l.states(root)["bad.flac"]; st.Issue || st.TrackID == 0 {
		t.Errorf("state of bad.flac = %+v, want a track and no issue", st)
	}
}

func TestScanUnreadableFileIsRetried(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, xFLAC, sampleFLAC)
	path := filepath.Join(root, xFLAC)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Issues != 1 || stats.Added != 1 {
		t.Errorf("stats = %+v, want Issues 1, Added 1", stats)
	}
	if stats := l.scan(Quick); stats.Issues != 1 || stats.Read != 1 {
		t.Errorf("a second scan = %+v, want the file retried", stats)
	}
	if err := os.Chmod(path, fileMode); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Added != 1 || stats.Issues != 0 {
		t.Errorf("after the fix = %+v, want Added 1", stats)
	}
}

func TestScanUnreadableMissingTrackIsNotDeleted(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, xFLAC, sampleFLAC)
	l.scan(Quick)
	id := l.id(root, xFLAC)
	remove(t, root, xFLAC)
	l.scan(Quick)
	l.clock.Advance(testGrace + time.Minute)
	l.copyTo(root, xFLAC, sampleFLAC)
	if err := os.Chmod(filepath.Join(root, xFLAC), 0); err != nil {
		t.Fatal(err)
	}
	stats := l.scan(Quick)
	if stats.Deleted != 0 || stats.Issues != 1 {
		t.Errorf("stats = %+v, want Deleted 0, Issues 1", stats)
	}
	if got := l.id(root, xFLAC); got != id {
		t.Errorf("the track at x.flac is %d, want %d", got, id)
	}
}

func TestScanNonUTF8NameWithWarnings(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	b, err := os.ReadFile(filepath.Join("testdata", sampleMP3))
	if err != nil {
		t.Fatal(err)
	}
	// Strip the ID3v2 tag: a header of 10 bytes and a syncsafe size.
	const header = 10
	size := int(b[6])<<21 | int(b[7])<<14 | int(b[8])<<7 | int(b[9])
	l.write(t, root, "caf\xe9.mp3", b[header+size:])
	stats := l.scan(Quick)
	wantStats(t, stats, &Stats{Seen: 1, Read: 1, Added: 1})
	if st := l.states(root)["caf\xe9.mp3"]; st.TrackID == 0 {
		t.Errorf("state = %+v, want a track", st)
	}
}

func TestScanGoodFileGoneBadKeepsTrack(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, xFLAC, sampleFLAC)
	l.scan(Quick)
	id := l.id(root, xFLAC)
	path := l.write(t, root, xFLAC, garbage())
	bump(t, path, 1)
	stats := l.scan(Quick)
	if stats.Issues != 1 || stats.Missing != 0 {
		t.Errorf("stats = %+v, want Issues 1, Missing 0", stats)
	}
	st := l.states(root)[xFLAC]
	if st.TrackID != id || st.Missing || !st.Issue {
		t.Errorf("state = %+v, want track %d, not missing, with an issue", st, id)
	}
}

func TestScanBatches(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	for i := range 7 {
		l.copyTo(root, fmt.Sprintf("%d.flac", i), sampleFLAC)
	}
	l.scanner.batchSize = 3
	wantStats(t, l.scan(Quick), &Stats{Seen: 7, Read: 7, Added: 7})
	if st := l.states(root); len(st) != 7 {
		t.Errorf("states = %d, want 7", len(st))
	}
}

func TestScanCancelled(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	const files, cancelAt = 40, 12
	for i := range files {
		l.copyTo(root, fmt.Sprintf("%02d.flac", i), sampleFLAC)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls atomic.Int32
	l.opts.Now = func() time.Time {
		if calls.Add(1) == cancelAt {
			cancel()
		}
		return l.clock.Now()
	}
	l.scanner = NewScanner(l.st, l.opts)
	l.scanner.batchSize = 5
	stats, err := l.scanner.Scan(ctx, Quick)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan = %v, want context.Canceled", err)
	}
	st := l.states(root)
	if len(st) != stats.Added || stats.Added == 0 || stats.Added >= files {
		t.Errorf("stored %d of %d, Added = %d: want every file read stored, and not all", len(st), files, stats.Added)
	}
	for rel, s := range st {
		if s.Issue {
			t.Errorf("%s became an issue", rel)
		}
	}
	l.use(l.roots...)
	if after := l.scan(Quick); after.Added != files-stats.Added {
		t.Errorf("a later scan added %d, want %d", after.Added, files-stats.Added)
	}
}

func TestScanRunningIsRefused(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	run, err := l.scanner.Claim(Full)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.scanner.Claim(Quick); !errors.Is(err, ErrScanRunning) {
		t.Errorf("Claim = %v, want ErrScanRunning", err)
	}
	if _, err = l.scanner.Scan(t.Context(), Quick); !errors.Is(err, ErrScanRunning) {
		t.Errorf("Scan = %v, want ErrScanRunning", err)
	}
	if _, err = run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = l.scanner.Scan(t.Context(), Quick); err != nil {
		t.Errorf("Scan after the claimed scan = %v, want nil", err)
	}
}

func TestClaimedScanReadsWhatItsModeAsks(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.copyTo(l.roots[0], xFLAC, sampleFLAC)
	l.scan(Quick)
	run, err := l.scanner.Claim(Full)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Read != 1 {
		t.Errorf("Read = %d, want 1: a Full scan reads every file", stats.Read)
	}
}

// retitled returns the titles in after that before's tracks, by ID, did not have.
func retitled(before, after *store.AlbumDetail) []string {
	titles := make(map[string]string, len(before.Tracks))
	for i := range before.Tracks {
		titles[before.Tracks[i].ID] = before.Tracks[i].Title
	}
	var changed []string
	for i := range after.Tracks {
		if titles[after.Tracks[i].ID] != after.Tracks[i].Title {
			changed = append(changed, after.Tracks[i].Title)
		}
	}
	return changed
}

func TestReindexOneFile(t *testing.T) {
	t.Parallel()
	const newTitle = "New Title"
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, aFLAC, sampleFLAC)
	l.copyTo(root, bystander, sampleFLAC)
	l.scan(Quick)
	before := l.albumDetail()
	err := audiometa.Write(filepath.Join(root, aFLAC), func(f *audiometa.File) error {
		f.Tags.Title = newTitle
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = l.scanner.reindex(t.Context(), l.states(root)[aFLAC].TrackID, root, aFLAC); err != nil {
		t.Fatal(err)
	}
	after := l.albumDetail()
	if after.ID != before.ID || !after.AddedAt.Equal(before.AddedAt) {
		t.Errorf("album %s added %v, want %s added %v", after.ID, after.AddedAt, before.ID, before.AddedAt)
	}
	if got := retitled(&before, &after); !slices.Equal(got, []string{newTitle}) {
		t.Errorf("titles that changed = %q, want only %q", got, newTitle)
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2}) // The store already matches the disk.
}

func TestReindexMissingFile(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, aFLAC, sampleFLAC)
	l.scan(Quick)
	id := l.states(root)[aFLAC].TrackID
	remove(t, root, aFLAC)
	if err := l.scanner.reindex(t.Context(), id, root, aFLAC); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("reindex = %v, want fs.ErrNotExist", err)
	}
}

func TestEditDuringScanIsRefused(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.scanner.lock <- struct{}{} // What a running scan holds.
	defer l.scanner.lock.unlock()
	err := l.scanner.exclusive(func() error {
		t.Error("the edit ran during a scan")
		return nil
	})
	if !errors.Is(err, ErrBusy) {
		t.Errorf("exclusive = %v, want ErrBusy", err)
	}
}

// TestScanHoldsTheLockUntilItEnds stops a scan just after its walk and checks
// an edit is refused until the scan is done.
func TestScanHoldsTheLockUntilItEnds(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.copyTo(l.roots[0], "one.flac", sampleFLAC)
	walked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	l.scanner.now = func() time.Time {
		once.Do(func() {
			close(walked)
			<-release
		})
		return l.clock.Now()
	}
	done := make(chan error, 1)
	go func() {
		_, err := l.scanner.Scan(t.Context(), Quick)
		done <- err
	}()
	<-walked
	if err := l.scanner.exclusive(func() error { return nil }); !errors.Is(err, ErrBusy) {
		t.Errorf("exclusive during a scan = %v, want ErrBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := l.scanner.exclusive(func() error { return nil }); err != nil {
		t.Errorf("exclusive after the scan = %v", err)
	}
}

func TestScanWaitsForAnEdit(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.copyTo(l.roots[0], "one.flac", sampleFLAC)
	started, scanned := make(chan struct{}), make(chan error, 1)
	err := l.scanner.exclusive(func() error {
		go func() {
			close(started)
			_, scanErr := l.scanner.Scan(t.Context(), Quick)
			scanned <- scanErr
		}()
		<-started
		select {
		case scanErr := <-scanned:
			t.Errorf("Scan returned during the edit: %v", scanErr)
		case <-time.After(50 * time.Millisecond):
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-scanned; err != nil {
		t.Errorf("Scan after the edit = %v", err)
	}
	if st := l.states(l.roots[0]); len(st) != 1 {
		t.Errorf("states = %+v, want the file scanned", st)
	}
}

func TestScanWaitingForAnEditStopsWithItsContext(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := l.scanner.exclusive(func() error {
		_, scanErr := l.scanner.Scan(ctx, Quick)
		return scanErr
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Scan = %v, want context.Canceled", err)
	}
}

func TestSweepRemovesLeftoverTemps(t *testing.T) {
	t.Parallel()
	const (
		temp   = "a/.one.flac.audiometa-123.tmp"
		hidden = "a/.keep.tmp"
	)
	tests := []struct {
		name      string
		tagWrites bool
		wantGone  bool
		wantSwept int
	}{
		{"writes on", true, true, 1},
		{"writes off", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			l.scanner.tagWrites = tt.tagWrites
			root := l.roots[0]
			l.copyTo(root, "a/one.flac", sampleFLAC)
			l.copyTo(root, temp, sampleFLAC)
			l.copyTo(root, hidden, sampleFLAC)
			wantStats(t, l.scan(Quick), &Stats{Seen: 1, Read: 1, Added: 1, Swept: tt.wantSwept})
			_, err := os.Stat(filepath.Join(root, temp))
			if gone := errors.Is(err, os.ErrNotExist); gone != tt.wantGone {
				t.Errorf("temp gone = %t (%v), want %t", gone, err, tt.wantGone)
			}
			if _, err = os.Stat(filepath.Join(root, hidden)); err != nil {
				t.Errorf("another hidden file: %v", err)
			}
		})
	}
}

func TestScanOutageKeepsLibrary(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "one.flac", sampleFLAC)
	l.copyTo(root, "two.mp3", sampleMP3)
	l.copyTo(root, "three.m4a", sampleM4A)
	l.scan(Quick)
	away := root + ".away"
	if err := os.Rename(root, away); err != nil {
		t.Fatal(err)
	}
	wantStats(t, l.scan(Quick), &Stats{SkippedRoots: 1})
	l.clock.Advance(testGrace + time.Minute)
	wantStats(t, l.scan(Quick), &Stats{SkippedRoots: 1})
	if st := l.states(root); len(st) != 3 {
		t.Errorf("states = %+v, want all 3 tracks", st)
	}
}

func TestScanOutageHoldsOldMissing(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, xFLAC, sampleFLAC)
	l.scan(Quick)
	remove(t, root, xFLAC)
	l.scan(Quick)
	l.clock.Advance(2 * testGrace)
	away := root + ".away"
	if err := os.Rename(root, away); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Deleted != 0 || stats.SkippedRoots != 1 {
		t.Errorf("during the outage = %+v, want Deleted 0, SkippedRoots 1", stats)
	}
	if err := os.Rename(away, root); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Deleted != 1 {
		t.Errorf("after the outage = %+v, want Deleted 1", stats)
	}
}

func TestIssueFor(t *testing.T) {
	t.Parallel()
	j := &job{Root: "/r", RelPath: aFLAC, Size: 10, MTime: time.UnixMilli(5)}
	tests := []struct {
		name     string
		err      error
		kind     store.IssueKind
		wantSize int64
	}{
		{"corrupted", &audiometa.CorruptedFileError{Path: "a", Reason: "x"}, store.IssueCorrupted, 10},
		{"out of bounds", &audiometa.OutOfBoundsError{Path: "a"}, store.IssueCorrupted, 10},
		{"unsupported", &audiometa.UnsupportedFormatError{Path: "a"}, store.IssueUnsupported, 10},
		{"wrapped", fmt.Errorf("open: %w", &audiometa.UnsupportedFormatError{Path: "a"}), store.IssueUnsupported, 10},
		{"permission", os.ErrPermission, store.IssueUnreadable, retrySize},
		{"anything else", errors.New("boom"), store.IssueUnreadable, retrySize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			is := issueFor(j, tt.err)
			if is.Kind != tt.kind || is.Size != tt.wantSize || is.Message != tt.err.Error() {
				t.Errorf("issue = %+v, want kind %s, size %d, message %q", is, tt.kind, tt.wantSize, tt.err)
			}
			if is.Root != "/r" || is.RelPath != aFLAC || !is.MTime.Equal(j.MTime) {
				t.Errorf("issue = %+v, want the job's location and mtime", is)
			}
		})
	}
}

func TestScanGivesEachAlbumAPublicID(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.scan(Quick)
	path := filepath.Join(root, "one.flac")
	retag(t, path, "Changed") // The update branch returns early from identify; it still needs an album ID.
	bump(t, path, 2)
	l.scan(Quick)

	db, err := sql.Open("sqlite3", "file:"+l.dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(), "SELECT ulid FROM albums")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if len(id) != 26 || seen[id] {
			t.Errorf("album ULID %q is not a fresh 26-character ID", id)
		}
		seen[id] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) < 2 {
		t.Errorf("%d albums, want at least 2", len(seen))
	}
}

// artistIDs returns the public ID of every artist, by name.
func artistIDs(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(), "SELECT name, ulid FROM artists")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]string{}
	for rows.Next() {
		var name, id string
		if err = rows.Scan(&name, &id); err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestScanKeepsArtistIDs(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	path := filepath.Join(root, "one.flac")
	retag(t, path, "Before")
	l.scan(Quick)
	before := artistIDs(t, l.dbPath)
	id := before["Before"]
	if len(id) != 26 {
		t.Fatalf("artist ULID = %q, want 26 characters", id)
	}

	l.scan(Full)
	if after := artistIDs(t, l.dbPath); !reflect.DeepEqual(after, before) {
		t.Errorf("after a full scan: %v, want %v", after, before)
	}

	retag(t, path, otherName) // The update branch returns early from identify; a new artist still needs an ID.
	bump(t, path, 2)
	l.scan(Quick)
	after := artistIDs(t, l.dbPath)
	if got := after[otherName]; len(got) != 26 || got == id {
		t.Errorf("new artist's ULID = %q, want a fresh 26-character ID", got)
	}
	if _, kept := after["Before"]; kept {
		t.Error("the unused artist was not pruned")
	}
}

func TestScanRefreshesSearchNames(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	path := filepath.Join(l.roots[0], "one.flac")
	l.copyTo(l.roots[0], "one.flac", sampleFLAC)
	retag(t, path, "Findable")
	l.scan(Quick)

	found, err := l.st.Search(t.Context(), store.ParseSearch("findable"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Artists) != 1 || len(found.Albums) != 1 {
		t.Errorf("after a scan: %+v, want the artist and the album", found)
	}
}
