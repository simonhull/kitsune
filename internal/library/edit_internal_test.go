package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// yearEdit is the edit most of these tests make.
func yearEdit() *Edit {
	return &Edit{Fields: map[Field][]string{FieldDate: {strconv.Itoa(newYear)}}}
}

// editAlbum sends a year edit with the album's current ETag.
func (l *editLib) editAlbum() error {
	l.t.Helper()
	return l.scanner.EditAlbum(l.t.Context(), l.album.ID, l.album.ETag, &AlbumEdit{All: *yearEdit()})
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
	if got := reread(t, filepath.Join(l.root, aFLAC)); *got.Digest != *before.Digest {
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
	edit := AlbumEdit{Tracks: map[string]Edit{tr.ID: {Fields: map[Field][]string{FieldTitle: {newTitle}}}}}
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit); err != nil {
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

func TestEditPutsATrackOnTopOfAll(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	first := &l.album.Tracks[0]
	edit := AlbumEdit{
		All:    Edit{Fields: map[Field][]string{FieldGenres: {rock}, FieldDate: {"1987"}}},
		Tracks: map[string]Edit{first.ID: {Fields: map[Field][]string{FieldTitle: {newTitle}, FieldGenres: {pop}}}},
	}
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	after := l.albumDetail()
	if got := retitled(&l.album, &after); !slices.Equal(got, []string{newTitle}) {
		t.Errorf("titles that changed = %q", got)
	}
	for i := range after.Tracks {
		tr := &after.Tracks[i]
		genre := []string{rock}
		if tr.ID == first.ID {
			genre = []string{pop}
		}
		got, err := l.st.AlbumTagValues(t.Context(), l.album.ID, GenreKeys())
		if err != nil {
			t.Fatal(err)
		}
		if tr.Year != newYear || !slices.Equal(got[tr.ID], genre) {
			t.Errorf("track %d: year %d, genres %q, want %d and %q", i, tr.Year, got[tr.ID], newYear, genre)
		}
	}
}

func TestEditRefusesATrackOfAnotherAlbum(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	edit := AlbumEdit{Tracks: map[string]Edit{"nope": *yearEdit()}}
	wantRefused(t, l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit), ErrUnsupportedEdit)
	l.wantUnedited()
}

func TestEditRefreshesTheNames(t *testing.T) {
	t.Parallel()
	failure := errors.New("disk full")
	tests := []struct {
		name string
		fail error
	}{
		{"a finished edit", nil},
		{"a stopped edit", failure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newEditLib(t)
			l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
				if tt.fail != nil {
					return tt.fail
				}
				return audiometa.WriteContext(ctx, path, edit)
			}
			tw := l.trackAt(l.root, "unindexed.flac", 1, l.opts.Now())
			tw.Album.Name, tw.Album.GroupKey = "Zebrafish", "zebrafish"
			if err := l.st.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{tw}}); err != nil {
				t.Fatal(err)
			}
			if err := l.editAlbum(); !errors.Is(err, tt.fail) {
				t.Fatalf("EditAlbum = %v, want %v", err, tt.fail)
			}
			found, err := l.st.Search(t.Context(), store.ParseSearch("zebrafish"))
			if err != nil {
				t.Fatal(err)
			}
			if len(found.Albums) != 1 {
				t.Errorf("albums = %+v, want the one the edit's refresh picked up", found.Albums)
			}
		})
	}
}

func TestEditComparesTheETagUnderTheLock(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, `"stale"`, &AlbumEdit{All: *yearEdit()}); !errors.Is(err, ErrStale) {
		t.Errorf("EditAlbum = %v, want ErrStale", err)
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
	if err := l.scanner.EditAlbum(t.Context(), "nope", "x", &AlbumEdit{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("EditAlbum = %v, want ErrNotFound", err)
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
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldTitle: {"  "}}}}
	wantRefused(t, l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit), ErrUnsupportedEdit)
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
		{"invalid artwork", audiometa.ErrInvalidArtwork, ErrUnsupportedEdit},
		{"conflicting edits", audiometa.ErrConflictingEdits, ErrUnsupportedEdit},
		{"failed verification", audiometa.ErrVerifyFailed, ErrUnsupportedEdit},
		{"concurrent change", audiometa.ErrConcurrentModification, ErrStale},
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
	if err := l.scanner.EditAlbum(ctx, l.album.ID, l.album.ETag, &AlbumEdit{All: *yearEdit()}); err != nil {
		t.Logf("EditAlbum = %v", err) // Either way, the store must match the disk.
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
}
