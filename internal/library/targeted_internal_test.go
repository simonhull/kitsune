package library

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// scanDirs runs a targeted scan and fails the test on an error it didn't expect.
func (l *testLib) scanDirs(root string, dirs ...string) *Stats {
	l.t.Helper()
	stats, err := l.scanner.ScanDirs(l.t.Context(), root, dirs)
	if err != nil {
		l.t.Fatalf("ScanDirs: %v", err)
	}
	return &stats
}

// live returns the paths under root whose tracks are not missing, sorted.
func (l *testLib) live(root string) []string {
	l.t.Helper()
	var out []string
	for rel, st := range l.states(root) {
		if st.TrackID != 0 && !st.Missing {
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	return out
}

func TestScanDirsReadsANewFolderWhole(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "old/one.flac", sampleFLAC)
	l.scan(Quick)
	l.copyTo(root, "new/cd1/two.flac", sampleFLAC)
	l.copyTo(root, "new/cd2/three.mp3", sampleMP3)
	// Its folders are asked for too, as the watcher does: each file is read once.
	stats := l.scanDirs(root, ".", "new", "new/cd1", "new/cd2")
	if stats.Added != 2 || stats.Read != 2 {
		t.Errorf("stats = %+v, want 2 read and added", stats)
	}
	want := []string{"new/cd1/two.flac", "new/cd2/three.mp3", "old/one.flac"}
	if got := l.live(root); !slices.Equal(got, want) {
		t.Errorf("live = %v, want %v", got, want)
	}
}

func TestScanDirsReadsOnlyItsFolders(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.copyTo(root, "b/two.flac", sampleFLAC)
	l.scan(Quick)
	bump(t, filepath.Join(root, "a", "one.flac"), 1)
	bump(t, filepath.Join(root, "b", "two.flac"), 1)
	remove(t, root, "b/two.flac")
	l.copyTo(root, "b/two.flac", sampleMP3) // Changed, but b isn't asked for.
	stats := l.scanDirs(root, "a")
	if stats.Read != 1 || stats.Updated != 1 || stats.Missing != 0 {
		t.Errorf("stats = %+v, want only a/one.flac read", stats)
	}
}

func TestScanDirsMarksWhatItsFolderLost(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.copyTo(root, "a/two.mp3", sampleMP3)
	l.copyTo(root, "b/three.flac", sampleFLAC)
	l.scan(Quick)
	remove(t, root, "a/two.mp3")
	if stats := l.scanDirs(root, "a"); stats.Missing != 1 {
		t.Errorf("stats = %+v, want 1 missing", stats)
	}
	want := []string{"a/one.flac", "b/three.flac"}
	if got := l.live(root); !slices.Equal(got, want) {
		t.Errorf("live = %v, want %v", got, want)
	}
}

func TestScanDirsMarksAVanishedFolderFromItsParent(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "artist/gone/cd1/one.flac", sampleFLAC)
	l.copyTo(root, "artist/gone/cd2/two.flac", sampleFLAC)
	l.copyTo(root, "artist/kept/three.flac", sampleFLAC)
	l.copyTo(root, "other/four.flac", sampleFLAC)
	l.scan(Quick)
	if err := os.RemoveAll(filepath.Join(root, "artist", "gone")); err != nil {
		t.Fatal(err)
	}
	// The vanished folder itself is asked for: it stands for its parent.
	if stats := l.scanDirs(root, "artist/gone/cd1"); stats.Missing != 2 {
		t.Errorf("stats = %+v, want 2 missing", stats)
	}
	want := []string{"artist/kept/three.flac", "other/four.flac"}
	if got := l.live(root); !slices.Equal(got, want) {
		t.Errorf("live = %v, want %v", got, want)
	}
}

func TestScanDirsMoveKeepsID(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "from/album/one.flac", sampleFLAC)
	l.copyTo(root, "keep.mp3", sampleMP3)
	l.scan(Quick)
	l.fingerprint()
	id := l.id(root, "from/album/one.flac")
	if err := os.MkdirAll(filepath.Join(root, "to"), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "from", "album"), filepath.Join(root, "to", "album")); err != nil {
		t.Fatal(err)
	}
	stats := l.scanDirs(root, "from", "to")
	if stats.Moved != 1 || stats.Added != 0 {
		t.Errorf("stats = %+v, want 1 moved, none added", stats)
	}
	if got := l.id(root, "to/album/one.flac"); got != id {
		t.Errorf("ID after the move = %d, want %d", got, id)
	}
}

func TestScanDirsLeavesAnUnreadableFolderAlone(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/locked/one.flac", sampleFLAC)
	l.copyTo(root, "b/two.flac", sampleFLAC)
	l.scan(Quick)
	locked := filepath.Join(root, "a", "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, dirMode) })
	if stats := l.scanDirs(root, "a/locked"); stats.Missing != 0 {
		t.Errorf("stats = %+v, want nothing missing", stats)
	}
}

func TestScanDirsLeavesAnUnavailableRootAlone(t *testing.T) {
	t.Parallel()
	for name, unmount := range map[string]func(t *testing.T, root string){
		"removed": func(t *testing.T, root string) {
			t.Helper()
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
		},
		"emptied": func(t *testing.T, root string) { // What an unmounted disk leaves.
			t.Helper()
			for _, dir := range []string{"a", "b"} {
				if err := os.RemoveAll(filepath.Join(root, dir)); err != nil {
					t.Fatal(err)
				}
			}
		},
		"every track gone": func(t *testing.T, root string) {
			t.Helper()
			remove(t, root, "a/one.flac")
			remove(t, root, "b/two.flac")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			l.copyTo(root, "a/one.flac", sampleFLAC)
			l.copyTo(root, "b/two.flac", sampleFLAC)
			l.scan(Quick)
			unmount(t, root)
			_, err := l.scanner.ScanDirs(t.Context(), root, []string{".", "a", "b"})
			if !errors.Is(err, ErrRootUnavailable) {
				t.Errorf("ScanDirs = %v, want ErrRootUnavailable", err)
			}
			if got := l.live(root); len(got) != 2 {
				t.Errorf("live = %v, want both tracks", got)
			}
		})
	}
}

func TestScanDirsLeavesAMassDisappearanceAlone(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	for i := range massFloor + 10 {
		l.copyTo(root, filepath.Join("pool", flac(i)), sampleFLAC)
	}
	l.copyTo(root, "kept/one.flac", sampleFLAC)
	l.scan(Quick)
	before := len(l.live(root))
	if err := os.RemoveAll(filepath.Join(root, "pool")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.scanner.ScanDirs(t.Context(), root, []string{"."}); !errors.Is(err, ErrRootUnavailable) {
		t.Errorf("ScanDirs = %v, want ErrRootUnavailable", err)
	}
	if after := len(l.live(root)); after != before {
		t.Errorf("live tracks = %d, want %d", after, before)
	}
}

func TestScanDirsIsRefusedWhileAScanRuns(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	run, err := l.scanner.Claim(Quick)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.scanner.ScanDirs(t.Context(), l.roots[0], []string{"."}); !errors.Is(err, ErrScanRunning) {
		t.Errorf("ScanDirs = %v, want ErrScanRunning", err)
	}
	if _, err = run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestScanDirsIgnoresWhatAScanSkips(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "@eaDir/one.flac", sampleFLAC)
	l.copyTo(root, ".hidden/two.flac", sampleFLAC)
	l.copyTo(root, "a/.three.flac", sampleFLAC)
	l.copyTo(root, TrashDir+"/01J0000000000000000000TRSH/a/four.flac", sampleFLAC)
	stats := l.scanDirs(root, ".", "@eaDir", ".hidden", "a", "../outside", TrashDir, TrashDir+"/01J0000000000000000000TRSH/a")
	if stats.Read != 0 {
		t.Errorf("stats = %+v, want nothing read", stats)
	}
}
