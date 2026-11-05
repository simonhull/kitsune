package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// editLib is a scanned library of aFLAC and bystander, one album.
type editLib struct {
	*testLib

	root  string
	album store.AlbumDetail
}

func newEditLib(t *testing.T) *editLib {
	t.Helper()
	l := &editLib{testLib: newTestLib(t, 1)}
	l.root = l.roots[0]
	l.copyTo(l.root, aFLAC, sampleFLAC)
	l.copyTo(l.root, bystander, sampleFLAC)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	l.album = l.albumDetail()
	return l
}

// editAlbum sends a year edit with the album's current ETag.
func (l *editLib) editAlbum() error {
	l.t.Helper()
	return l.scanner.EditAlbum(l.t.Context(), l.album.ID, l.album.ETag, &Edit{Year: new(newYear)})
}

// wantRefused checks err is an [*EditError] from before the first write.
func wantRefused(t *testing.T, err, want error) {
	t.Helper()
	var stopped *EditError
	if !errors.As(err, &stopped) || !errors.Is(err, want) {
		t.Fatalf("err = %v, want an EditError wrapping %v", err, want)
	}
	if len(stopped.Changed) != 0 || len(stopped.Remaining) != 2 {
		t.Errorf("changed %q, remaining %q: want nothing changed", stopped.Changed, stopped.Remaining)
	}
}

// wantUnedited checks no file holds the edit.
func (l *editLib) wantUnedited() {
	l.t.Helper()
	for _, rel := range []string{aFLAC, bystander} {
		if got := reread(l.t, filepath.Join(l.root, rel)); got.Year == newYear {
			l.t.Errorf("%s took the edit", rel)
		}
	}
}

// wantAlbumEdited checks the store holds the same album and tracks with the new year.
func (l *editLib) wantAlbumEdited() {
	l.t.Helper()
	after := l.albumDetail()
	if after.ID != l.album.ID || after.Year != newYear || after.ETag == l.album.ETag || !after.AddedAt.Equal(l.album.AddedAt) {
		l.t.Errorf("album = %+v, want the same album with the new year and ETag", after.AlbumSummary)
	}
	for i := range after.Tracks {
		if tr := &after.Tracks[i]; tr.Year != newYear || tr.ID != l.album.Tracks[i].ID {
			l.t.Errorf("track %d = %+v, want the new year on the same ID", i, tr)
		}
	}
}

func TestEditAlbumWritesEveryFileAndIndexesIt(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	before := reread(t, filepath.Join(l.root, aFLAC))
	if err := l.editAlbum(); err != nil {
		t.Fatal(err)
	}
	l.wantAlbumEdited()
	if got := reread(t, filepath.Join(l.root, aFLAC)); got.Digest != before.Digest {
		t.Error("the audio digest changed")
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
	temps, err := filepath.Glob(filepath.Join(l.root, ".*"))
	if err != nil || len(temps) != 0 {
		t.Errorf("left behind %d hidden files (%v)", len(temps), err)
	}
}

func TestEditTrackLeavesItsNeighborAlone(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	tr := &l.album.Tracks[0]
	if err := l.scanner.EditTrack(t.Context(), tr.ID, tr.ETag, &Edit{Title: new(newTitle)}); err != nil {
		t.Fatal(err)
	}
	after := l.albumDetail()
	if got := retitled(&l.album, &after); !slices.Equal(got, []string{newTitle}) {
		t.Errorf("titles that changed = %q", got)
	}
	if i := slices.IndexFunc(after.Tracks, func(t store.Track) bool { return t.ID != tr.ID }); after.Tracks[i].ETag != l.etagOf(after.Tracks[i].ID) {
		t.Error("the other track's ETag changed")
	}
}

// etagOf returns the ETag the track had before the edit.
func (l *editLib) etagOf(id string) string {
	i := slices.IndexFunc(l.album.Tracks, func(t store.Track) bool { return t.ID == id })
	return l.album.Tracks[i].ETag
}

func TestEditComparesTheETagUnderTheLock(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, `"stale"`, &Edit{Year: new(newYear)}); !errors.Is(err, ErrStale) {
		t.Errorf("EditAlbum = %v, want ErrStale", err)
	}
	if err := l.scanner.EditTrack(t.Context(), l.album.Tracks[0].ID, l.album.ETag, &Edit{Year: new(newYear)}); !errors.Is(err, ErrStale) {
		t.Errorf("EditTrack with the album's ETag = %v, want ErrStale", err)
	}
	l.wantUnedited()
}

func TestEditOfAVanishedFileIsStale(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	remove(t, l.root, bystander)
	if err := l.editAlbum(); !errors.Is(err, ErrStale) {
		t.Errorf("EditAlbum = %v, want ErrStale", err)
	}
	if got := reread(t, filepath.Join(l.root, aFLAC)); got.Year == newYear {
		t.Error("the surviving file took the edit")
	}
}

func TestEditWhileBusyIsRefused(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	l.scanner.lock <- struct{}{}
	defer l.scanner.lock.unlock()
	if err := l.editAlbum(); !errors.Is(err, ErrBusy) {
		t.Errorf("EditAlbum = %v, want ErrBusy", err)
	}
}

func TestEditOfUnknownIDs(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if err := l.scanner.EditAlbum(t.Context(), "nope", "x", &Edit{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("EditAlbum = %v, want ErrNotFound", err)
	}
	if err := l.scanner.EditTrack(t.Context(), "nope", "x", &Edit{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("EditTrack = %v, want ErrNotFound", err)
	}
}

func TestPreflightRefusesAReadOnlyFile(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if err := os.Chmod(filepath.Join(l.root, bystander), 0o444); err != nil { //nolint:gosec // G302: read-only is the point.
		t.Fatal(err)
	}
	wantRefused(t, l.editAlbum(), ErrNotWritable)
	l.wantUnedited()
}

func TestPreflightRefusesAReadOnlyFolder(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if err := os.Chmod(l.root, 0o555); err != nil { //nolint:gosec // G302: the point of the test.
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(l.root, dirMode); err != nil {
			t.Error(err)
		}
	})
	wantRefused(t, l.editAlbum(), ErrNotWritable)
	l.wantUnedited()
}

func TestPreflightRefusesARootThatLeftTheConfig(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	l.opts.Roots = []string{t.TempDir()}
	l.scanner = NewScanner(l.st, l.opts)
	wantRefused(t, l.editAlbum(), ErrNotWritable)
	l.wantUnedited()
}

func TestPreflightRefusesASymlinkOutOfTheRoot(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	outside := filepath.Join(t.TempDir(), "outside.flac")
	link := filepath.Join(l.root, bystander)
	if err := os.Rename(link, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	l.scan(Quick) // The link's target keeps the size but takes the rename's time.
	l.album = l.albumDetail()
	wantRefused(t, l.editAlbum(), ErrNotWritable)
	if got := reread(t, outside); got.Year == newYear {
		t.Error("the write followed the link out of the root")
	}
}

func TestSymlinkInsideTheRootIsEdited(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	target := filepath.Join(l.root, ".store", bystander)
	if err := os.Mkdir(filepath.Dir(target), dirMode); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(l.root, bystander)
	if err := os.Rename(link, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	l.scan(Quick)
	l.album = l.albumDetail()
	if err := l.editAlbum(); err != nil {
		t.Fatal(err)
	}
	if got := reread(t, target); got.Year != newYear {
		t.Errorf("target year = %d, want %d", got.Year, newYear)
	}
}

func TestPreflightRefusesWhatTheFileCantTake(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &Edit{Title: new("  ")})
	wantRefused(t, err, ErrUnsupportedEdit)
}

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   error
		want error
	}{
		{"unsupported write", audiometa.ErrUnsupportedWrite, ErrUnsupportedEdit},
		{"DRM", audiometa.ErrDRMProtected, ErrUnsupportedEdit},
		{"too large", audiometa.ErrTagTooLarge, ErrUnsupportedEdit},
		{"permission", fs.ErrPermission, ErrNotWritable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := classify(tt.in); !errors.Is(err, tt.want) || !errors.Is(err, tt.in) {
				t.Errorf("classify(%v) = %v, want both errors", tt.in, err)
			}
		})
	}
}

func TestWriteStopsAndReindexesWhatChanged(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	boom := errors.New("disk full")
	writes := 0
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		writes++
		if writes == 2 {
			return boom
		}
		return audiometa.WriteContext(ctx, path, edit)
	}
	err := l.editAlbum()
	stopped, ok := errors.AsType[*EditError](err)
	if !ok || !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want an EditError wrapping the cause", err)
	}
	if len(stopped.Changed) != 1 || len(stopped.Remaining) != 1 {
		t.Fatalf("changed %q, remaining %q", stopped.Changed, stopped.Remaining)
	}
	after := l.albumDetail()
	for i := range after.Tracks {
		tr := &after.Tracks[i]
		if slices.Contains(stopped.Changed, tr.ID) != (tr.Year == newYear) {
			t.Errorf("track %s year %d, changed %q: the store must follow the files", tr.ID, tr.Year, stopped.Changed)
		}
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
}

func TestCanceledEditIndexesTheFileItWrote(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	l.scanner.write = func(_ context.Context, path string, edit func(*audiometa.File) error) error {
		defer cancel() // The file is written, and the signal arrives.
		return audiometa.Write(path, edit)
	}
	if err := l.scanner.EditAlbum(ctx, l.album.ID, l.album.ETag, &Edit{Year: new(newYear)}); err != nil {
		t.Logf("EditAlbum = %v", err) // Either way, the store must match the disk.
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
}
