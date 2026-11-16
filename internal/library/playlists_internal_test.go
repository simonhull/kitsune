package library

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	listRel = "Playlists/mix.m3u8"
	oneRel  = "a/one.flac"
	twoRel  = "a/two.flac"
)

// filePlaylist returns the file playlist read from the file at rel, as anyone
// sees it, and whether there is one.
func (l *testLib) filePlaylist(rel string) (store.PlaylistDetail, bool) {
	l.t.Helper()
	list, err := l.st.Playlists(l.t.Context(), 1, nil, nil)
	if err != nil {
		l.t.Fatal(err)
	}
	for i := range list {
		if f := list[i].File; f != nil && f.RelPath == rel {
			d, detailErr := l.st.Playlist(l.t.Context(), 1, list[i].ID, nil)
			if detailErr != nil {
				l.t.Fatal(detailErr)
			}
			return d, true
		}
	}
	return store.PlaylistDetail{}, false
}

// mustFilePlaylist is filePlaylist for a file playlist that must be there.
func (l *testLib) mustFilePlaylist(rel string) store.PlaylistDetail {
	l.t.Helper()
	d, ok := l.filePlaylist(rel)
	if !ok {
		l.t.Fatalf("no file playlist read from %s", rel)
	}
	return d
}

// paths returns the path of each song of d, in order.
func (l *testLib) paths(d *store.PlaylistDetail) []string {
	l.t.Helper()
	out := make([]string, len(d.Entries))
	for i := range d.Entries {
		f, err := l.st.TrackFile(l.t.Context(), d.Entries[i].Track.ID)
		if err != nil {
			l.t.Fatal(err)
		}
		out[i] = filepath.ToSlash(f.RelPath)
	}
	return out
}

// reasons returns each missing line of d as "<line> <reason> <text>".
func reasons(d *store.PlaylistDetail) []string {
	out := make([]string, len(d.Missing))
	for i, m := range d.Missing {
		out[i] = strconv.Itoa(m.Line) + " " + m.Reason + " " + m.Text
	}
	return out
}

// A playlist file and the songs it names arrive in one scan; an unchanged file
// is never read again; a changed one is, keeping its ID; one gone takes its
// playlist with it.
func TestScanReadsPlaylistFiles(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneRel, sampleFLAC)
	l.copyTo(root, twoRel, sampleFLAC)
	l.write(t, root, listRel, []byte("#EXTM3U\n../a/one.flac\n../a/two.flac\n../a/one.flac\n"))
	l.wantPlaylistScan(Quick, 1, 0)
	d := l.mustFilePlaylist(listRel)
	l.wantSongs(&d, "mix", oneRel, twoRel, oneRel)
	l.wantPlaylistScan(Quick, 0, 0) // Unchanged: only stat'd.
	l.write(t, root, listRel, []byte("#PLAYLIST:Renamed\n../a/two.flac\n"))
	l.wantPlaylistScan(Quick, 1, 0)
	again := l.mustFilePlaylist(listRel)
	if again.ID != d.ID {
		t.Errorf("read again as %s, want %s kept", again.ID, d.ID)
	}
	l.wantSongs(&again, "Renamed", twoRel)
	l.wantPlaylistScan(Full, 1, 0)
	remove(t, root, listRel)
	l.wantPlaylistScan(Quick, 0, 1)
	if _, ok := l.filePlaylist(listRel); ok {
		t.Error("the playlist outlived its file")
	}
}

// wantPlaylistScan scans in mode and fails the test unless it read and deleted
// the playlists it says.
func (l *testLib) wantPlaylistScan(mode Mode, read, gone int) {
	l.t.Helper()
	if stats := l.scan(mode); stats.Playlists != read || stats.PlaylistsGone != gone {
		l.t.Errorf("%s scan = %+v, want %d playlists read and %d gone", mode, stats, read, gone)
	}
}

// wantSongs fails the test unless d is named name and holds the songs at rels,
// in order, with every line found.
func (l *testLib) wantSongs(d *store.PlaylistDetail, name string, rels ...string) {
	l.t.Helper()
	if got := l.paths(d); !slices.Equal(got, rels) || d.Name != name || len(d.Missing) != 0 {
		l.t.Errorf("playlist %q = %v, missing %+v; want %q = %v", d.Name, got, d.Missing, name, rels)
	}
}

// A root that looks down deletes no playlist, as it marks no track missing.
func TestScanOutageKeepsFilePlaylists(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneRel, sampleFLAC)
	l.write(t, root, listRel, []byte("../a/one.flac\n"))
	l.scan(Quick)
	away := filepath.Join(t.TempDir(), "away")
	if err := os.Rename(root, away); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, dirMode); err != nil { // An empty mount point.
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.PlaylistsGone != 0 || stats.SkippedRoots != 1 {
		t.Errorf("stats = %+v, want the root skipped and nothing gone", stats)
	}
	if _, ok := l.filePlaylist(listRel); !ok {
		t.Error("an outage took a file playlist")
	}
}

// Lines that found no song say why, and find it once it arrives; matching
// folds case and Unicode form when one song alone matches, never guessing
// between two.
func TestPlaylistLinesMatch(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "a/Café.flac", sampleFLAC)
	l.copyTo(root, "b/x.flac", sampleFLAC)
	l.copyTo(root, "b/X.flac", sampleFLAC)
	l.write(t, root, "mix.m3u", []byte("A/CAFÉ.FLAC\nb/x.FLAC\na/one.mp3\nhttp://radio/x\n/nowhere/c/d.mp3\nlater/three.flac\n"))
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.scan(Quick)
	d := l.mustFilePlaylist("mix.m3u")
	if got := l.paths(&d); !slices.Equal(got, []string{"a/Café.flac"}) {
		t.Errorf("songs = %v, want the folded match alone", got)
	}
	want := []string{"2 noSong b/x.FLAC", "3 noSong a/one.mp3", "4 notFile http://…", "5 outside …/c/d.mp3", "6 noSong later/three.flac"}
	if got := reasons(&d); !slices.Equal(got, want) {
		t.Errorf("missing = %q, want %q", got, want)
	}
	if d.Missing[1].OtherExtension != "one.flac" || d.Missing[0].OtherExtension != "" {
		t.Errorf("hints = %+v", d.Missing)
	}
	l.copyTo(root, "later/three.flac", sampleFLAC)
	if stats := l.scanDirs(root, "later"); stats.Playlists != 1 {
		t.Errorf("a targeted scan that added a song read %d playlists, want the one waiting for it", stats.Playlists)
	}
	d = l.mustFilePlaylist("mix.m3u")
	if got := l.paths(&d); !slices.Equal(got, []string{"a/Café.flac", "later/three.flac"}) {
		t.Errorf("songs after the song arrived = %v", got)
	}
}

// The watcher's targeted scan reads a playlist file in its folder, and takes
// the playlist away when the file goes.
func TestScanDirsReadsPlaylistFiles(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneRel, sampleFLAC)
	l.scan(Quick)
	l.write(t, root, listRel, []byte("../a/one.flac\n"))
	if stats := l.scanDirs(root, "Playlists"); stats.Playlists != 1 {
		t.Errorf("stats = %+v, want the new file read", stats)
	}
	l.mustFilePlaylist(listRel)
	remove(t, root, listRel)
	if stats := l.scanDirs(root, "Playlists"); stats.PlaylistsGone != 1 {
		t.Errorf("stats = %+v, want the playlist gone", stats)
	}
}

// A song that moves keeps its place with no read of the file, and a read of
// the changed file keeps it by the line that named it.
func TestMovedSongKeepsItsEntry(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneRel, sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.write(t, root, listRel, []byte("../a/one.flac\n../keep.mp3\n"))
	l.scan(Quick)
	l.fingerprint()
	if err := os.MkdirAll(filepath.Join(root, "b"), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, oneRel), filepath.Join(root, "b", "one.flac")); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Moved != 1 || stats.Playlists != 0 {
		t.Errorf("stats = %+v, want the move and no read", stats)
	}
	d := l.mustFilePlaylist(listRel)
	if got := l.paths(&d); !slices.Equal(got, []string{"b/one.flac", keepMP3}) {
		t.Errorf("songs after the move = %v", got)
	}
	l.write(t, root, listRel, []byte("#EXTM3U\n../a/one.flac\n../keep.mp3\n"))
	l.scan(Quick)
	d = l.mustFilePlaylist(listRel)
	if got := l.paths(&d); !slices.Equal(got, []string{"b/one.flac", keepMP3}) || len(d.Missing) != 0 {
		t.Errorf("songs after a read = %v, missing %+v; want the moved song kept by its line", got, d.Missing)
	}
}

// wantList fails the test unless the file playlist at listRel holds songs live
// songs and missing as its lines that found none.
func (l *testLib) wantList(songs int, missing ...string) {
	l.t.Helper()
	d := l.mustFilePlaylist(listRel)
	if got := reasons(&d); len(d.Entries) != songs || !slices.Equal(got, missing) {
		l.t.Errorf("%d songs, missing %q; want %d, %q", len(d.Entries), got, songs, missing)
	}
}

// trash moves the songs ids to the Trash, failing the test on an error.
func (l *testLib) trash(ids ...string) {
	l.t.Helper()
	if _, err := l.scanner.TrashTracks(l.t.Context(), 0, ids); err != nil {
		l.t.Fatal(err)
	}
}

// A song in the Trash leaves a file playlist and is listed as in the Trash;
// Restore brings it back, and the purge takes its entry with it.
func TestTrashedSongInAFilePlaylist(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := newTestLib(t, 1)
		l.opts.TagWrites, l.opts.Now = true, time.Now
		l.scanner = NewScanner(l.st, l.opts)
		root := l.roots[0]
		l.copyTo(root, oneRel, sampleFLAC)
		l.copyTo(root, twoRel, sampleFLAC)
		l.write(t, root, listRel, []byte("../a/one.flac\n../a/two.flac\n"))
		l.scan(Quick)
		one := l.ulidAt(oneRel)
		l.trash(one)
		l.wantList(1, "1 trashed ../a/one.flac")
		if _, err := l.scanner.RestoreTracks(t.Context(), []string{one}); err != nil {
			t.Fatal(err)
		}
		l.wantList(2)
		l.trash(one)
		time.Sleep(TrashRetention + time.Second)
		if n, err := l.scanner.PurgeTrash(t.Context()); err != nil || n != 1 {
			t.Fatalf("purge = %d, %v", n, err)
		}
		l.wantList(1) // The entry went with its song.
	})
}

// A playlist file that can't be read is an unreadable scan issue, tried again
// every scan, and the issue goes once it reads.
func TestUnreadablePlaylistFileIsAnIssue(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneRel, sampleFLAC)
	path := l.write(t, root, listRel, []byte("../a/one.flac\n"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running as a user who reads any file")
	}
	if stats := l.scan(Quick); stats.Issues != 1 || stats.Playlists != 0 {
		t.Errorf("stats = %+v, want one issue", stats)
	}
	if st := l.states(root)[listRel]; !st.Issue {
		t.Errorf("state = %+v, want an issue", st)
	}
	if err := os.Chmod(path, fileMode); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Quick); stats.Playlists != 1 {
		t.Errorf("stats = %+v, want the file read", stats)
	}
	if _, issue := l.states(root)[listRel]; issue {
		t.Error("the issue outlived the read")
	}
}

// saveLib is a scanned library of one song.
func saveLib(t *testing.T, tagWrites bool) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	l.opts.TagWrites = tagWrites
	l.scanner = NewScanner(l.st, l.opts)
	l.copyTo(l.roots[0], oneRel, sampleFLAC)
	l.scan(Quick)
	return l
}

// saveList saves data as Playlists/mix.m3u8.
func (l *testLib) saveList(data string, replace bool) (SavedPlaylist, error) {
	return l.scanner.SavePlaylist(l.t.Context(), l.roots[0], "Playlists", "mix.m3u8", []byte(data), replace)
}

// A save needs tag writes, and a folder in a root.
func TestSavePlaylistRefusals(t *testing.T) {
	t.Parallel()
	if _, err := saveLib(t, false).saveList(oneLine, false); !errors.Is(err, ErrTagWritesOff) {
		t.Errorf("save with tag writes off = %v", err)
	}
	l := saveLib(t, true)
	for _, dir := range []string{"..", ".hidden", "/etc"} {
		if _, err := l.scanner.SavePlaylist(t.Context(), l.roots[0], dir, "mix.m3u8", nil, false); !errors.Is(err, ErrBadFolder) {
			t.Errorf("save into %q = %v, want ErrBadFolder", dir, err)
		}
	}
	if _, err := l.scanner.SavePlaylist(t.Context(), t.TempDir(), "", "mix.m3u8", nil, false); !errors.Is(err, ErrBadFolder) {
		t.Errorf("save into a folder that isn't a root = %v, want ErrBadFolder", err)
	}
}

// oneLine is a playlist file naming the one song.
const oneLine = "#EXTM3U\n../a/one.flac\n"

// A save writes through a temp file beside its target, never overwrites
// without replace, and reads the file at once.
func TestSavePlaylist(t *testing.T) {
	t.Parallel()
	l := saveLib(t, true)
	saved, err := l.saveList(oneLine, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := l.mustFilePlaylist(listRel); d.ID != saved.ID {
		t.Errorf("saved %+v, read %s", saved, d.ID)
	}
	l.wantList(1)
	_, err = l.saveList("x", false)
	if exists, ok := errors.AsType[*ExistsError](err); !ok || exists.FreeName != "mix 2.m3u8" {
		t.Errorf("a second save = %v, want an ExistsError offering mix 2.m3u8", err)
	}
	if _, err = l.saveList("#EXTM3U\n", true); err != nil {
		t.Fatal(err)
	}
	if d := l.mustFilePlaylist(listRel); d.ID != saved.ID || len(d.Entries) != 0 {
		t.Errorf("after replace: %s with %d songs, want %s with none", d.ID, len(d.Entries), saved.ID)
	}
	entries, err := os.ReadDir(filepath.Join(l.roots[0], "Playlists"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("Playlists holds %d files, want the playlist alone: no temp file left", len(entries))
	}
}
